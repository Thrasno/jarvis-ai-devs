package repository

import (
	"context"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/stretchr/testify/mock"
)

type MockAuditRepository struct {
	mock.Mock
}

var _ AuditRepository = (*MockAuditRepository)(nil)

func (m *MockAuditRepository) Insert(ctx context.Context, entry *model.AuditEntry) error {
	return m.Called(ctx, entry).Error(0)
}

func (m *MockAuditRepository) List(ctx context.Context, filter model.AuditFilter) ([]*model.AuditEntry, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.AuditEntry), args.Error(1)
}

func (m *MockAuditRepository) Count(ctx context.Context, filter model.AuditFilter) (int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).(int64), args.Error(1)
}
