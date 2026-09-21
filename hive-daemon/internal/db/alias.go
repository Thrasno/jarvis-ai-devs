package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// projectLifecycle is process-local because one daemon owns its SQLite file.
// Read leases cover a complete sync network cycle; write leases cover a project
// promotion before its SQLite transaction begins. Keys are ordered to avoid
// source/target lock inversion.
var projectLifecycle = struct {
	mu    sync.Mutex
	locks map[string]*sync.RWMutex
}{locks: make(map[string]*sync.RWMutex)}

// AcquireProjectLifecycleRead serializes a sync cycle with promotion.
func AcquireProjectLifecycleRead(projects ...string) func() {
	return acquireProjectLifecycle(false, projects...)
}

// AcquireProjectLifecycleWrite serializes promotion with every sync cycle for
// the source and target keys.
func AcquireProjectLifecycleWrite(projects ...string) func() {
	return acquireProjectLifecycle(true, projects...)
}

func acquireProjectLifecycle(write bool, projects ...string) func() {
	keys := make([]string, 0, len(projects))
	seen := map[string]bool{}
	for _, project := range projects {
		project = canonicalProjectKey(project)
		if project != "" && !seen[project] {
			seen[project] = true
			keys = append(keys, project)
		}
	}
	sort.Strings(keys)
	projectLifecycle.mu.Lock()
	locks := make([]*sync.RWMutex, 0, len(keys))
	for _, key := range keys {
		lock := projectLifecycle.locks[key]
		if lock == nil {
			lock = &sync.RWMutex{}
			projectLifecycle.locks[key] = lock
		}
		locks = append(locks, lock)
	}
	projectLifecycle.mu.Unlock()
	for _, lock := range locks {
		if write {
			lock.Lock()
		} else {
			lock.RLock()
		}
	}
	return func() {
		for i := len(locks) - 1; i >= 0; i-- {
			if write {
				locks[i].Unlock()
			} else {
				locks[i].RUnlock()
			}
		}
	}
}

// ErrAliasSourceEqualsTarget is returned when source and target are the same.
var ErrAliasSourceEqualsTarget = errors.New("alias source and target must differ")

// ErrAliasTargetIsSource is returned when the requested target is already a
// source in an existing alias (cycle guard).
var ErrAliasTargetIsSource = errors.New("alias target is already a source; chained aliases are not allowed")

// ErrAliasDuplicateSource is returned when the source already has an alias
// pointing to a different target.
var ErrAliasDuplicateSource = errors.New("alias source already redirects to a different target")

// ErrAliasSourceIsTarget is returned when the proposed source is already a
// target in an existing alias (bidirectional chain guard).
var ErrAliasSourceIsTarget = errors.New("alias source is already a target in an existing alias; chained aliases are not allowed")

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
	release := AcquireProjectLifecycleWrite(source, target)
	defer release()

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

	// Bidirectional chain guard: reject if source is already a target_project.
	var existingSource string
	err = d.sqlDB.QueryRowContext(ctx,
		`SELECT target_project FROM project_aliases WHERE target_project = ?`, source,
	).Scan(&existingSource)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("alias source-is-target check: %w", err)
	}
	if err == nil {
		// source is already a target — chaining is not allowed
		return ErrAliasSourceIsTarget
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
	original := strings.TrimSpace(project)
	var target string
	err := d.sqlDB.QueryRowContext(ctx, `SELECT target_project FROM project_aliases WHERE source_project = ?`, original).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		canonical := canonicalProjectKey(original)
		if canonical == "" || canonical == original {
			return "", false, nil
		}
		err = d.sqlDB.QueryRowContext(ctx, `SELECT target_project FROM project_aliases WHERE source_project = ?`, canonical).Scan(&target)
		if err == nil {
			return canonicalProjectKey(target), true, nil
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("resolve alias: %w", err)
	}
	return target, true, nil
}

// resolveProjectIngressTx resolves a retired project before a local ingress
// can write any project-keyed row. It deliberately runs inside the caller's
// transaction: resolving before opening the transaction leaves a race where a
// concurrent promotion can retire the source after validation but before the
// insert. Aliases are stored under canonical keys, so fold before lookup and
// fold the target again defensively.
func resolveProjectIngressTx(ctx context.Context, tx *sql.Tx, project string) (string, error) {
	project = canonicalProjectKey(project)
	if project == "" {
		return "", nil
	}
	var target string
	err := tx.QueryRowContext(ctx, `SELECT target_project FROM project_aliases WHERE source_project = ?`, project).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		// Compatibility for historical aliases created before canonical storage.
		// The canonical lookup above is authoritative; this fallback only admits
		// a case-only legacy spelling and still returns the canonical target.
		err = tx.QueryRowContext(ctx, `SELECT target_project FROM project_aliases WHERE lower(source_project) = lower(?)`, project).Scan(&target)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return project, nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve project ingress alias: %w", err)
	}
	return canonicalProjectKey(target), nil
}

// resolveProjectIngress is the non-transactional counterpart for boundaries
// that open their own transaction after resolving. Transactional writers must
// prefer resolveProjectIngressTx.
func (d *DB) resolveProjectIngress(ctx context.Context, project string) (string, error) {
	project = canonicalProjectKey(project)
	if project == "" {
		return "", nil
	}
	target, found, err := d.ResolveAlias(ctx, project)
	if err != nil {
		return "", err
	}
	if found {
		return canonicalProjectKey(target), nil
	}
	return project, nil
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

	// Bidirectional chain guard: reject if source is already a target_project.
	var existingSource string
	err = tx.QueryRowContext(ctx,
		`SELECT target_project FROM project_aliases WHERE target_project = ?`, source,
	).Scan(&existingSource)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("alias source-is-target check (tx): %w", err)
	}
	if err == nil {
		// source is already a target — chaining is not allowed
		return ErrAliasSourceIsTarget
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
