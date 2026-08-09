package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

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

// RegisterProjectIdentity records a project literal the API has observed,
// without changing legacy project columns. A current remote spelling wins
// display precedence.
//
// The registry is keyed by the literal, so it is a table of project names, not
// a canonical grouping. It answers exactly one question — has the API ever seen
// THIS project? — and that answer now agrees with what a caller can read, since
// rows are selected by the same plain equality.
//
// It was keyed canonically, which grouped spellings a human might call the same
// project. That grouping was not the API's to make: the daemon is the sole
// authority on project identity. It also made the registry a live identity fold
// that a read query could join against, which is precisely how one project came
// to read another's rows.
func RegisterProjectIdentity(ctx context.Context, pool *pgxpool.Pool, spelling, remoteSpelling string, seenAt time.Time) error {
	return registerProjectIdentity(ctx, pool, spelling, remoteSpelling, seenAt)
}

func registerProjectIdentity(ctx context.Context, db pgxQuerier, spelling, remoteSpelling string, seenAt time.Time) error {
	if strings.TrimSpace(spelling) == "" {
		return fmt.Errorf("project spelling is required")
	}
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}
	remoteSpelling = strings.TrimSpace(remoteSpelling)
	_, err := db.Exec(ctx, `
		INSERT INTO project_identities (project_key, first_spelling, first_seen_at, remote_spelling, remote_seen_at)
		VALUES ($1, $1, $2::timestamptz, NULLIF($3, ''), CASE WHEN $3 = '' THEN NULL ELSE $2::timestamptz END)
		ON CONFLICT (project_key) DO UPDATE SET
			first_seen_at = LEAST(project_identities.first_seen_at, EXCLUDED.first_seen_at),
			remote_spelling = COALESCE(EXCLUDED.remote_spelling, project_identities.remote_spelling),
			remote_seen_at = COALESCE(EXCLUDED.remote_seen_at, project_identities.remote_seen_at),
			updated_at = now()`, spelling, seenAt, remoteSpelling)
	if err != nil {
		return wrapPgError(err, "register project identity")
	}
	return nil
}

// BackfillProjectIdentityRegistry records every legacy project literal the
// existing rows carry, without changing legacy project columns. Each literal is
// its own registry key, so a project whose rows predate the registry stays
// readable through the exact spelling those rows store.
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
	if err := newPostgresProjectKeyLockRepositoryWithQuerier(tx).LockProjectKeys(ctx, keys); err != nil {
		return err
	}
	// The backfill populates the identity registry only. It must never touch
	// project_blocks: quarantine matches a stored literal with plain equality, so
	// rewriting a block's key here would silently repoint it at another project
	// (or at none) on every boot. Migration 020 is the one-time correction for
	// blocks written before the exact-equality contract.
	for _, registration := range registrations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO project_identities (project_key, first_spelling, first_seen_at)
			VALUES ($1, $1, $2)
			ON CONFLICT (project_key) DO NOTHING`,
			registration.spelling, registration.seenAt); err != nil {
			return wrapPgError(err, "register project identity")
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
		registrations = append(registrations, registration)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapPgError(err, "iterate legacy project registrations")
	}
	sort.SliceStable(registrations, func(i, j int) bool {
		if registrations[i].spelling != registrations[j].spelling {
			return registrations[i].spelling < registrations[j].spelling
		}
		return registrations[i].seenAt.Before(registrations[j].seenAt)
	})
	return registrations, nil
}
