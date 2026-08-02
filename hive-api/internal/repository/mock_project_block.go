package repository

import (
	"context"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/mock"
)

type MockProjectBlockRepository struct {
	mock.Mock
}

var _ ProjectBlockRepository = (*MockProjectBlockRepository)(nil)

func (m *MockProjectBlockRepository) BlockProject(ctx context.Context, create model.ProjectBlockCreate) (*model.ProjectBlock, error) {
	args := m.Called(ctx, create)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ProjectBlock), args.Error(1)
}

func (m *MockProjectBlockRepository) GetByCanonicalKey(ctx context.Context, canonicalProjectKey string) (*model.ProjectBlock, error) {
	args := m.Called(ctx, canonicalProjectKey)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ProjectBlock), args.Error(1)
}

func (m *MockProjectBlockRepository) ListInboxCommands(ctx context.Context, subject model.ProjectBlockAckSubject) ([]model.ProjectBlockCommand, error) {
	args := m.Called(ctx, subject)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.ProjectBlockCommand), args.Error(1)
}

func (m *MockProjectBlockRepository) EnsureAckDelivery(ctx context.Context, block *model.ProjectBlock, subject model.ProjectBlockAckSubject) (model.ProjectBlockCommand, error) {
	args := m.Called(ctx, block, subject)
	return args.Get(0).(model.ProjectBlockCommand), args.Error(1)
}

func (m *MockProjectBlockRepository) GetAckDelivery(ctx context.Context, canonicalProjectKey, commandID string, subject model.ProjectBlockAckSubject) (*model.ProjectBlockAckDelivery, error) {
	args := m.Called(ctx, canonicalProjectKey, commandID, subject)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ProjectBlockAckDelivery), args.Error(1)
}

func (m *MockProjectBlockRepository) RecordAck(ctx context.Context, ack model.ProjectBlockAck) (model.ProjectBlockAck, error) {
	args := m.Called(ctx, ack)
	return args.Get(0).(model.ProjectBlockAck), args.Error(1)
}

func (m *MockProjectBlockRepository) LatestAckForCommand(ctx context.Context, canonicalProjectKey, commandID string) (*model.ProjectBlockAck, error) {
	args := m.Called(ctx, canonicalProjectKey, commandID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ProjectBlockAck), args.Error(1)
}

func (m *MockProjectBlockRepository) QuarantineProgress(ctx context.Context, canonicalProjectKey string, generation int64, after string, limit int) (model.QuarantineProgressResponse, error) {
	args := m.Called(ctx, canonicalProjectKey, generation, after, limit)
	return args.Get(0).(model.QuarantineProgressResponse), args.Error(1)
}

func (m *MockProjectBlockRepository) ListQuarantines(ctx context.Context) ([]model.QuarantineSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.QuarantineSummary), args.Error(1)
}
