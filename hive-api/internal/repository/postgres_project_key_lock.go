package repository

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"time"
)

const (
	projectKeyLockTimeout       = 750 * time.Millisecond
	projectKeyLockRetryInterval = 25 * time.Millisecond
)

type postgresProjectKeyLockRepository struct {
	db pgxQuerier
}

func newPostgresProjectKeyLockRepositoryWithQuerier(db pgxQuerier) ProjectKeyLockRepository {
	return &postgresProjectKeyLockRepository{db: db}
}

func (r *postgresProjectKeyLockRepository) LockCanonicalProjectKeys(ctx context.Context, canonicalKeys []string) error {
	ctx, cancel := context.WithTimeout(ctx, projectKeyLockTimeout)
	defer cancel()
	for _, canonical := range CanonicalProjectKeys(canonicalKeys) {
		lockID := projectKeyAdvisoryLockID(canonical)
		for {
			var acquired bool
			if err := r.db.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, lockID).Scan(&acquired); err != nil {
				if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) {
					return ErrProjectKeyLockBusy
				}
				return wrapPgError(err, "lock project key")
			}
			if acquired {
				break
			}
			timer := time.NewTimer(projectKeyLockRetryInterval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return ErrProjectKeyLockBusy
			case <-timer.C:
			}
		}
	}
	return nil
}

func projectKeyAdvisoryLockID(canonical string) int64 {
	sum := sha256.Sum256([]byte("project-key:" + canonical))
	return int64(binary.BigEndian.Uint64(sum[:8]))
}
