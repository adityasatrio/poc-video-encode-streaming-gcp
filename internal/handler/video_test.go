package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cloud.google.com/go/video/transcoder/apiv1/transcoderpb"
	"github.com/adityasatrio/poc-video-encode-streaming-gcp/internal/mocks"
	"github.com/adityasatrio/poc-video-encode-streaming-gcp/internal/model"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestGenerateUploadURL(t *testing.T) {
	mockGCS := new(mocks.MockGCSService)
	mockTranscoder := new(mocks.MockTranscoderService)

	mockGCS.On(
		"GenerateSignedURL",
		mock.MatchedBy(func(ctx context.Context) bool { return true }),
		"intro.mp4",
		"video/mp4",
	).Return("550e8400-e29b-41d4-a716-446655440000", "https://storage.googleapis.com/bucket/...", nil)

	handler := NewVideoHandler(mockGCS, mockTranscoder)
	e := echo.New()

	req := httptest.NewRequest(
		http.MethodPost,
		"/videos/upload-url",
		strings.NewReader(`{"filename":"intro.mp4","content_type":"video/mp4"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	err := handler.GenerateUploadURL(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp model.UploadURLResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", resp.VideoID)
	assert.Equal(t, "https://storage.googleapis.com/bucket/...", resp.SignedURL)
	assert.Equal(t, 900, resp.ExpiresIn)

	mockGCS.AssertExpectations(t)
}

func TestGenerateUploadURLInvalidRequest(t *testing.T) {
	mockGCS := new(mocks.MockGCSService)
	mockTranscoder := new(mocks.MockTranscoderService)
	handler := NewVideoHandler(mockGCS, mockTranscoder)
	e := echo.New()

	req := httptest.NewRequest(
		http.MethodPost,
		"/videos/upload-url",
		strings.NewReader(`{"invalid": "json"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	err := handler.GenerateUploadURL(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestStartTranscode(t *testing.T) {
	mockGCS := new(mocks.MockGCSService)
	mockTranscoder := new(mocks.MockTranscoderService)

	videoID := "550e8400-e29b-41d4-a716-446655440000"
	jobID := "projects/my-project/locations/us-central1/jobs/1234567890"

	mockGCS.On("GetSourceURI", videoID).Return("gs://raw-bucket/550e8400-e29b-41d4-a716-446655440000/original.mp4")
	mockTranscoder.On(
		"CreateJob",
		mock.MatchedBy(func(ctx context.Context) bool { return true }),
		videoID,
		"gs://raw-bucket/550e8400-e29b-41d4-a716-446655440000/original.mp4",
	).Return(jobID, nil)

	handler := NewVideoHandler(mockGCS, mockTranscoder)
	e := echo.New()

	req := httptest.NewRequest(
		http.MethodPost,
		"/videos/"+videoID+"/transcode",
		strings.NewReader(`{"preset":"preset/web-hls"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	c.SetParamNames("video_id")
	c.SetParamValues(videoID)

	err := handler.StartTranscode(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp model.TranscodeResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, jobID, resp.JobID)
	assert.Equal(t, "PENDING", resp.State)

	mockGCS.AssertExpectations(t)
	mockTranscoder.AssertExpectations(t)
}

func TestStartTranscodeMissingVideoID(t *testing.T) {
	mockGCS := new(mocks.MockGCSService)
	mockTranscoder := new(mocks.MockTranscoderService)
	handler := NewVideoHandler(mockGCS, mockTranscoder)
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/videos//transcode", nil)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	c.SetParamNames("video_id")
	c.SetParamValues("")

	err := handler.StartTranscode(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetTranscodeStatus(t *testing.T) {
	mockGCS := new(mocks.MockGCSService)
	mockTranscoder := new(mocks.MockTranscoderService)

	jobID := "projects/my-project/locations/us-central1/jobs/1234567890"
	mockTranscoder.On(
		"GetJob",
		mock.MatchedBy(func(ctx context.Context) bool { return true }),
		jobID,
	).Return(&transcoderpb.Job{
		Name:      jobID,
		State:     transcoderpb.Job_RUNNING,
		StartTime: timestamppb.Now(),
	}, nil)

	handler := NewVideoHandler(mockGCS, mockTranscoder)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/videos/transcode/"+jobID+"/status", nil)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	c.SetParamNames("job_id")
	c.SetParamValues(jobID)

	err := handler.GetTranscodeStatus(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp model.TranscodeStatusResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, jobID, resp.JobID)
	assert.Equal(t, "RUNNING", resp.State)

	mockTranscoder.AssertExpectations(t)
}

func TestGetTranscodeStatusMissingJobID(t *testing.T) {
	mockGCS := new(mocks.MockGCSService)
	mockTranscoder := new(mocks.MockTranscoderService)
	handler := NewVideoHandler(mockGCS, mockTranscoder)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/videos/transcode//status", nil)
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	c.SetParamNames("job_id")
	c.SetParamValues("")

	err := handler.GetTranscodeStatus(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
