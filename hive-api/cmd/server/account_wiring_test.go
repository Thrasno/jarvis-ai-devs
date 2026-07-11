package main

import (
	"context"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/handler"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type accountServiceStub struct{}

func (accountServiceStub) ChangePassword(context.Context, string, string, string) error { return nil }

func TestWireServices_WiresAccountService(t *testing.T) {
	userRepo := &repository.MockUserRepository{}
	auditRepo := &repository.MockAuditRepository{}
	factories := defaultServiceFactories()
	factories.newUserRepo = func(*pgxpool.Pool) repository.UserRepository { return userRepo }
	factories.newAuditRepo = func(*pgxpool.Pool) repository.AuditRepository { return auditRepo }
	factories.newTxManager = func(*pgxpool.Pool) repository.TxManager { return nil }
	factories.newAccountService = func(gotUsers repository.UserRepository, gotAudit repository.AuditRepository, gotTx repository.TxManager) handler.AccountService {
		require.Same(t, userRepo, gotUsers)
		require.Same(t, auditRepo, gotAudit)
		require.Nil(t, gotTx)
		return accountServiceStub{}
	}

	deps := wireServicesWithFactories(nil, &config.Config{}, factories)
	require.IsType(t, accountServiceStub{}, deps.accountSvc)
}
