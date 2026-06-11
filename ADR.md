# ADR-001: Video Transcoding & Streaming Architecture

**Date**: 2026-06-11  
**Status**: Accepted  
**Context**: POC for video encoding and streaming on GCP  
**Decision Makers**: Engineering Team

## Summary

Implement a native GCP video transcoding and streaming pipeline using Cloud Storage, Cloud Transcoder API, and Cloud CDN to provide scalable, cost-effective HLS video streaming.

## Problem Statement

Need to encode and stream videos in multiple quality levels (adaptive bitrate streaming) with:
- Minimal backend complexity (serverless preferred)
- Global content delivery with low latency
- Cost-effective operation at scale
- No third-party video processing dependencies

## Decision

Use a native GCP architecture combining:
1. **Cloud Run** - Go Echo API for orchestration
2. **Cloud Storage** - Raw video storage + HLS output
3. **Cloud Transcoder API** - Server-side video encoding
4. **Cloud CDN** - Global distribution

## Architecture Overview

### High-Level Flow

```
Upload → Cloud Storage (raw) → Transcoder API → Cloud Storage (HLS) → Cloud CDN → User
```

### Complete Architecture Diagram

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

### Request Flow Sequence

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

## Component Details

### 1. Backend API (Cloud Run)

**Technology**: Go Echo v4.11.3

**Configuration**:
- Region: us-central1 (primary)
- Memory: 256MB - 2GB (auto-tuned)
- Concurrency: 80 requests per instance
- Autoscaling: 0-100 instances
- Timeout: 60 minutes (for long transcoding status checks)

**Responsibilities**:
- Generate time-limited signed URLs for client uploads
- Orchestrate transcoding jobs on Cloud Transcoder API
- Monitor job status and return progress
- Handle errors with proper HTTP status codes

**Endpoints**:
```
POST   /videos/upload-url                    → Generate signed URL
POST   /videos/:video_id/transcode           → Start transcoding job
GET    /videos/transcode/:job_id/status      → Check job progress
GET    /health                               → Liveness probe
```

### 2. Cloud Storage - Raw Bucket

**Purpose**: Temporary storage for source videos  
**Lifecycle**: Auto-delete after 30 days  
**Access Pattern**: Write once (signed URL), read by Transcoder API  
**Security**: Private, no public access  

**Bucket Structure**:
```
gs://raw-bucket/
└── {video_id}/
    └── original.mp4
```

**Cost Optimization**:
- Lifecycle policy removes files automatically
- Reduces long-term storage costs
- Frees quota for new uploads

### 3. Cloud Transcoder API

**Service**: Cloud Video Transcoding API  
**Location**: us-central1  
**Preset**: `preset/web-hls` (pre-configured, optimized)

**Output Configuration**:
- **Formats**: HLS (HTTP Live Streaming)
- **Renditions**: 
  - 360p @ 800 kbps (mobile)
  - 720p @ 2.5 Mbps (tablet)
  - 1080p @ 5 Mbps (desktop)
- **Segment Duration**: 10 seconds
- **Codecs**: H.264 video, AAC audio
- **Master Playlist**: Adaptive bitrate selection

**Job States**:
- `PENDING`: Queued, waiting to start
- `RUNNING`: Currently processing
- `SUCCEEDED`: Completed successfully
- `FAILED`: Error occurred (check logs)

**Processing Time**:
- Typical: 2-5 minutes for 1-hour video
- Depends on: Video length, output quality, concurrent jobs
- Cost: $0.001 per minute of output (per rendition)

### 4. Cloud Storage - HLS Bucket

**Purpose**: Permanent storage of encoded streams  
**Access Pattern**: Read by Cloud CDN (cached globally)  
**Lifecycle**: Permanent (no auto-delete)  

**Bucket Structure**:
```
gs://hls-bucket/
└── {video_id}/
    ├── master.m3u8              (updated during encoding)
    ├── 360p_playlist.m3u8       (10s cache)
    ├── 720p_playlist.m3u8       (10s cache)
    ├── 1080p_playlist.m3u8      (10s cache)
    └── segments/
        ├── 360p_00001.ts        (1-year cache)
        ├── 360p_00002.ts
        ├── 720p_00001.ts
        └── ...
```

**Cache Headers** (set by Transcoder):
- `.ts` segments: `Cache-Control: public, max-age=31536000` (1 year)
- `.m3u8` playlists: `Cache-Control: public, max-age=10` (10 seconds)

### 5. Cloud CDN

**Backend**: Cloud Storage (HLS bucket)  
**Distribution**: Global edge locations  

**Regional PoP**: 
- **Primary**: Jakarta (CGK) for Indonesia
- **Regional**: Singapore, Sydney, Tokyo, Hong Kong

**Cache Strategy**:
- **Segments** (`.ts`): Cached forever (31536000 seconds)
  - Immutable once encoded
  - Massive bandwidth savings
  - First request: ~100ms (origin)
  - Cached request: <50ms (PoP)
  
- **Playlists** (`.m3u8`): Short TTL (10 seconds)
  - Reflects new segments during encoding
  - Balances freshness vs cache efficiency
  - Updated as encoding progresses

**Cache Hit Ratio Target**: 70-80%
- Typical video: 80%+ hit ratio
- Popular content: 90%+ hit ratio
- Obscure content: 20-30% hit ratio

**Bandwidth Cost Reduction**:
- Without CDN: $0.12/GB egress
- With CDN (75% hit): $0.03/GB egress
- Savings: 75% reduction

### 6. Cloud Load Balancer & SSL

**Configuration**:
- **Type**: Global HTTP(S) Load Balancer
- **SSL Certificate**: Google-managed or custom
- **Backend**: Cloud CDN
- **Forwarding Rules**: IPv4/IPv6, port 443

**Purpose**:
- SSL/TLS termination
- Geographic routing to nearest CDN PoP
- DDoS protection (implicit with GCP)

## Deployment Topology

### GCP Project Structure

```
GCP Project
├── Compute
│   └── Cloud Run (video-processor)
│       ├── Service: us-central1
│       ├── Service Account: video-processor@project.iam
│       └── Concurrency: 80
├── Storage
│   ├── gs://project-raw-videos (raw bucket)
│   │   └── Lifecycle: delete after 30 days
│   └── gs://project-hls-output (HLS bucket, CDN backend)
│       └── Permanent retention
├── Processing
│   └── Cloud Transcoder API
│       ├── Location: us-central1
│       └── Preset: preset/web-hls
└── Networking
    ├── Cloud Load Balancer
    ├── Cloud CDN (backed by HLS bucket)
    └── SSL Certificate
```

### IAM & Security

**Service Account**: `video-processor@PROJECT_ID.iam.gserviceaccount.com`

**Required Roles**:
```
roles/storage.objectAdmin
  ├── Read/write: gs://raw-bucket/*
  └─ Read/write: gs://hls-bucket/*

roles/transcoder.admin
  ├── Create transcoding jobs
  ├── Get job status
  └─ List jobs
```

**Signed URL Security**:
- Expires in 15 minutes (configurable)
- Specific to video ID and object path
- Only allows PUT method
- Client uploads directly to GCS (no backend bandwidth)

### Network Topology

```
Internet Users (Global)
        │
        │ HTTPS
        ▼
Cloud Load Balancer
(Global, SSL-terminated)
        │
    ┌───┼───┬───────┬────────┐
    │   │   │       │        │
    ▼   ▼   ▼       ▼        ▼
  CDN   CDN CDN     CDN      CDN
  SG    SYD CGK     NRT      HKG
    │   │   │       │        │
    └───┼───┼───────┼────────┘
        │
        ▼
   Backend Bucket
   (gs://hls-bucket)
        │
        ▼
   Cloud Storage
```

## Scaling Considerations

### Compute Scaling (Cloud Run)

| Load | Instances | Memory | Cost/Month |
|------|-----------|--------|-----------|
| Low (0-10 req/s) | 1-2 | 256-512MB | ~$15 |
| Medium (10-100 req/s) | 5-20 | 512MB-1GB | ~$30-50 |
| High (100+ req/s) | 50-100 | 1-2GB | ~$100-200 |

**Autoscaling Rules**:
- Scale up: CPU > 60% or Memory > 75%
- Scale up: Queue depth > 30
- Min instances: 1 (cost optimization)
- Max instances: 100 (prevent runaway)
- Cooldown: 60 seconds

### Storage Scaling

| Phase | Videos | Raw Bucket | HLS Bucket | Cost/Month |
|-------|--------|-----------|-----------|-----------|
| 1 | <100 | 1-10GB | 5-50GB | $1-2 |
| 2 | 100-1K | 100-500GB | 500GB-2TB | $15-50 |
| 3 | 1K-10K | 500GB-2TB | 2TB-10TB | $200-500 |
| 4 | 10K+ | 2TB+ | 10TB+ | $1000+ |

**Actions at Scale**:
- Phase 2: Enable versioning and backups
- Phase 3: Archive old content to Coldline
- Phase 4: Multiregional replication, sharded storage

### Transcoding Job Concurrency

| Concurrency | Avg Duration | Queue | Videos/Day | Cost/Video |
|-------------|--------------|-------|-----------|-----------|
| 1-10 jobs | 1-2 hours | 0-2 | ~10 | $2-5 |
| 10-50 jobs | 30-60 min | 2-10 | ~30 | $1-2 |
| 50+ jobs | 10-30 min | 10+ | ~100+ | $0.50-1 |

**Recommendation**: Default to 5-10 concurrent jobs. Burst to 20-30 during peak hours.

## Cost Estimation

### Example: 100 videos/month (1GB each)

| Component | Calculation | Cost |
|-----------|-------------|------|
| Cloud Run | 100 req/s × 30min/day | $15 |
| Transcoder | 100 GB × $0.002/min | $200 |
| Storage (Raw) | 1TB temp × $0.02/GB | $20 |
| Storage (HLS) | 500GB × $0.02/GB | $10 |
| Egress (CDN) | 20GB origin × $0.12/GB | $5 |
| Cloud CDN | 1 forwarding rule | $20 |
| Load Balancer | Minimum monthly | $25 |
| Monitoring | Free first 150MB | Free |
| **TOTAL** | | **~$295/month** |

**Scaling Examples**:
- 1,000 videos/month: ~$1,500/month
- 5,000 videos/month: ~$6,000/month
- 10,000+ videos/month: ~$12,000/month

## Monitoring & Alerting

### Key Metrics

**Backend API (Cloud Run)**:
- Request latency (p50, p95, p99)
  - Alert: p99 > 5 seconds
- Error rate (5xx, 4xx)
  - Alert: > 1%
- Instance count
  - Alert: Maxing out (approaching limit)
- Memory utilization
  - Alert: > 90%

**Cloud Transcoder**:
- Job success rate
  - Alert: < 95%
- Job duration (avg, max)
  - Alert: Avg > 2× baseline
- Queue depth
  - Alert: > 100 pending
- Failed jobs by reason
  - Track patterns

**Cloud Storage**:
- Bucket size
  - Alert: Raw > quota
  - Alert: HLS > threshold
- Request rate
  - Baseline: 1000+ req/s per bucket
- Lifecycle effectiveness
  - Verify auto-delete working

**Cloud CDN**:
- Cache hit ratio (per region)
  - Alert: < 70%
- Request latency (origin vs CDN)
  - CDN should be < 50ms
- Egress cost
  - Alert: > budget
- BytesCached / BytesServed
  - Target: > 80%

## Advantages of This Architecture

✅ **Serverless**: No VM management (Cloud Run + Transcoder API)  
✅ **Scalable**: Auto-scaling from 0-100 instances  
✅ **Cost-Effective**: Pay per use, lifecycle auto-cleanup  
✅ **Global**: CDN with 30+ edge locations  
✅ **Low Latency**: Jakarta PoP for Indonesia users (<50ms)  
✅ **Secure**: Signed URLs, private buckets, IAM  
✅ **Simple**: No third-party integrations needed  
✅ **Managed**: GCP manages transcoding, CDN, load balancing  

## Trade-offs & Considerations

| Factor | Choice | Alternative | Reason |
|--------|--------|-------------|--------|
| **Encoding Service** | Cloud Transcoder API | FFmpeg/custom | Managed, scalable, cheaper at scale |
| **API Framework** | Go Echo | Node.js, Python | Lightweight, fast, minimal overhead |
| **Storage** | GCS | DynamoDB + S3 | Native to GCP, better integration |
| **CDN** | Cloud CDN | Cloudflare, CloudFront | Native, cheaper for GCP, Jakarta PoP |
| **Job Orchestration** | Polling | Pub/Sub + Cloud Tasks | Simpler for LMS, polling acceptable for UI |

## Implementation Decisions

1. **Direct Client Upload**: Signed URLs allow clients to upload directly to GCS
   - Reduces backend bandwidth costs
   - Faster upload (direct to bucket)
   - Simplifies backend (no file handling)

2. **Async Transcoding**: Jobs run asynchronously, client polls for status
   - Supports long videos (>1 hour)
   - Non-blocking operations
   - Simple webhook alternative if needed

3. **Multiple Renditions**: 360p, 720p, 1080p for adaptive bitrate
   - Supports various network conditions
   - Mobile-friendly (360p fallback)
   - Desktop-friendly (1080p option)

4. **HLS Format**: HTTP Live Streaming
   - Industry standard
   - Supported by all browsers/devices
   - Better than DASH for simplicity

5. **10-second Segments**: Balance between buffering and playlist updates
   - Typical for live/on-demand
   - Reduces rebuffering
   - Small playlist file size

## Future Enhancements

- **Pub/Sub Integration**: Replace polling with event-driven callbacks
- **Metadata DB**: Store video metadata in Firestore for faster queries
- **Thumbnail Generation**: Extract keyframe at 25% progress
- **Analytics**: Track view counts, completion rates, quality selection
- **DRM**: Add Widevine for premium content
- **Webhook Notifications**: Notify client when transcoding completes
- **Regional Transcoding**: Distribute encoding across regions for speed
- **Custom Bitrates**: Allow clients to specify output quality

## References

- [Google Cloud Transcoder API](https://cloud.google.com/transcoder/docs)
- [Cloud Storage Signed URLs](https://cloud.google.com/storage/docs/access-control/signed-urls)
- [Cloud CDN Documentation](https://cloud.google.com/cdn/docs)
- [HTTP Live Streaming (HLS) RFC 8216](https://tools.ietf.org/html/rfc8216)
- [Cloud Run Best Practices](https://cloud.google.com/run/docs/quickstarts/build-and-deploy)

## Approval

- **Status**: Accepted ✅
- **Date Accepted**: 2026-06-11
- **Implementation**: Complete
