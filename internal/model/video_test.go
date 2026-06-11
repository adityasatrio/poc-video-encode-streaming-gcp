package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUploadURLRequestMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected UploadURLRequest
	}{
		{
			name: "valid mp4",
			json: `{"filename":"video.mp4","content_type":"video/mp4"}`,
			expected: UploadURLRequest{
				Filename:    "video.mp4",
				ContentType: "video/mp4",
			},
		},
		{
			name: "valid mov",
			json: `{"filename":"video.mov","content_type":"video/quicktime"}`,
			expected: UploadURLRequest{
				Filename:    "video.mov",
				ContentType: "video/quicktime",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req UploadURLRequest
			err := json.Unmarshal([]byte(tt.json), &req)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, req)
		})
	}
}

func TestUploadURLResponseMarshaling(t *testing.T) {
	resp := UploadURLResponse{
		VideoID:   "550e8400-e29b-41d4-a716-446655440000",
		SignedURL: "https://storage.googleapis.com/bucket/path?signature=...",
		ExpiresIn: 900,
	}

	data, err := json.Marshal(resp)
	assert.NoError(t, err)

	var decoded UploadURLResponse
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, resp, decoded)
}

func TestTranscodeRequestMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected TranscodeRequest
	}{
		{
			name: "web-hls preset",
			json: `{"preset":"preset/web-hls"}`,
			expected: TranscodeRequest{
				Preset: "preset/web-hls",
			},
		},
		{
			name: "empty preset",
			json: `{"preset":""}`,
			expected: TranscodeRequest{
				Preset: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req TranscodeRequest
			err := json.Unmarshal([]byte(tt.json), &req)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, req)
		})
	}
}

func TestTranscodeResponseMarshaling(t *testing.T) {
	resp := TranscodeResponse{
		JobID:   "projects/my-project/locations/us-central1/jobs/1234567890",
		State:   "PENDING",
		Message: "Transcoding job submitted successfully",
	}

	data, err := json.Marshal(resp)
	assert.NoError(t, err)

	var decoded TranscodeResponse
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, resp, decoded)
}

func TestTranscodeStatusResponseMarshaling(t *testing.T) {
	resp := TranscodeStatusResponse{
		JobID:    "projects/my-project/locations/us-central1/jobs/1234567890",
		State:    "RUNNING",
		Progress: 45,
		Message:  "",
	}

	data, err := json.Marshal(resp)
	assert.NoError(t, err)

	var decoded TranscodeStatusResponse
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, resp, decoded)
}

func TestJobStates(t *testing.T) {
	validStates := []string{
		"PENDING",
		"RUNNING",
		"SUCCEEDED",
		"FAILED",
	}

	for _, state := range validStates {
		resp := TranscodeStatusResponse{
			JobID: "test-job",
			State: state,
		}
		assert.Equal(t, state, resp.State)
	}
}
