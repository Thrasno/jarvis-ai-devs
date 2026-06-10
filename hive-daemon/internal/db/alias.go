package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrAliasSourceEqualsTarget is returned when source and target are the same.
var ErrAliasSourceEqualsTarget = errors.New("alias source and target must differ")

// ErrAliasTargetIsSource is returned when the requested target is already a
// source in an existing alias (cycle guard).
var ErrAliasTargetIsSource = errors.New("alias target is already a source; chained aliases are not allowed")

// ErrAliasDuplicateSource is returned when the source already has an alias
// pointing to a different target.
var ErrAliasDuplicateSource = errors.New("alias source already redirects to a different target")

// ProjectAlias represents a single entry in the project_aliases table.
type ProjectAlias struct {
	SourceProject string
	TargetProject string
	Scope         string // "local" | "global"
	Reason        string
	CreatedAt     time.Time
	CreatedBy     string
	SyncedAt      *time.Time
}

// AddAlias creates a permanent source→target redirect. Guards:
//   - self-alias: source == target → error
//   - chain/cycle: target is already a source_project → error
//   - idempotent: same source→target re-insert is a no-op
//   - conflict: source→different target → error
func (d *DB) AddAlias(ctx context.Context, source, target, scope, reason string) error {
	source = strings.TrimSpace(source)
	target = strings.TrimSpace(target)
	if source == target {
		return ErrAliasSourceEqualsTarget
	}

	// Cycle guard: reject if target is already a source_project.
	var existing string
	err := d.sqlDB.QueryRowContext(ctx,
		`SELECT source_project FROM project_aliases WHERE source_project = ?`, target,
	).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("alias cycle check: %w", err)
	}
	if err == nil {
		// target exists as a source — would create a chain
		return ErrAliasTargetIsSource
	}

	createdBy := detectUsername()
	createdAt := time.Now().UTC().Format("2006-01-02 15:04:05")

	// Attempt idempotent upsert: on conflict (same source) do update only when
	// target matches. If target differs, the WHERE clause blocks the update and
	// RowsAffected == 0, which we detect as a conflict.
	result, err := d.sqlDB.ExecContext(ctx, `
INSERT INTO project_aliases (source_project, target_project, scope, reason, created_at, created_by)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(source_project) DO UPDATE SET
    target_project = excluded.target_project,
    scope          = excluded.scope,
    reason         = excluded.reason
WHERE project_aliases.target_project = excluded.target_project`,
		source, target, scope, reason, createdAt, createdBy,
	)
	if err != nil {
		return fmt.Errorf("add alias: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("add alias rows affected: %w", err)
	}
	if rows == 0 {
		// A row existed for this source with a different target — conflict.
		return ErrAliasDuplicateSource
	}
	return nil
}

// ResolveAlias returns the target_project for the given source_project if an
// alias exists. Resolution is intentionally single-hop; chains are not followed.
// Returns ("", false, nil) when no alias exists for the given project.
func (d *DB) ResolveAlias(ctx context.Context, project string) (string, bool, error) {
	var target string
	err := d.sqlDB.QueryRowContext(ctx,
		`SELECT target_project FROM project_aliases WHERE source_project = ?`, project,
	).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("resolve alias: %w", err)
	}
	return target, true, nil
}

// RemoveAlias hard-deletes the alias row for the given source_project. After
// removal, ResolveAlias will return ("", false, nil) for the same source.
// Removing a non-existent alias is a no-op.
func (d *DB) RemoveAlias(ctx context.Context, source string) error {
	_, err := d.sqlDB.ExecContext(ctx,
		`DELETE FROM project_aliases WHERE source_project = ?`, source,
	)
	if err != nil {
		return fmt.Errorf("remove alias: %w", err)
	}
	return nil
}

// ListAliases returns all alias rows ordered by source_project. This is an
// internal DB method; no CLI or TUI surface exposes it directly.
func (d *DB) ListAliases(ctx context.Context) ([]ProjectAlias, error) {
	rows, err := d.sqlDB.QueryContext(ctx, `
SELECT source_project, target_project, scope, reason, created_at, created_by, synced_at
FROM project_aliases
ORDER BY source_project`)
	if err != nil {
		return nil, fmt.Errorf("list aliases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var aliases []ProjectAlias
	for rows.Next() {
		var a ProjectAlias
		var createdAt string
		var syncedAt sql.NullString
		if err := rows.Scan(&a.SourceProject, &a.TargetProject, &a.Scope, &a.Reason, &createdAt, &a.CreatedBy, &syncedAt); err != nil {
			return nil, fmt.Errorf("scan alias: %w", err)
		}
		a.CreatedAt, _ = parseTimeStr(createdAt)
		if syncedAt.Valid && syncedAt.String != "" {
			t, _ := parseTimeStr(syncedAt.String)
			a.SyncedAt = &t
		}
		aliases = append(aliases, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate aliases: %w", err)
	}
	return aliases, nil
}

// addAliasTx is the transaction-scoped helper used by MergeGovernanceProject.
// It applies the same guards as AddAlias but operates within an existing *sql.Tx.
func addAliasTx(ctx context.Context, tx *sql.Tx, source, target, scope, reason string) error {
	source = strings.TrimSpace(source)
	target = strings.TrimSpace(target)
	if source == target {
		return ErrAliasSourceEqualsTarget
	}

	// Cycle guard: reject if target is already a source_project.
	var existing string
	err := tx.QueryRowContext(ctx,
		`SELECT source_project FROM project_aliases WHERE source_project = ?`, target,
	).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("alias cycle check (tx): %w", err)
	}
	if err == nil {
		return ErrAliasTargetIsSource
	}

	createdBy := detectUsername()
	createdAt := time.Now().UTC().Format("2006-01-02 15:04:05")

	result, err := tx.ExecContext(ctx, `
INSERT INTO project_aliases (source_project, target_project, scope, reason, created_at, created_by)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(source_project) DO UPDATE SET
    target_project = excluded.target_project,
    scope          = excluded.scope,
    reason         = excluded.reason
WHERE project_aliases.target_project = excluded.target_project`,
		source, target, scope, reason, createdAt, createdBy,
	)
	if err != nil {
		return fmt.Errorf("add alias (tx): %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("add alias rows affected (tx): %w", err)
	}
	if rows == 0 {
		return ErrAliasDuplicateSource
	}
	return nil
}
