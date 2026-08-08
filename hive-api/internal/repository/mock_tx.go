package repository

import (
	"context"
	"time"
)

type noopProjectIdentityRepository struct{}

func (noopProjectIdentityRepository) Register(context.Context, string, string, time.Time) error {
	return nil
}

type MockTxManager struct {
	Users             UserRepository
	Audit             AuditRepository
	ProjectBlocks     ProjectBlockRepository
	Memory            MemoryRepository
	Prompt            PromptRepository
	Session           SessionRepository
	ProjectKeyLocks   ProjectKeyLockRepository
	ProjectIdentities ProjectIdentityRepository
	Committed         bool
	RolledBack        bool
}

var _ TxManager = (*MockTxManager)(nil)

func NewMockTxManager(users UserRepository, audit AuditRepository) *MockTxManager {
	return &MockTxManager{Users: users, Audit: audit}
}

func (m *MockTxManager) WithinTx(ctx context.Context, fn func(context.Context, TxRepositories) error) error {
	if m.ProjectIdentities == nil {
		m.ProjectIdentities = noopProjectIdentityRepository{}
	}
	err := fn(ctx, TxRepositories{Users: m.Users, Audit: m.Audit, ProjectBlocks: m.ProjectBlocks, Memory: m.Memory, Prompt: m.Prompt, Session: m.Session, ProjectKeyLocks: m.ProjectKeyLocks, ProjectIdentities: m.ProjectIdentities})
	if err != nil {
		m.RolledBack = true
		return err
	}
	m.Committed = true
	return nil
}

func (m *MockTxManager) ReadOnlyRepeatableRead(ctx context.Context, fn func(context.Context, TxRepositories) error) error {
	return fn(ctx, TxRepositories{Users: m.Users, Audit: m.Audit, ProjectBlocks: m.ProjectBlocks, Memory: m.Memory, Prompt: m.Prompt, Session: m.Session, ProjectKeyLocks: m.ProjectKeyLocks})
}
