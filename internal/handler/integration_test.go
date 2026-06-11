package handler

import (
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

func TestFullVideoWorkflow(t *testing.T) {
	mockGCS := new(mocks.MockGCSService)
	mockTranscoder := new(mocks.MockTranscoderService)

	videoID := "550e8400-e29b-41d4-a716-446655440000"
	jobID := "projects/my-project/locations/us-central1/jobs/1234567890"

	mockGCS.On(
		"GenerateSignedURL",
		mock.MatchedBy(func(ctx interface{}) bool { return true }),
		"video.mp4",
		"video/mp4",
	).Return(videoID, "https://storage.googleapis.com/bucket/...", nil)

	mockGCS.On("GetSourceURI", videoID).Return("gs://raw-bucket/550e8400-e29b-41d4-a716-446655440000/original.mp4")

	mockTranscoder.On(
		"CreateJob",
		mock.MatchedBy(func(ctx interface{}) bool { return true }),
		videoID,
		"gs://raw-bucket/550e8400-e29b-41d4-a716-446655440000/original.mp4",
	).Return(jobID, nil)

	mockTranscoder.On(
		"GetJob",
		mock.MatchedBy(func(ctx interface{}) bool { return true }),
		jobID,
	).Return(&transcoderpb.Job{
		Name:      jobID,
		State:     transcoderpb.Job_SUCCEEDED,
		StartTime: timestamppb.Now(),
	}, nil)

	handler := NewVideoHandler(mockGCS, mockTranscoder)
	e := echo.New()

	// Step 1: Generate upload URL
	req1 := httptest.NewRequest(
		http.MethodPost,
		"/videos/upload-url",
		strings.NewReader(`{"filename":"video.mp4","content_type":"video/mp4"}`),
	)
	req1.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)

	err := handler.GenerateUploadURL(c1)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec1.Code)

	var uploadResp model.UploadURLResponse
	err = json.Unmarshal(rec1.Body.Bytes(), &uploadResp)
	assert.NoError(t, err)
	assert.Equal(t, videoID, uploadResp.VideoID)

	// Step 2: Trigger transcoding
	req2 := httptest.NewRequest(
		http.MethodPost,
		"/videos/"+videoID+"/transcode",
		strings.NewReader(`{}`),
	)
	req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("video_id")
	c2.SetParamValues(videoID)

	err = handler.StartTranscode(c2)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec2.Code)

	var transcodeResp model.TranscodeResponse
	err = json.Unmarshal(rec2.Body.Bytes(), &transcodeResp)
	assert.NoError(t, err)
	assert.Equal(t, jobID, transcodeResp.JobID)

	// Step 3: Check status
	req3 := httptest.NewRequest(
		http.MethodGet,
		"/videos/transcode/"+jobID+"/status",
		nil,
	)
	rec3 := httptest.NewRecorder()
	c3 := e.NewContext(req3, rec3)
	c3.SetParamNames("job_id")
	c3.SetParamValues(jobID)

	err = handler.GetTranscodeStatus(c3)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec3.Code)

	var statusResp model.TranscodeStatusResponse
	err = json.Unmarshal(rec3.Body.Bytes(), &statusResp)
	assert.NoError(t, err)
	assert.Equal(t, jobID, statusResp.JobID)
	assert.Equal(t, "SUCCEEDED", statusResp.State)

	mockGCS.AssertExpectations(t)
	mockTranscoder.AssertExpectations(t)
}

func TestErrorHandling(t *testing.T) {
	mockGCS := new(mocks.MockGCSService)
	mockTranscoder := new(mocks.MockTranscoderService)

	mockGCS.On(
		"GenerateSignedURL",
		mock.MatchedBy(func(ctx interface{}) bool { return true }),
		"video.mp4",
		"video/mp4",
	).Return("", "", assert.AnError)

	handler := NewVideoHandler(mockGCS, mockTranscoder)
	e := echo.New()

	req := httptest.NewRequest(
		http.MethodPost,
		"/videos/upload-url",
		strings.NewReader(`{"filename":"video.mp4","content_type":"video/mp4"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.GenerateUploadURL(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp map[string]string
	err = json.Unmarshal(rec.Body.Bytes(), &errResp)
	assert.NoError(t, err)
	assert.NotEmpty(t, errResp["error"])

	mockGCS.AssertExpectations(t)
}
