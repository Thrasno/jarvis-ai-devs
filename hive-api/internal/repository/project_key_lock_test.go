package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestProjectLockKeysSortsDedupesLiteralsAndDropsBlank pins that the lock scope
// is the project literal. It used to fold its input, which made distinct
// projects contend on one lock and, more importantly, kept a canonicalizer
// alive in a module that must never derive identity.
func TestProjectLockKeysSortsDedupesLiteralsAndDropsBlank(t *testing.T) {
	got := ProjectLockKeys([]string{
		" Beta Project ",
		"alpha/project",
		"beta-project",
		"",
		"   ",
		"Alpha Project",
		"alpha/project",
	})

	require.Equal(t, []string{" Beta Project ", "Alpha Project", "alpha/project", "beta-project"}, got)
}

func TestPostgresProjectKeyLockRepository_TransactionLockBlocksTheSameLiteral(t *testing.T) {
	pool, cleanup := startPostgresWithSessions(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx1, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx1.Rollback(ctx) //nolint:errcheck

	locks1 := newPostgresProjectKeyLockRepositoryWithQuerier(tx1)
	require.NoError(t, locks1.LockProjectKeys(ctx, []string{"jarvis-dev"}))

	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx2.Rollback(ctx) //nolint:errcheck

	locks2 := newPostgresProjectKeyLockRepositoryWithQuerier(tx2)
	require.NoError(t, locks2.LockProjectKeys(ctx, []string{"Jarvis Dev"}),
		"a different spelling is a different project and must not contend")

	start := time.Now()
	err = locks2.LockProjectKeys(ctx, []string{"jarvis-dev"})
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
		done <- lockResult{tx: 1, err: newPostgresProjectKeyLockRepositoryWithQuerier(tx1).LockProjectKeys(ctx, []string{"beta", "alpha"})}
	}()
	go func() {
		<-start
		done <- lockResult{tx: 2, err: newPostgresProjectKeyLockRepositoryWithQuerier(tx2).LockProjectKeys(ctx, []string{"alpha", "beta"})}
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
