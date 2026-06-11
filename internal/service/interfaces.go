package service

import (
	"context"

	transcoderpb "cloud.google.com/go/video/transcoder/apiv1/transcoderpb"
)

type GCSServiceInterface interface {
	GenerateSignedURL(ctx context.Context, filename, contentType string) (videoID, signedURL string, err error)
	GetSourceURI(videoID string) string
	Close() error
}

type TranscoderServiceInterface interface {
	CreateJob(ctx context.Context, videoID, sourceURI string) (string, error)
	GetJob(ctx context.Context, jobName string) (*transcoderpb.Job, error)
	Close() error
}
