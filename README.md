# Video Encode & Streaming - GCP POC

Native GCP video transcoding and streaming pipeline using Cloud Storage, Transcoder API, and Cloud CDN.

## Architecture Overview

### High-Level Flow

```
┌─────────────┐      ┌──────────────┐      ┌────────────────┐      ┌──────────────┐
│   Client    │      │  Go Echo     │      │  Cloud         │      │  HLS         │
│  (Browser)  │─────▶│  API Server  │─────▶│  Transcoder    │─────▶│  Output      │
│             │      │  (Cloud Run) │      │  API           │      │  (GCS)       │
└─────────────┘      └──────────────┘      └────────────────┘      └──────────────┘
       │                     │                      │                      │
       │                     │                      │                      │
       └─────────────────────┼──────────────────────┼──────────────────────┘
                             │                      │
                        PUT (signed URL)       Trigger Job
                        Direct Upload

        Raw Video            Status Check
        (GCS Bucket)
        
        ┌──────────┐
        │GCS Bucket│
        │  (Raw)   │
        └────┬─────┘
             │
             │ Input
             ▼
        ┌──────────────────────┐
        │  Transcoder API      │
        │  - 360p/720p/1080p   │
        │  - HLS segments      │
        │  - Master playlist   │
        └──────────┬───────────┘
                   │
                   │ Output
                   ▼
        ┌──────────────────────────┐
        │ GCS HLS Bucket           │
        │ - master.m3u8            │
        │ - variant playlists      │
        │ - .ts segments (10s)     │
        └──────────┬───────────────┘
                   │
                   │ Serve
                   ▼
        ┌──────────────────────────┐
        │ Cloud CDN                │
        │ - Global cache           │
        │ - Jakarta PoP            │
        │ - 31536000s TTL (segments)
        │ - 10s TTL (playlists)    │
        └──────────────────────────┘
```

### Request Flow Diagram

```
        Client Request                Backend API                GCP Services
        ═══════════════                ════════════                ════════════

1. GET /videos/upload-url
   {filename, content_type}  ──────────────┐
                                           │
                                    ┌──────▼─────┐
                                    │ Generate   │
                                    │ Signed URL │
                                    └──────┬─────┘
                                           │
                            ┌──────────────┤
                            │              │
                            ▼              ▼
                       Return URL    [GCS Bucket]
                       + Video ID    (Raw Storage)
                                    
2. Client Uploads Video
   PUT /<signed-url>    ────────────────┐
   <binary video data>                   │
                                    ┌────▼──┐
                                    │ GCS   │
                                    │ Store │
                                    └───────┘

3. POST /videos/{video_id}/transcode  ──────┐
   {preset: "preset/web-hls"}               │
                                    ┌───────▼────────┐
                                    │ Create Job     │
                                    │ Submit to      │
                                    │ Transcoder API │
                                    └───────┬────────┘
                                            │
                                    ┌───────▼────────┐
                                    │ Transcoder API │
                                    │ - Encode 360p  │
                                    │ - Encode 720p  │
                                    │ - Encode 1080p │
                                    │ - Generate HLS │
                                    └───────┬────────┘
                                            │
                                    ┌───────▼────────┐
                                    │ Output to HLS  │
                                    │ Bucket         │
                                    └────────────────┘
                            Return Job ID

4. GET /videos/transcode/{job_id}/status  ──┐
                                            │
                                    ┌───────▼──────────┐
                                    │ Query Job Status │
                                    │ from Transcoder  │
                                    └───────┬──────────┘
                                            │
                                    Return: {state, progress}
```

### System Components

#### 1. Backend API (Cloud Run)
- **Framework**: Go Echo
- **Language**: Go 1.21+
- **Location**: Cloud Run (us-central1)
- **Responsibilities**:
  - Generate signed URLs for client uploads
  - Orchestrate transcoding jobs
  - Monitor job status
  - Handle errors and retries

#### 2. Cloud Storage - Raw Bucket
- **Purpose**: Store uploaded source videos
- **Lifecycle**: Auto-delete after 30 days
- **Access**: Private, via signed URLs only
- **Size**: Temporary (cleaned up post-encode)

#### 3. Cloud Transcoder API
- **Presets**: `preset/web-hls` (pre-configured)
- **Output Formats**:
  - HLS (HTTP Live Streaming)
  - Multiple renditions (360p, 720p, 1080p)
  - 10-second segments (.ts files)
  - Master playlist (.m3u8)
- **Processing**: Asynchronous, typically 2-20 mins depending on video length

#### 4. Cloud Storage - HLS Bucket
- **Purpose**: Store encoded HLS streams
- **Structure**:
  ```
  gs://hls-bucket/
  └── {video_id}/
      ├── master.m3u8         (no-cache, updated frequently)
      ├── 360p_playlist.m3u8   (short TTL, 10s)
      ├── 720p_playlist.m3u8   (short TTL, 10s)
      ├── 1080p_playlist.m3u8  (short TTL, 10s)
      └── segments/
          ├── 360p_00001.ts    (cache forever)
          ├── 360p_00002.ts    (cache forever)
          ├── 720p_00001.ts    (cache forever)
          └── ...
  ```

#### 5. Cloud CDN
- **Backend**: Cloud Storage (HLS bucket)
- **Distribution**: Global edge locations
- **Regional PoP**: Jakarta (CGK) for Indonesia latency
- **Cache Strategy**:
  - `.ts` files: 31536000 seconds (1 year, immutable)
  - `.m3u8` playlists: 10 seconds (frequent updates)

### Network Topology

```
                    ┌─────────────────────────────────┐
                    │      Internet Users             │
                    │   (Browser/Mobile Clients)      │
                    └──────────────┬──────────────────┘
                                   │
                                   │ HTTPS
                                   ▼
                    ┌─────────────────────────────────┐
                    │    Cloud Load Balancer          │
                    │   (Global, SSL-terminated)      │
                    └──────────────┬──────────────────┘
                                   │
                    ┌──────────────┬──────────────────┐
                    │              │                  │
                    ▼              ▼                  ▼
            ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
            │ Cloud CDN   │  │ Cloud CDN   │  │ Cloud CDN   │
            │ Edge (SGP)  │  │ Edge (SYD)  │  │ Edge (CGK)  │
            └──────┬──────┘  └──────┬──────┘  └──────┬──────┘
                   │                │                │
                   └────────────────┼────────────────┘
                                    │
                                    ▼
                    ┌─────────────────────────────────┐
                    │   Backend Bucket (GCS)          │
                    │   - HLS Manifests               │
                    │   - Video Segments              │
                    │   - Cache-Control Headers       │
                    └─────────────────────────────────┘
```

### Data Flow with Timing

```
Time    Client                Backend                GCS                Transcoder
────────────────────────────────────────────────────────────────────────────────
T+0     POST /upload-url  ──────────────────────────────────────────────────────►
        Request signed URL

T+0.2   ◄──────────────── Return signed_url + video_id ──────────────────────────
        {video_id, signed_url, expires_in: 900}

T+1     PUT /<signed-url> ──────────────────────────────────────────────────────►
        [video.mp4 data]   
                                                     Store in
                                                     gs://raw-bucket/{video_id}/
T+2     ◄──────────────── 200 OK ──────────────────────────────────────────────
        Upload complete

T+2     POST /transcode   ──────────────────────────────────────────────────────►
        {video_id}

T+2.5   ◄──────────────── Return job_id + PENDING status ──────────────────────
        {job_id, state: PENDING}
                                         Read from bucket ──────────────────────►
                                         Submit job
                                                              ┌─ Processing starts
                                                              │ (2-20 minutes)
T+5     GET /transcode/{job_id}/status ───────────────────────────────────────►
        (polling)         

T+5.2   ◄──────────────── {state: RUNNING, progress: 10} ──────────────────────

...polling continues...

T+N     (After transcoding completes)
        GET /transcode/{job_id}/status ───────────────────────────────────────►

T+N.2   ◄──────────────── {state: SUCCEEDED, progress: 100} ─────────────────
        Ready to stream! 
                                                              Output to
                                                              gs://hls-bucket/{video_id}/
T+N+1   GET master.m3u8 ──────────────────────────────────────────────────────►
        (via Cloud CDN)

T+N+1.1 ◄──────────────── CDN Hits (or first request) ──────────────────────
        m3u8 content
        (cached 10 seconds)

T+N+1.2 GET segment-00001.ts ───────────────────────────────────────────────────►
        (via Cloud CDN)

T+N+1.3 ◄──────────────── CDN serves segment ──────────────────────────────────
        (cached 1 year)
```

### Deployment Topology

```
GCP Project Structure:
═══════════════════════════════════════════════════════════════════

┌─────────────────────────────────────────────────────────────────┐
│                     GCP Project                                 │
│                  (my-project-id)                                │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Cloud Run (Compute)                                      │  │
│  │                                                          │  │
│  │  ┌────────────────────────────────────────────────────┐ │  │
│  │  │ video-processor (Go Echo)                          │ │  │
│  │  │ - Instance: us-central1                            │ │  │
│  │  │ - Memory: 256MB - 2GB                              │ │  │
│  │  │ - Concurrency: 80                                  │ │  │
│  │  │ - Autoscale: 0-100 instances                       │ │  │
│  │  │ - Service Account: video-processor@project.iam...  │ │  │
│  │  │                                                    │ │  │
│  │  │ Endpoints:                                         │ │  │
│  │  │   POST  /videos/upload-url                        │ │  │
│  │  │   POST  /videos/{id}/transcode                    │ │  │
│  │  │   GET   /videos/transcode/{id}/status             │ │  │
│  │  │   GET   /health                                   │ │  │
│  │  └────────────────────────────────────────────────────┘ │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Cloud Storage (Data Layer)                               │  │
│  │                                                          │  │
│  │ ┌──────────────────┐  ┌──────────────────┐             │  │
│  │ │ raw-videos       │  │ hls-output       │             │  │
│  │ │ (Private Bucket) │  │ (CDN Backend)    │             │  │
│  │ │                  │  │                  │             │  │
│  │ │ └─ video-id-1/   │  │ └─ video-id-1/   │             │  │
│  │ │    original.mp4  │  │    master.m3u8   │             │  │
│  │ │                  │  │    360p.m3u8     │             │  │
│  │ │ └─ video-id-2/   │  │    720p.m3u8     │             │  │
│  │ │    original.mp4  │  │    1080p.m3u8    │             │  │
│  │ │                  │  │    segments/     │             │  │
│  │ └──────────────────┘  └──────────────────┘             │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Cloud Transcoder API (Processing)                        │  │
│  │                                                          │  │
│  │ Location: us-central1                                   │  │
│  │ Jobs: 0-1000 concurrent                                 │  │
│  │ Output Formats:                                         │  │
│  │  - HLS with 3 renditions (360p, 720p, 1080p)          │  │
│  │  - Segment duration: 10 seconds                         │  │
│  │  - Codec: H.264 video, AAC audio                        │  │
│  │                                                          │  │
│  │ Job States:                                             │  │
│  │  - PENDING: Queued, waiting to start                   │  │
│  │  - RUNNING: Currently processing                        │  │
│  │  - SUCCEEDED: Completed successfully                    │  │
│  │  - FAILED: Error occurred                               │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

IAM & Security:
═══════════════════════════════════════════════════════════════════

Service Account: video-processor@my-project-id.iam.gserviceaccount.com

Roles:
  ✓ roles/storage.objectAdmin
    └─ Read/write raw-videos bucket
    └─ Read raw-videos/*, write hls-output/*
  
  ✓ roles/transcoder.admin  
    └─ Create and manage transcoding jobs
    └─ List jobs, get job details

Network:
═══════════════════════════════════════════════════════════════════

┌────────────────┐
│  Clients       │
│  (Global)      │
└────────┬───────┘
         │ HTTPS
         ▼
    ┌────────────────────────────────────────┐
    │ Cloud Load Balancer                    │
    │ (Global, SSL-terminated)               │
    │ Certificate: SSL/TLS                   │
    │ Domain: videos.example.com             │
    └────────────┬───────────────────────────┘
                 │
         ┌───────┴───────┬────────────┬────────────┐
         │               │            │            │
         ▼               ▼            ▼            ▼
    ┌────────┐      ┌────────┐  ┌────────┐  ┌────────┐
    │CDN PoP │      │CDN PoP │  │CDN PoP │  │CDN PoP │
    │Singapore│      │Sydney │  │Jakarta │  │Tokyo   │
    └────┬───┘      └────┬───┘  └────┬───┘  └────┬───┘
         │               │           │           │
         └───────────────┼───────────┼───────────┘
                         │
                    ┌────▼─────┐
                    │ GCS Bucket│
                    │(HLS Data) │
                    └───────────┘
```

### Regional Deployment

```
Primary Region: us-central1
├─ Cloud Run (backend API)
├─ Cloud Transcoder (processing)
├─ GCS buckets (storage)
└─ Cost: Optimized for compute

CDN Edge Locations (for Asia-Pacific):
├─ Jakarta (CGK) - Primary for Indonesia, Malaysia
├─ Singapore (SIN) - Southeast Asia
├─ Sydney (SYD) - Australia, NZ
├─ Tokyo (NRT) - Japan, Korea
└─ Hong Kong (HKG) - China, Taiwan

Latency from Jakarta:
├─ Upload: 5-20ms (direct to bucket)
├─ Status check: 10-30ms (API call)
└─ Stream playback: 1-10ms (CDN cached)
```

### Storage Architecture

```
Raw Bucket Lifecycle:
  Video Upload → Storage (30 days) → Auto-delete
  
  └─ Cost: $0.02 per GB/month
  └─ Transition: None (always hot)
  └─ Retention: 30 days (configurable)

HLS Output Bucket (Permanent):
  Encoded Output → Permanent Storage → CDN Cache
  
  ├─ .m3u8 files: 10s cache (frequent updates)
  ├─ .ts segments: Forever cache (immutable)
  └─ Cost: $0.02 per GB/month storage
            + ~$0.12 per GB egress (CDN handles)

Cache Pattern:
  ┌─────────────────┐
  │ First Request   │
  │ (no CDN cache)  │
  └────────┬────────┘
           │
           ▼
     GCS Origin
     (slow, ~100ms)
           │
           ▼
  ┌─────────────────┐
  │ Store in CDN    │
  │ Cache           │
  └────────┬────────┘
           │
  ┌────────▼────────────────────────┐
  │ Subsequent Requests (cached)    │
  │ - .ts segments: Forever (31536k)│
  │ - .m3u8 playlists: 10s          │
  │ - Response time: <50ms (CDN PoP)│
  └─────────────────────────────────┘
```

## API Endpoints

### 1. Generate Upload URL
```
POST /videos/upload-url
```

Request:
```json
{
  "filename": "intro.mp4",
  "content_type": "video/mp4"
}
```

Response:
```json
{
  "video_id": "550e8400-e29b-41d4-a716-446655440000",
  "signed_url": "https://storage.googleapis.com/raw-bucket/550e8400-e29b-41d4-a716-446655440000/original.mp4?...",
  "expires_in": 900
}
```

The client then uploads the video directly to the `signed_url` using PUT:
```bash
curl -X PUT --data-binary @video.mp4 \
  'https://storage.googleapis.com/raw-bucket/550e8400-e29b-41d4-a716-446655440000/original.mp4?...'
```

### 2. Start Transcoding Job
```
POST /videos/:video_id/transcode
```

Request (optional):
```json
{
  "preset": "preset/web-hls"
}
```

Response:
```json
{
  "job_id": "projects/my-project/locations/us-central1/jobs/1234567890",
  "state": "PENDING",
  "message": "Transcoding job submitted successfully"
}
```

### 3. Get Transcoding Status
```
GET /videos/transcode/:job_id/status
```

Response:
```json
{
  "job_id": "projects/my-project/locations/us-central1/jobs/1234567890",
  "state": "RUNNING",
  "progress": 45,
  "message": ""
}
```

Job states: `PENDING`, `RUNNING`, `SUCCEEDED`, `FAILED`

## Setup Instructions

### Prerequisites

- GCP Project with billing enabled
- Go 1.21+
- gcloud CLI configured

### Create GCS Buckets

```bash
export PROJECT_ID="your-project-id"
export RAW_BUCKET="$PROJECT_ID-raw-videos"
export HLS_BUCKET="$PROJECT_ID-hls-output"

# Create buckets
gsutil mb gs://$RAW_BUCKET
gsutil mb gs://$HLS_BUCKET

# Set lifecycle policies (optional: auto-delete old raw files after 30 days)
cat > lifecycle.json << EOF
{
  "lifecycle": {
    "rule": [{
      "action": {"type": "Delete"},
      "condition": {"age": 30}
    }]
  }
}
EOF

gsutil lifecycle set lifecycle.json gs://$RAW_BUCKET
```

### Enable Required APIs

```bash
gcloud services enable \
  transcoder.googleapis.com \
  storage.googleapis.com \
  compute.googleapis.com
```

### Service Account Setup

```bash
# Create service account
gcloud iam service-accounts create video-processor \
  --display-name="Video Processor Service Account"

export SERVICE_ACCOUNT="video-processor@${PROJECT_ID}.iam.gserviceaccount.com"

# Grant permissions
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SERVICE_ACCOUNT}" \
  --role="roles/storage.objectAdmin"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SERVICE_ACCOUNT}" \
  --role="roles/transcoder.admin"

# Create and download key
gcloud iam service-accounts keys create key.json \
  --iam-account=$SERVICE_ACCOUNT

export GOOGLE_APPLICATION_CREDENTIALS="$(pwd)/key.json"
```

### Local Development

```bash
# Clone repository
git clone https://github.com/adityasatrio/poc-video-encode-streaming-gcp.git
cd poc-video-encode-streaming-gcp

# Setup environment
cp .env.example .env
# Edit .env with your GCP details

# Install dependencies
go mod download

# Run server
go run main.go
```

The server will start on `http://localhost:8080`

## Deploy to Cloud Run

```bash
# Build and push container
gcloud run deploy video-processor \
  --source . \
  --platform managed \
  --region us-central1 \
  --set-env-vars GCP_PROJECT_ID=$PROJECT_ID,RAW_BUCKET=$RAW_BUCKET,HLS_BUCKET=$HLS_BUCKET \
  --service-account=$SERVICE_ACCOUNT

# Get service URL
gcloud run services describe video-processor --region us-central1 --format='value(status.url)'
```

## CDN Setup

### Create Backend Bucket

```bash
gcloud compute backend-buckets create video-hls-backend \
  --gcs-bucket-name=$HLS_BUCKET \
  --enable-cdn \
  --cache-mode=CACHE_ALL_STATIC

# Set cache TTL for .ts files (immutable, cache forever)
gcloud compute backend-buckets update video-hls-backend \
  --cache-ttl=31536000
```

### Create Load Balancer & URL Map

```bash
# Create URL map
gcloud compute url-maps create video-stream-map \
  --default-backend-bucket=video-hls-backend

# Create HTTPS certificate (requires domain setup)
gcloud compute ssl-certificates create video-cdn-cert \
  --certificate=cert.pem \
  --private-key=key.pem

# Create HTTPS proxy
gcloud compute target-https-proxies create video-stream-proxy \
  --url-map=video-stream-map \
  --ssl-certificates=video-cdn-cert

# Create forwarding rule
gcloud compute forwarding-rules create video-stream-rule \
  --global \
  --target-https-proxy=video-stream-proxy \
  --address=video-cdn-ip \
  --ports=443
```

## Cache Control

Configure cache headers in Cloud Storage to optimize CDN behavior:

```bash
# Cache .ts segments for 1 year (they never change)
gsutil -h "Cache-Control:public, max-age=31536000" cp output.ts gs://$HLS_BUCKET/video-id/

# Cache .m3u8 playlists for 10 seconds (playlist can change)
gsutil -h "Cache-Control:public, max-age=10" cp output.m3u8 gs://$HLS_BUCKET/video-id/
```

For programmatic control, use the Transcoder API to set metadata on outputs.

## Project Structure

```
.
├── main.go                    # Entry point, server setup
├── go.mod                     # Dependencies
├── Dockerfile                 # Cloud Run deployment
├── .env.example              # Environment template
├── internal/
│   ├── handler/
│   │   └── video.go          # HTTP handlers
│   ├── service/
│   │   ├── gcs.go            # GCS signed URL logic
│   │   └── transcoder.go      # Transcoder API logic
│   └── model/
│       └── video.go          # Data models
└── README.md                  # This file
```

## Usage Example

### Step 1: Get Upload URL

```bash
curl -X POST http://localhost:8080/videos/upload-url \
  -H "Content-Type: application/json" \
  -d '{
    "filename": "intro.mp4",
    "content_type": "video/mp4"
  }'
```

Response:
```json
{
  "video_id": "550e8400-e29b-41d4-a716-446655440000",
  "signed_url": "https://storage.googleapis.com/...",
  "expires_in": 900
}
```

### Step 2: Upload Video

```bash
curl -X PUT --data-binary @intro.mp4 \
  'https://storage.googleapis.com/...'
```

### Step 3: Trigger Transcoding

```bash
curl -X POST http://localhost:8080/videos/550e8400-e29b-41d4-a716-446655440000/transcode \
  -H "Content-Type: application/json" \
  -d '{
    "preset": "preset/web-hls"
  }'
```

Response:
```json
{
  "job_id": "projects/my-project/locations/us-central1/jobs/1234567890",
  "state": "PENDING",
  "message": "Transcoding job submitted successfully"
}
```

### Step 4: Monitor Transcoding

```bash
curl http://localhost:8080/videos/transcode/projects%2Fmy-project%2Flocations%2Fus-central1%2Fjobs%2F1234567890/status
```

### Step 5: Stream Video

Once transcoding completes, stream via Cloud CDN:
```
https://your-cdn-domain/video-id/master.m3u8
```

## Infrastructure & Scaling

### Compute Scaling (Cloud Run)

```
Request Load                 Cloud Run Configuration
═════════════════════════════════════════════════════════

Low Traffic                  Mid Traffic                  Peak Traffic
(0-10 req/s)                (10-100 req/s)              (100+ req/s)
│                           │                           │
├─ 1-2 instances            ├─ 5-20 instances           ├─ 50-100 instances
├─ 256-512MB memory         ├─ 512MB-1GB memory         ├─ 1-2GB memory
├─ Concurrency: 50-80       ├─ Concurrency: 80          ├─ Concurrency: 80
└─ Cost: ~$0.50/month       └─ Cost: ~$20-50/month      └─ Cost: ~$100-200/month

Autoscaling Rules:
├─ Scale up: CPU > 60% OR Memory > 75%
├─ Scale up: Request queue depth > 30
├─ Min instances: 1 (cost optimization)
├─ Max instances: 100 (prevent runaway)
└─ Cooldown: 60 seconds (prevent thrashing)

Per-Instance Metrics:
├─ Max Requests: 1000 concurrent
├─ Memory per instance: 256MB - 2GB
├─ CPU cores: 1-4 (based on memory)
├─ Network bandwidth: 100 Mbps per instance
└─ Timeout: 60 minutes max (transcoding jobs can be long)
```

### Storage Scaling

```
Storage Tier Evolution:
═══════════════════════════════════════════════════════

Phase 1: Small (< 100 videos)
├─ Raw bucket: 1-10GB
├─ HLS bucket: 5-50GB
├─ Cost: ~$1-2/month

Phase 2: Medium (100-1000 videos)
├─ Raw bucket: 100-500GB (temp, auto-deleted)
├─ HLS bucket: 500GB-2TB
├─ Cost: ~$15-50/month
├─ Action: Enable versioning/backups

Phase 3: Large (1000-10000 videos)
├─ Raw bucket: 500GB-2TB (temp)
├─ HLS bucket: 2TB-10TB
├─ Cost: ~$200-500/month
├─ Action: Archive old content to Coldline
├─ Action: Implement regional buckets

Phase 4: Massive (10000+ videos)
├─ Raw bucket: 2TB+ (managed bucket)
├─ HLS bucket: 10TB+ (sharded storage)
├─ Cost: $1000+/month
├─ Action: Multiregional replication
├─ Action: Datastore metadata instead of GCS
└─ Action: CDN in multiple regions
```

### Transcoding Job Scaling

```
Concurrent Jobs vs Processing Time:
═══════════════════════════════════════════════════════

Max Concurrent:   1-10 jobs      10-50 jobs     50+ jobs
───────────────────────────────────────────────────────
Duration (avg):   1-2 hours      30-60 mins     10-30 mins
───────────────────────────────────────────────────────
Queue depth:      0-2            2-10           10+ (backlog)
───────────────────────────────────────────────────────
Throughput:       ~10 videos     ~30 videos     ~100+ videos
                  per day        per day        per day
───────────────────────────────────────────────────────
Cost/video:       ~$2-5          ~$1-2          ~$0.50-1

Recommendation:
├─ Default: 5-10 concurrent jobs
├─ Burst: Allow 20-30 during peak
├─ Queue overflow: Switch to async (Pub/Sub)
└─ Monitor: GCP Cloud Monitoring API
```

### Network & CDN Optimization

```
Cache Hit Rate Impact on Bandwidth Cost:
═══════════════════════════════════════════════════════

Hit Rate    Origin Bandwidth    CDN Bandwidth    Cost Savings
────────────────────────────────────────────────────────────
0%          100 GB              100 GB           $0 (no CDN)
25%         75 GB               100 GB           20%
50%         50 GB               100 GB           40%
75%         25 GB               100 GB           70%
90%         10 GB               100 GB           90%
95%         5 GB                100 GB           95%

Typical Achievement:
├─ First view: Cache miss (100ms, origin)
├─ Same region, next 10s: Cache hit (10ms, CDN)
├─ Popular content: 85-95% cache hit ratio
├─ Obscure content: 10-30% cache hit ratio
└─ Expected: 70-80% average

Cost Reduction with CDN:
├─ Without CDN: $0.12/GB egress × 100GB = $12/month
├─ With CDN (75% hit): $0.12/GB × 25GB = $3/month
└─ Savings: 75% reduction in bandwidth cost
```

### Monitoring & Alerting

```
Key Metrics to Monitor:
═══════════════════════════════════════════════════════

Backend API (Cloud Run):
├─ Request latency (p50, p95, p99)
│  └─ Alert: p99 > 5 seconds
├─ Error rate (5xx, 4xx)
│  └─ Alert: Error rate > 1%
├─ Instance count
│  └─ Alert: Max instances scaling
└─ Memory utilization
   └─ Alert: > 90% usage

Cloud Transcoder:
├─ Job success rate
│  └─ Alert: < 95% success
├─ Job duration (avg, max)
│  └─ Alert: Avg > 2x baseline
├─ Queue depth
│  └─ Alert: > 100 pending jobs
└─ Failed jobs by reason
   └─ Track common failure patterns

Cloud Storage:
├─ Bucket size (raw, hls)
│  └─ Alert: Raw > quota
├─ Request rate (reads, writes)
│  └─ Baseline: 1000+ req/s per bucket
└─ Lifecycle policy effectiveness
   └─ Verify auto-delete working

Cloud CDN:
├─ Cache hit ratio (per region)
│  └─ Alert: < 70%
├─ Request latency (origin vs CDN)
│  └─ CDN should be <50ms
├─ Egress cost
│  └─ Alert: > budget
└─ BytesCached / BytesServed ratio
   └─ Target: > 80%
```

### Cost Estimation

```
Monthly Cost Breakdown (100 videos, 1GB each):
═══════════════════════════════════════════════════════

Component                     Calculation              Cost
────────────────────────────────────────────────────────────
Cloud Run (API)               100 req/sec * 30min/day   $15
                              = ~3000 mins/month

Cloud Transcoder              100 videos @ 1GB avg      $200
                              = 100 GB transcoded

Cloud Storage (Raw)           1TB temp (30-day TTL)     $20
                              = 1TB * $0.02/GB

Cloud Storage (HLS Output)    500GB permanent           $10
                              = 500GB * $0.02/GB

Egress (via CDN)              100GB * 80% cached       $5
                              = 20GB * $0.12/GB (origin)

Cloud CDN                      Included in Cloud LB     $20
                              (minimum monthly)

Cloud Load Balancer           1 forwarding rule         $0.035/hour
                              + SSL certificate        = ~$25/month

Monitoring & Logging          Cloud Monitoring APIs    Free (first 150MB)
                                                       + $0.25/GB

─────────────────────────────────────────────────────────────
TOTAL                                                  ~$295/month

Scaling Scenarios:
├─ 1000 videos/month:  ~$1,500/month
├─ 5000 videos/month:  ~$6,000/month
└─ 10000+ videos/month: ~$12,000/month
    (Consider volume discounts, regional optimization)
```

## Performance & Optimization

### Transcoding

- Preset `preset/web-hls` uses GCP optimized settings
- Custom config allows fine-tuning bitrates for different use cases
- Pricing: per-minute of output video
- 360p: ~2-3 mins to transcode 1 hour video
- 1080p: ~15-20 mins per output quality

### Streaming

- Cloud CDN caches `.ts` segments (immutable, cache forever)
- `.m3u8` playlists cached with short TTL (10-30 seconds) to reflect segment availability
- Jakarta PoP provides <50ms latency to Indonesia region

### Cost Optimization

1. **Raw Video Cleanup**: Use GCS lifecycle policy to delete raw files after 30 days
2. **HLS Output Caching**: Cloud CDN saves egress costs via global caching
3. **Batch Processing**: Process videos during off-peak hours for cheaper transcoding (if on GCP pricing tiers)

## Monitoring

### Cloud Logging

Transcoder job logs available in Cloud Logging:

```bash
gcloud logging read "resource.type=api" \
  --format=json \
  --limit=50
```

### Metrics

Track in Cloud Monitoring:
- Transcoder API job count (success/failure)
- Cloud CDN cache hit ratio
- GCS bucket storage growth

## Troubleshooting

### Signed URL Issues

If `storage.SignedURL()` fails with permission errors locally:

1. Ensure service account key is set: `export GOOGLE_APPLICATION_CREDENTIALS=path/to/key.json`
2. On Cloud Run, attach service account directly (no key needed)

### Transcoder Job Fails

Check job status for detailed error:
```bash
gcloud transcoder jobs describe projects/$PROJECT_ID/locations/us-central1/jobs/JOB_ID
```

Common issues:
- Source file not found: ensure raw upload completed
- Output bucket permission: verify service account has `storage.objectAdmin` on HLS bucket
- Invalid job config: validate rendition settings (bitrate, dimensions)

### CDN Cache Issues

Clear cache if needed:
```bash
gcloud compute backend-buckets update video-hls-backend --no-cache
# Then re-enable
gcloud compute backend-buckets update video-hls-backend --enable-cdn
```

## References

- [GCP Transcoder API Docs](https://cloud.google.com/transcoder/docs)
- [Cloud Storage Signed URLs](https://cloud.google.com/storage/docs/access-control/signed-urls)
- [Cloud CDN Documentation](https://cloud.google.com/cdn/docs)
- [HLS Specification](https://tools.ietf.org/html/rfc8216)
