package repository

import (
	"context"

	"github.com/stretchr/testify/mock"
)

type MockProjectKeyLockRepository struct {
	mock.Mock
}

var _ ProjectKeyLockRepository = (*MockProjectKeyLockRepository)(nil)

func (m *MockProjectKeyLockRepository) LockProjectKeys(ctx context.Context, canonicalKeys []string) error {
	return m.Called(ctx, canonicalKeys).Error(0)
}
