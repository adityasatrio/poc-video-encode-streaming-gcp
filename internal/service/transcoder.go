package service

import (
	"context"
	"fmt"

	transcoder "cloud.google.com/go/video/transcoder/apiv1"
	transcoderpb "cloud.google.com/go/video/transcoder/apiv1/transcoderpb"
)

type TranscoderService struct {
	client    *transcoder.Client
	projectID string
	location  string
	hlsBucket string
}

var _ TranscoderServiceInterface = (*TranscoderService)(nil)

func NewTranscoderService(ctx context.Context, projectID, location, hlsBucket string) (*TranscoderService, error) {
	client, err := transcoder.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create transcoder client: %w", err)
	}

	return &TranscoderService{
		client:    client,
		projectID: projectID,
		location:  location,
		hlsBucket: hlsBucket,
	}, nil
}

func (s *TranscoderService) Close() error {
	return s.client.Close()
}

func (s *TranscoderService) CreateJob(ctx context.Context, videoID, sourceURI string) (jobID string, err error) {
	outputURI := fmt.Sprintf("gs://%s/%s/", s.hlsBucket, videoID)
	parent := fmt.Sprintf("projects/%s/locations/%s", s.projectID, s.location)

	req := &transcoderpb.CreateJobRequest{
		Parent: parent,
		Job: &transcoderpb.Job{
			InputUri:  sourceURI,
			OutputUri: outputURI,
			JobConfig: &transcoderpb.Job_TemplateId{
				TemplateId: "preset/web-hls",
			},
		},
	}

	resp, err := s.client.CreateJob(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create transcoder job: %w", err)
	}

	return resp.Name, nil
}

func (s *TranscoderService) GetJob(ctx context.Context, jobName string) (*transcoderpb.Job, error) {
	req := &transcoderpb.GetJobRequest{
		Name: jobName,
	}

	job, err := s.client.GetJob(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get job status: %w", err)
	}

	return job, nil
}
