package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"
)

type MockGCSService struct {
	mock.Mock
}

func (m *MockGCSService) GenerateSignedURL(ctx context.Context, filename, contentType string) (videoID, signedURL string, err error) {
	args := m.Called(ctx, filename, contentType)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *MockGCSService) GetSourceURI(videoID string) string {
	args := m.Called(videoID)
	return args.String(0)
}

func (m *MockGCSService) Close() error {
	args := m.Called()
	return args.Error(0)
}
