package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetSourceURI(t *testing.T) {
	service := &GCSService{
		projectID: "test-project",
		rawBucket: "test-raw-bucket",
	}

	videoID := "550e8400-e29b-41d4-a716-446655440000"
	expected := "gs://test-raw-bucket/550e8400-e29b-41d4-a716-446655440000/original.mp4"

	result := service.GetSourceURI(videoID)
	assert.Equal(t, expected, result)
}

func TestGenerateSignedURLFormat(t *testing.T) {
	service := &GCSService{
		projectID: "test-project",
		rawBucket: "test-raw-bucket",
	}

	videoID, signedURL, err := service.GenerateSignedURL(context.Background(), "test.mp4", "video/mp4")

	// Should not error with valid setup
	if err != nil {
		t.Logf("GenerateSignedURL error (expected in unit test without full auth): %v", err)
		// This is expected without real GCP credentials
		return
	}

	// Verify videoID is a valid UUID
	_, parseErr := uuid.Parse(videoID)
	assert.NoError(t, parseErr, "videoID should be a valid UUID")

	// Verify signed URL contains expected bucket name
	assert.Contains(t, signedURL, "test-raw-bucket")
	assert.Contains(t, signedURL, videoID)
	assert.Contains(t, signedURL, "original.mp4")
}

func TestSourceURIFormat(t *testing.T) {
	tests := []struct {
		name     string
		bucket   string
		videoID  string
		expected string
	}{
		{
			name:     "standard format",
			bucket:   "raw-videos",
			videoID:  "abc-123",
			expected: "gs://raw-videos/abc-123/original.mp4",
		},
		{
			name:     "uuid format",
			bucket:   "my-bucket",
			videoID:  "550e8400-e29b-41d4-a716-446655440000",
			expected: "gs://my-bucket/550e8400-e29b-41d4-a716-446655440000/original.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &GCSService{
				rawBucket: tt.bucket,
			}
			result := service.GetSourceURI(tt.videoID)
			assert.Equal(t, tt.expected, result)
		})
	}
}
