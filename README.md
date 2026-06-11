# Video Encode & Streaming - GCP POC

Native GCP video transcoding and streaming pipeline using Cloud Storage, Transcoder API, and Cloud CDN.

## Architecture

```
Upload → Cloud Storage (raw) → Transcoder API → Cloud Storage (HLS output) → Cloud CDN → User
```

### Components

- **Cloud Storage (Raw Bucket)**: Stores uploaded raw video files
- **Transcoder API**: Encodes videos into HLS format with multiple renditions (360p, 720p, 1080p)
- **Cloud Storage (HLS Bucket)**: Stores HLS outputs (`.m3u8` playlists and `.ts` segments)
- **Cloud CDN**: Caches and serves HLS streams globally
- **Go Echo API**: Backend service that orchestrates the pipeline

**For detailed architecture decisions, topology diagrams, scaling strategies, and cost analysis, see [ADR.md](./ADR.md).**

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
├── ADR.md                     # Architecture Decision Record
├── TESTING.md                 # Testing guide
├── Makefile                   # Build & test targets
├── internal/
│   ├── handler/
│   │   ├── video.go          # HTTP handlers
│   │   ├── video_test.go      # Handler tests
│   │   └── integration_test.go # Integration tests
│   ├── service/
│   │   ├── interfaces.go      # Service interfaces
│   │   ├── gcs.go             # GCS signed URL logic
│   │   ├── gcs_test.go        # GCS tests
│   │   ├── transcoder.go      # Transcoder API logic
│   │   └── transcoder_test.go # Transcoder tests
│   ├── model/
│   │   ├── video.go          # Data models
│   │   └── video_test.go     # Model tests
│   └── mocks/
│       ├── gcs.go            # Mock GCS service
│       └── transcoder.go      # Mock Transcoder service
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

## Testing

Run the comprehensive test suite:

```bash
make test              # Run all tests with race detection
make test-verbose      # Verbose output with test names
make test-coverage     # Generate HTML coverage report
```

See [TESTING.md](./TESTING.md) for detailed testing guide, mock usage, and best practices.

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
