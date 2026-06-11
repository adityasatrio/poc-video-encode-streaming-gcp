package service

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
)

type GCSService struct {
	client    *storage.Client
	rawBucket string
	projectID string
}

var _ GCSServiceInterface = (*GCSService)(nil)

func NewGCSService(ctx context.Context, projectID, rawBucket string) (*GCSService, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	return &GCSService{
		client:    client,
		rawBucket: rawBucket,
		projectID: projectID,
	}, nil
}

func (s *GCSService) Close() error {
	return s.client.Close()
}

func (s *GCSService) GenerateSignedURL(ctx context.Context, filename string, contentType string) (videoID, signedURL string, err error) {
	videoID = uuid.New().String()
	objectPath := fmt.Sprintf("%s/original.mp4", videoID)

	opts := &storage.SignedURLOptions{
		Scheme:      storage.SigningSchemeV4,
		Method:      "PUT",
		Expires:     time.Now().Add(15 * time.Minute),
		ContentType: contentType,
	}

	signedURL, err = storage.SignedURL(s.rawBucket, objectPath, opts)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return videoID, signedURL, nil
}

func (s *GCSService) GetSourceURI(videoID string) string {
	return fmt.Sprintf("gs://%s/%s/original.mp4", s.rawBucket, videoID)
}
