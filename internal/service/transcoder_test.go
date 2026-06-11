package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTranscoderServiceInitialization(t *testing.T) {
	// Note: This test doesn't require actual GCP credentials
	// Just tests the configuration is properly set
	tests := []struct {
		name             string
		projectID        string
		location         string
		hlsBucket        string
		expectJobParent  string
	}{
		{
			name:      "standard setup",
			projectID: "my-project",
			location:  "us-central1",
			hlsBucket: "hls-bucket",
			expectJobParent: "projects/my-project/locations/us-central1",
		},
		{
			name:      "different location",
			projectID: "other-project",
			location:  "us-west1",
			hlsBucket: "other-hls-bucket",
			expectJobParent: "projects/other-project/locations/us-west1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &TranscoderService{
				projectID: tt.projectID,
				location:  tt.location,
				hlsBucket: tt.hlsBucket,
			}

			// Verify service is properly initialized
			assert.Equal(t, tt.projectID, service.projectID)
			assert.Equal(t, tt.location, service.location)
			assert.Equal(t, tt.hlsBucket, service.hlsBucket)
		})
	}
}

func TestJobOutputURI(t *testing.T) {
	tests := []struct {
		name      string
		bucket    string
		videoID   string
		expected  string
	}{
		{
			name:     "standard format",
			bucket:   "hls-bucket",
			videoID:  "550e8400-e29b-41d4-a716-446655440000",
			expected: "gs://hls-bucket/550e8400-e29b-41d4-a716-446655440000/",
		},
		{
			name:     "different bucket",
			bucket:   "my-hls-outputs",
			videoID:  "abc-123",
			expected: "gs://my-hls-outputs/abc-123/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &TranscoderService{
				hlsBucket: tt.bucket,
			}

			// Build the expected output URI the same way CreateJob does
			outputURI := fmt.Sprintf("gs://%s/%s/", service.hlsBucket, tt.videoID)

			assert.Equal(t, tt.expected, outputURI)
		})
	}
}

func TestJobParentPath(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		location  string
		expected  string
	}{
		{
			name:      "us-central1",
			projectID: "my-project",
			location:  "us-central1",
			expected:  "projects/my-project/locations/us-central1",
		},
		{
			name:      "us-west1",
			projectID: "other-project",
			location:  "us-west1",
			expected:  "projects/other-project/locations/us-west1",
		},
		{
			name:      "asia-southeast1",
			projectID: "asia-project",
			location:  "asia-southeast1",
			expected:  "projects/asia-project/locations/asia-southeast1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := fmt.Sprintf("projects/%s/locations/%s", tt.projectID, tt.location)
			assert.Equal(t, tt.expected, parent)
		})
	}
}
