package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type pgxQuerier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type TxRepositories struct {
	Users UserRepository
	Audit AuditRepository
}

type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, repos TxRepositories) error) error
}
