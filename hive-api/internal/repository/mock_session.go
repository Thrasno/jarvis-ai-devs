package repository

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/mock"
)

// MockSessionRepository implementa SessionRepository para tests de capa de servicio.
type MockSessionRepository struct {
	mock.Mock
}

var _ SessionRepository = (*MockSessionRepository)(nil)

func (m *MockSessionRepository) CreateSession(ctx context.Context, session *model.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *MockSessionRepository) UpsertSession(ctx context.Context, session *model.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *MockSessionRepository) EndSession(ctx context.Context, sessionID, summary string) error {
	args := m.Called(ctx, sessionID, summary)
	return args.Error(0)
}

func (m *MockSessionRepository) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Session), args.Error(1)
}

func (m *MockSessionRepository) EnsureManualSaveSession(ctx context.Context, project string) (string, error) {
	args := m.Called(ctx, project)
	return args.String(0), args.Error(1)
}

func (m *MockSessionRepository) ListSessionsSince(ctx context.Context, project string, since time.Time, cursor model.PullCursor, limit int) ([]*model.Session, bool, error) {
	args := m.Called(ctx, project, since, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).([]*model.Session), args.Bool(1), args.Error(2)
}
