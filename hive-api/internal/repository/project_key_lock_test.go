package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCanonicalProjectKeysSortsDedupesAndDropsEmpty(t *testing.T) {
	got := CanonicalProjectKeys([]string{
		" Beta Project ",
		"alpha/project",
		"beta-project",
		"",
		"Alpha Project",
	})

	require.Equal(t, []string{"alpha-project", "beta-project"}, got)
}

func TestPostgresProjectKeyLockRepository_TransactionLockBlocksSameCanonicalKey(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx1, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx1.Rollback(ctx) //nolint:errcheck

	locks1 := newPostgresProjectKeyLockRepositoryWithQuerier(tx1)
	require.NoError(t, locks1.LockCanonicalProjectKeys(ctx, []string{"jarvis-dev"}))

	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx2.Rollback(ctx) //nolint:errcheck

	start := time.Now()
	err = newPostgresProjectKeyLockRepositoryWithQuerier(tx2).LockCanonicalProjectKeys(ctx, []string{"Jarvis Dev"})
	require.ErrorIs(t, err, ErrProjectKeyLockBusy)
	require.Less(t, time.Since(start), time.Second, "lock acquisition must be bounded, not wait for the first transaction to commit")

	require.NoError(t, tx1.Commit(ctx))
}

func TestPostgresProjectKeyLockRepository_ReversedOrderDoesNotDeadlock(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx1, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx1.Rollback(ctx) //nolint:errcheck
	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx2.Rollback(ctx) //nolint:errcheck

	start := make(chan struct{})
	type lockResult struct {
		tx  int
		err error
	}
	done := make(chan lockResult, 2)
	go func() {
		<-start
		done <- lockResult{tx: 1, err: newPostgresProjectKeyLockRepositoryWithQuerier(tx1).LockCanonicalProjectKeys(ctx, []string{"beta", "alpha"})}
	}()
	go func() {
		<-start
		done <- lockResult{tx: 2, err: newPostgresProjectKeyLockRepositoryWithQuerier(tx2).LockCanonicalProjectKeys(ctx, []string{"alpha", "beta"})}
	}()

	close(start)
	first := <-done
	require.NoError(t, first.err)
	if first.tx == 1 {
		require.NoError(t, tx1.Commit(ctx))
	} else {
		require.NoError(t, tx2.Commit(ctx))
	}
	require.NoError(t, (<-done).err)
}
