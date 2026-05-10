package repository

import "context"

type MockTxManager struct {
	Users      UserRepository
	Audit      AuditRepository
	Committed  bool
	RolledBack bool
}

var _ TxManager = (*MockTxManager)(nil)

func NewMockTxManager(users UserRepository, audit AuditRepository) *MockTxManager {
	return &MockTxManager{Users: users, Audit: audit}
}

func (m *MockTxManager) WithinTx(ctx context.Context, fn func(context.Context, TxRepositories) error) error {
	err := fn(ctx, TxRepositories{Users: m.Users, Audit: m.Audit})
	if err != nil {
		m.RolledBack = true
		return err
	}
	m.Committed = true
	return nil
}
