package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresTxManager struct {
	pool *pgxpool.Pool
}

func NewPostgresTxManager(pool *pgxpool.Pool) TxManager {
	return &postgresTxManager{pool: pool}
}

func (m *postgresTxManager) WithinTx(ctx context.Context, fn func(context.Context, TxRepositories) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return wrapPgError(err, "begin transaction")
	}

	repos := TxRepositories{
		Users: newPostgresUserRepositoryWithQuerier(tx),
		Audit: newPostgresAuditRepositoryWithQuerier(tx),
	}
	if err := fn(ctx, repos); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return wrapPgError(tx.Commit(ctx), "commit transaction")
}
