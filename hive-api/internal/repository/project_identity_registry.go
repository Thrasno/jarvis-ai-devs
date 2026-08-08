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
// The sync path canonicalizes payload projects before storing them, so rows
// carry the canonical key as their literal project value; without its own
// spelling row that value would stay unresolvable in the registry. Canonical
// keys are idempotent, so registering one as a spelling is always sound.
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
	if err := coalesceCanonicalProjectBlocks(ctx, tx); err != nil {
		return err
	}
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
		if _, err := tx.Exec(ctx, `
			UPDATE project_blocks
			SET canonical_project_key = $1
			WHERE project = $2 AND canonical_project_key <> $1`, key, registration.spelling); err != nil {
			return wrapPgError(err, "canonicalize project block identity")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return wrapPgError(err, "commit project identity registry backfill")
	}
	return nil
}

type legacyProjectBlockHead struct {
	id, canonicalProjectKey string
	generation              int64
	action, reason          string
	confirmation, marker    string
	actorUserID             string
	blocked                 bool
}

// coalesceCanonicalProjectBlocks folds legacy block heads that the shared Go
// identity contract maps to one key. A later generation is an unambiguous
// successor; equal generations are accepted only when their current state is
// semantically identical, otherwise the entire startup transaction is rejected.
func coalesceCanonicalProjectBlocks(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
		SELECT id::text, canonical_project_key, generation, action, reason,
		       confirmation, export_marker, COALESCE(actor_user_id, ''), blocked
		FROM project_blocks
		ORDER BY canonical_project_key ASC, id ASC`)
	if err != nil {
		return wrapPgError(err, "read legacy project block heads")
	}
	defer rows.Close()

	groups := make(map[string][]legacyProjectBlockHead)
	for rows.Next() {
		var head legacyProjectBlockHead
		if err := rows.Scan(&head.id, &head.canonicalProjectKey, &head.generation,
			&head.action, &head.reason, &head.confirmation, &head.marker, &head.actorUserID, &head.blocked); err != nil {
			return wrapPgError(err, "scan legacy project block head")
		}
		key := projectkey.Canonicalize(head.canonicalProjectKey)
		if key != "" {
			groups[key] = append(groups[key], head)
		}
	}
	if err := rows.Err(); err != nil {
		return wrapPgError(err, "iterate legacy project block heads")
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		heads := groups[key]
		if len(heads) < 2 {
			continue
		}
		sort.Slice(heads, func(i, j int) bool {
			if heads[i].generation != heads[j].generation {
				return heads[i].generation > heads[j].generation
			}
			if heads[i].canonicalProjectKey != heads[j].canonicalProjectKey {
				return heads[i].canonicalProjectKey < heads[j].canonicalProjectKey
			}
			return heads[i].id < heads[j].id
		})
		winner := heads[0]
		for _, loser := range heads[1:] {
			if loser.generation == winner.generation && !sameProjectBlockState(winner, loser) {
				return fmt.Errorf("project identity conflict for %q: legacy block heads diverge at generation %d", key, winner.generation)
			}
			if err := mergeLegacyProjectBlock(ctx, tx, winner.canonicalProjectKey, loser.canonicalProjectKey, winner.generation); err != nil {
				return fmt.Errorf("project identity conflict for %q: %w", key, err)
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE project_blocks SET canonical_project_key = $1 WHERE canonical_project_key = $2`, key, winner.canonicalProjectKey); err != nil {
			return wrapPgError(err, "canonicalize coalesced project block")
		}
		if _, err := tx.Exec(ctx, `UPDATE project_quarantine_commands SET canonical_project_key = $1 WHERE canonical_project_key = $2`, key, winner.canonicalProjectKey); err != nil {
			return wrapPgError(err, "canonicalize coalesced quarantine commands")
		}
	}
	return nil
}

func sameProjectBlockState(left, right legacyProjectBlockHead) bool {
	return left.action == right.action && left.reason == right.reason &&
		left.confirmation == right.confirmation && left.marker == right.marker &&
		left.actorUserID == right.actorUserID && left.blocked == right.blocked
}

func mergeLegacyProjectBlock(ctx context.Context, tx pgx.Tx, winnerKey, loserKey string, winnerGeneration int64) error {
	for _, table := range []string{"project_block_acks", "project_block_ack_deliveries"} {
		var conflicts bool
		q := fmt.Sprintf(`SELECT EXISTS (
			SELECT 1 FROM %s loser JOIN %s winner
			  ON winner.command_id = loser.command_id
			 AND winner.ack_auth_subject = loser.ack_auth_subject
			 WHERE loser.canonical_project_key = $1 AND winner.canonical_project_key = $2)`, table, table)
		if err := tx.QueryRow(ctx, q, loserKey, winnerKey).Scan(&conflicts); err != nil {
			return wrapPgError(err, "check project block acknowledgement conflict")
		}
		if conflicts {
			return fmt.Errorf("%s acknowledgement identity overlaps", table)
		}
	}
	if err := reconcileLegacyProjectBlockCommands(ctx, tx, winnerKey, loserKey, winnerGeneration); err != nil {
		return err
	}
	for _, table := range []string{"project_block_acks", "project_block_ack_deliveries", "project_quarantine_commands"} {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET canonical_project_key = $1 WHERE canonical_project_key = $2`, table), winnerKey, loserKey); err != nil {
			return wrapPgError(err, "merge legacy project block records")
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM project_blocks WHERE canonical_project_key = $1`, loserKey); err != nil {
		return wrapPgError(err, "remove merged legacy project block head")
	}
	return nil
}

func reconcileLegacyProjectBlockCommands(ctx context.Context, tx pgx.Tx, winnerKey, loserKey string, winnerGeneration int64) error {
	if err := tx.QueryRow(ctx, `SELECT generation FROM project_blocks WHERE canonical_project_key = $1`, winnerKey).Scan(&winnerGeneration); err != nil {
		return wrapPgError(err, "read surviving project block generation")
	}
	var exceedsHead bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM project_quarantine_commands
		WHERE canonical_project_key IN ($1, $2) AND generation > $3)`, winnerKey, loserKey, winnerGeneration).Scan(&exceedsHead); err != nil {
		return wrapPgError(err, "check project quarantine command generations")
	}
	if exceedsHead {
		return fmt.Errorf("quarantine command generation exceeds surviving head")
	}

	rows, err := tx.Query(ctx, `
		SELECT loser.command_id::text
		FROM project_quarantine_commands loser
		JOIN project_quarantine_commands winner ON winner.generation = loser.generation
		WHERE loser.canonical_project_key = $1 AND winner.canonical_project_key = $2
		ORDER BY loser.generation ASC, loser.command_id ASC`, loserKey, winnerKey)
	if err != nil {
		return wrapPgError(err, "read overlapping project quarantine commands")
	}
	commandIDs := make([]string, 0)
	for rows.Next() {
		var commandID string
		if err := rows.Scan(&commandID); err != nil {
			rows.Close()
			return wrapPgError(err, "scan overlapping project quarantine command")
		}
		commandIDs = append(commandIDs, commandID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return wrapPgError(err, "iterate overlapping project quarantine commands")
	}
	rows.Close()
	for _, commandID := range commandIDs {
		var generation int64
		if err := tx.QueryRow(ctx, `
			SELECT candidate.generation
			FROM generate_series(1, $3) AS candidate(generation)
			WHERE NOT EXISTS (
				SELECT 1 FROM project_quarantine_commands
				WHERE canonical_project_key IN ($1, $2) AND generation = candidate.generation)
			ORDER BY candidate.generation ASC
			LIMIT 1`, winnerKey, loserKey, winnerGeneration).Scan(&generation); err != nil {
			if err == pgx.ErrNoRows {
				if err := tx.QueryRow(ctx, `SELECT COALESCE(max(generation), 0) + 1 FROM project_quarantine_commands WHERE canonical_project_key IN ($1, $2)`, winnerKey, loserKey).Scan(&generation); err != nil {
					return wrapPgError(err, "allocate appended project quarantine command generation")
				}
				if _, err := tx.Exec(ctx, `UPDATE project_quarantine_commands SET generation = $1 WHERE command_id = $2::uuid AND canonical_project_key = $3`, generation, commandID, loserKey); err != nil {
					return wrapPgError(err, "append project quarantine command generation")
				}
				winnerGeneration = generation + 1
				if _, err := tx.Exec(ctx, `UPDATE project_blocks SET generation = $1 WHERE canonical_project_key = $2`, winnerGeneration, winnerKey); err != nil {
					return wrapPgError(err, "advance surviving project block generation")
				}
				continue
			}
			return wrapPgError(err, "allocate project quarantine command generation")
		}
		if _, err := tx.Exec(ctx, `UPDATE project_quarantine_commands SET generation = $1 WHERE command_id = $2::uuid AND canonical_project_key = $3`, generation, commandID, loserKey); err != nil {
			return wrapPgError(err, "reassign project quarantine command generation")
		}
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
