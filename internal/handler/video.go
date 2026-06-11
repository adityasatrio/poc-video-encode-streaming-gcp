package handler

import (
	"net/http"

	"github.com/adityasatrio/poc-video-encode-streaming-gcp/internal/model"
	"github.com/adityasatrio/poc-video-encode-streaming-gcp/internal/service"
	"github.com/labstack/echo/v4"
)

type VideoHandler struct {
	gcsService       *service.GCSService
	transcoderService *service.TranscoderService
}

func NewVideoHandler(gcs *service.GCSService, transcoder *service.TranscoderService) *VideoHandler {
	return &VideoHandler{
		gcsService:        gcs,
		transcoderService: transcoder,
	}
}

// GenerateUploadURL creates a signed URL for client-side upload
func (h *VideoHandler) GenerateUploadURL(c echo.Context) error {
	var req model.UploadURLRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	videoID, signedURL, err := h.gcsService.GenerateSignedURL(c.Request().Context(), req.Filename, req.ContentType)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, model.UploadURLResponse{
		VideoID:   videoID,
		SignedURL: signedURL,
		ExpiresIn: 15 * 60, // 15 minutes
	})
}

// StartTranscode submits a transcoding job
func (h *VideoHandler) StartTranscode(c echo.Context) error {
	videoID := c.Param("video_id")
	if videoID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "video_id required"})
	}

	sourceURI := h.gcsService.GetSourceURI(videoID)
	jobID, err := h.transcoderService.CreateJob(c.Request().Context(), videoID, sourceURI)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, model.TranscodeResponse{
		JobID:   jobID,
		State:   "PENDING",
		Message: "Transcoding job submitted successfully",
	})
}

// GetTranscodeStatus returns the current transcoding job status
func (h *VideoHandler) GetTranscodeStatus(c echo.Context) error {
	jobID := c.Param("job_id")
	if jobID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "job_id required"})
	}

	job, err := h.transcoderService.GetJob(c.Request().Context(), jobID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	progress := int32(0)
	message := ""

	return c.JSON(http.StatusOK, model.TranscodeStatusResponse{
		JobID:    job.Name,
		State:    job.State.String(),
		Progress: progress,
		Message:  message,
	})
}
