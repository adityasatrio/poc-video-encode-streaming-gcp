package model

type VideoID string

type UploadURLRequest struct {
	Filename    string `json:"filename" binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
}

type UploadURLResponse struct {
	VideoID   string `json:"video_id"`
	SignedURL string `json:"signed_url"`
	ExpiresIn int    `json:"expires_in"` // seconds
}

type TranscodeRequest struct {
	Preset string `json:"preset"` // e.g., "preset/web-hls" or custom
}

type TranscodeResponse struct {
	JobID   string `json:"job_id"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type TranscodeStatusResponse struct {
	JobID    string `json:"job_id"`
	State    string `json:"state"`
	Progress int32  `json:"progress"`
	Message  string `json:"message,omitempty"`
}
