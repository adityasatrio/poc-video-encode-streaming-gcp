package mocks

import (
	"context"

	transcoderpb "cloud.google.com/go/video/transcoder/apiv1/transcoderpb"
	"github.com/stretchr/testify/mock"
)

type MockTranscoderService struct {
	mock.Mock
}

func (m *MockTranscoderService) CreateJob(ctx context.Context, videoID, sourceURI string) (string, error) {
	args := m.Called(ctx, videoID, sourceURI)
	return args.String(0), args.Error(1)
}

func (m *MockTranscoderService) GetJob(ctx context.Context, jobName string) (*transcoderpb.Job, error) {
	args := m.Called(ctx, jobName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*transcoderpb.Job), args.Error(1)
}

func (m *MockTranscoderService) Close() error {
	args := m.Called()
	return args.Error(0)
}
