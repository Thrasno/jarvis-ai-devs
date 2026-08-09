package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/projectkey"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type projectIdentityRegistration struct {
	spelling string
	seenAt   time.Time
}

type ProjectIdentityRepository interface {
	Register(ctx context.Context, spelling, remoteSpelling string, seenAt time.Time) error
}

type postgresProjectIdentityRepository struct{ db pgxQuerier }

func newPostgresProjectIdentityRepositoryWithQuerier(db pgxQuerier) ProjectIdentityRepository {
	return &postgresProjectIdentityRepository{db: db}
}

func (r *postgresProjectIdentityRepository) Register(ctx context.Context, spelling, remoteSpelling string, seenAt time.Time) error {
	return registerProjectIdentity(ctx, r.db, spelling, remoteSpelling, seenAt)
}

// RegisterProjectIdentity records a new API-facing spelling without changing
// legacy project columns. A current remote spelling wins display precedence;
// otherwise the earliest observed spelling remains the fallback.
//
// Both the observed spelling and the canonical key it folds to are registered.
// The registry answers "has the API ever seen this project?", grouping the
// spellings a human would call the same project; it is NOT how rows are
// selected. Reads and quarantine match the stored literal with plain equality
// and never consult it. Canonical keys are idempotent, so registering one as a
// spelling is always sound.
func RegisterProjectIdentity(ctx context.Context, pool *pgxpool.Pool, spelling, remoteSpelling string, seenAt time.Time) error {
	return registerProjectIdentity(ctx, pool, spelling, remoteSpelling, seenAt)
}

func registerProjectIdentity(ctx context.Context, db pgxQuerier, spelling, remoteSpelling string, seenAt time.Time) error {
	key := projectkey.Canonicalize(spelling)
	if key == "" {
		return fmt.Errorf("canonical project key is required")
	}
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}
	remoteSpelling = strings.TrimSpace(remoteSpelling)
	_, err := db.Exec(ctx, `
		WITH registered AS (
		INSERT INTO project_identities (project_key, first_spelling, first_seen_at, remote_spelling, remote_seen_at)
		VALUES ($1, $2, $3::timestamptz, NULLIF($4, ''), CASE WHEN $4 = '' THEN NULL ELSE $3::timestamptz END)
		ON CONFLICT (project_key) DO UPDATE SET
			first_spelling = CASE
				WHEN EXCLUDED.first_seen_at < project_identities.first_seen_at
					OR (EXCLUDED.first_seen_at = project_identities.first_seen_at AND EXCLUDED.first_spelling < project_identities.first_spelling)
				THEN EXCLUDED.first_spelling ELSE project_identities.first_spelling END,
			first_seen_at = LEAST(project_identities.first_seen_at, EXCLUDED.first_seen_at),
			remote_spelling = COALESCE(EXCLUDED.remote_spelling, project_identities.remote_spelling),
			remote_seen_at = COALESCE(EXCLUDED.remote_seen_at, project_identities.remote_seen_at),
			updated_at = now()
		RETURNING project_key)
		INSERT INTO project_identity_spellings (spelling, project_key)
		SELECT DISTINCT observed.spelling, registered.project_key
		FROM registered, unnest(ARRAY[$2, $1]) AS observed(spelling)
		ON CONFLICT (spelling) DO UPDATE SET project_key = EXCLUDED.project_key`, key, spelling, seenAt, remoteSpelling)
	if err != nil {
		return wrapPgError(err, "register project identity")
	}
	return nil
}

// BackfillProjectIdentityRegistry records every legacy project spelling under
// the shared canonical key without changing legacy project columns.
func BackfillProjectIdentityRegistry(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return wrapPgError(err, "begin project identity registry backfill")
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	registrations, err := readProjectIdentityRegistrations(ctx, tx)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(registrations))
	for _, registration := range registrations {
		keys = append(keys, registration.spelling)
	}
	if err := newPostgresProjectKeyLockRepositoryWithQuerier(tx).LockCanonicalProjectKeys(ctx, keys); err != nil {
		return err
	}
	// The backfill populates the spelling registry only. It must never touch
	// project_blocks: quarantine matches a stored literal with plain equality, so
	// rewriting a block's key here would silently repoint it at another project
	// (or at none), and folding two block heads together would quarantine a
	// project no admin named.
	for _, registration := range registrations {
		key := projectkey.Canonicalize(registration.spelling)
		if _, err := tx.Exec(ctx, `
			INSERT INTO project_identities (project_key, first_spelling, first_seen_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (project_key) DO NOTHING`,
			key, registration.spelling, registration.seenAt); err != nil {
			return wrapPgError(err, "register canonical project identity")
		}
		if _, err := tx.Exec(ctx, `
				INSERT INTO project_identity_spellings (spelling, project_key)
				VALUES ($1, $2)
				ON CONFLICT (spelling) DO UPDATE SET project_key = EXCLUDED.project_key`,
			registration.spelling, key); err != nil {
			return wrapPgError(err, "register project identity spelling")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return wrapPgError(err, "commit project identity registry backfill")
	}
	return nil
}

func readProjectIdentityRegistrations(ctx context.Context, tx pgx.Tx) ([]projectIdentityRegistration, error) {
	const query = `
		SELECT project, seen_at FROM (
			SELECT project, created_at AS seen_at, id::text AS stable_id FROM memories
			UNION ALL SELECT project, created_at, id::text FROM sessions
			UNION ALL SELECT project, created_at, id::text FROM user_prompts
			UNION ALL SELECT project, created_at, sequence::text FROM memory_mutations
			UNION ALL SELECT project, updated_at, consumer_id FROM mutation_cursors
			UNION ALL SELECT project, started_at, id::text FROM sync_attempt_logs
			UNION ALL SELECT project, created_at, id::text FROM project_blocks
			UNION ALL SELECT project, created_at, command_id::text FROM project_quarantine_commands
		) registrations
		WHERE btrim(project) <> ''
		ORDER BY seen_at ASC, project ASC, stable_id ASC`
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return nil, wrapPgError(err, "read legacy project registrations")
	}
	defer rows.Close()

	registrations := make([]projectIdentityRegistration, 0)
	for rows.Next() {
		var registration projectIdentityRegistration
		if err := rows.Scan(&registration.spelling, &registration.seenAt); err != nil {
			return nil, wrapPgError(err, "scan legacy project registration")
		}
		if projectkey.Canonicalize(registration.spelling) != "" {
			registrations = append(registrations, registration)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, wrapPgError(err, "iterate legacy project registrations")
	}
	sort.SliceStable(registrations, func(i, j int) bool {
		left, right := projectkey.Canonicalize(registrations[i].spelling), projectkey.Canonicalize(registrations[j].spelling)
		if left != right {
			return left < right
		}
		return registrations[i].seenAt.Before(registrations[j].seenAt)
	})
	return registrations, nil
}
