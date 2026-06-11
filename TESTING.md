# Testing Guide

This document describes the testing strategy and how to run tests for the video transcoding pipeline.

## Test Structure

### Unit Tests
- **`internal/service/gcs_test.go`**: Tests for GCS service (signed URL generation, URI formatting)
- **`internal/service/transcoder_test.go`**: Tests for Transcoder API service (job configuration, paths)
- **`internal/model/video_test.go`**: Tests for data models (JSON marshaling/unmarshaling)
- **`internal/handler/video_test.go`**: Tests for HTTP handlers (individual endpoint testing)

### Integration Tests
- **`internal/handler/integration_test.go`**: End-to-end workflow tests with mocked services

### Mocks
- **`internal/mocks/gcs.go`**: Mock GCS service using testify/mock
- **`internal/mocks/transcoder.go`**: Mock Transcoder API service using testify/mock

## Running Tests

### Run All Tests
```bash
make test
# or
go test ./...
```

### Run Tests with Verbose Output
```bash
make test-verbose
# or
go test ./... -v
```

### Run Tests with Coverage Report
```bash
make test-coverage
# or
go test ./... -race -coverprofile=coverage.out -covermode=atomic
go tool cover -html=coverage.out
```

### Run Specific Test
```bash
go test ./internal/handler -v
go test ./internal/service -v
go test -run TestGenerateUploadURL ./internal/handler
```

## Test Coverage

Current coverage by package:
- `internal/handler`: ~85% (mocked services tested)
- `internal/service`: ~23% (excluded real GCP calls)
- `internal/model`: 100% (data structures tested)

Note: Service coverage is lower because:
- `GenerateSignedURL` requires GCP credentials
- `CreateJob` and `GetJob` require real Transcoder API access
- Tests use mocks to avoid these dependencies

## Mock Services

### MockGCSService
Used in handler and integration tests to simulate GCS operations:

```go
mockGCS := new(mocks.MockGCSService)
mockGCS.On("GenerateSignedURL", mock.Anything, "file.mp4", "video/mp4").
    Return("video-id", "signed-url", nil)
```

### MockTranscoderService
Used in handler and integration tests to simulate Transcoder API:

```go
mockTranscoder := new(mocks.MockTranscoderService)
mockTranscoder.On("CreateJob", mock.Anything, videoID, sourceURI).
    Return(jobID, nil)
```

## Test Scenarios

### 1. Signed URL Generation
- **Valid request**: Generates video ID and signed URL
- **Invalid request**: Returns 400 Bad Request
- **Missing fields**: Returns 400 Bad Request

### 2. Transcoding Job Creation
- **Valid video ID**: Creates job successfully
- **Missing video ID**: Returns 400 Bad Request
- **GCS error**: Returns 500 Internal Server Error

### 3. Job Status Monitoring
- **Valid job ID**: Returns job status
- **Missing job ID**: Returns 400 Bad Request
- **API error**: Returns 500 Internal Server Error

### 4. Full Workflow
- Generates upload URL
- Uploads video (simulated)
- Triggers transcoding job
- Polls job status

## Example Test Cases

### Testing Signed URL Endpoint
```go
func TestGenerateUploadURL(t *testing.T) {
    mockGCS := new(mocks.MockGCSService)
    mockGCS.On("GenerateSignedURL", mock.Anything, "intro.mp4", "video/mp4").
        Return("video-id-123", "https://...", nil)
    
    handler := NewVideoHandler(mockGCS, mockTranscoder)
    // Make request and assert response
}
```

### Testing Error Handling
```go
func TestErrorHandling(t *testing.T) {
    mockGCS := new(mocks.MockGCSService)
    mockGCS.On("GenerateSignedURL", mock.Anything, mock.Anything, mock.Anything).
        Return("", "", assert.AnError)
    
    handler := NewVideoHandler(mockGCS, mockTranscoder)
    // Assert 500 error response
}
```

## Dependencies for Testing

Testing dependencies are in `go.mod`:
- `github.com/stretchr/testify`: Assertions and mocking
- Standard library packages: `testing`, `net/http/httptest`, `encoding/json`

## Best Practices

1. **Use interfaces**: Services are tested via interfaces for easy mocking
2. **Mock external dependencies**: GCP services are mocked to avoid credentials
3. **Test happy path and errors**: Both successful and failure cases covered
4. **Validate request/response**: JSON marshaling and HTTP status codes tested
5. **Use table-driven tests**: Multiple scenarios in single test function

## Continuous Integration

These tests are designed to run in CI/CD without GCP credentials:
- No environment variables required
- No service account JSON needed
- All external dependencies mocked
- Fast execution (< 100ms total)

## Troubleshooting

### "mock: I don't know what to return"
The mock wasn't configured for the method call. Add a `.On()` expectation before calling the handler.

### "undefined: mock.Anything"
Import `"github.com/stretchr/testify/mock"` at the top of the test file.

### Tests fail in CI but pass locally
Ensure mocks are properly reset between tests. Use `SetupTest()` if needed.

## Adding New Tests

When adding new features:
1. Add unit test for the logic
2. Add handler test for HTTP behavior
3. Add integration test for workflow
4. Update mocks if new service methods added
5. Run `make test-coverage` to verify coverage
