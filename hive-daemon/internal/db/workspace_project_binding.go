package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrWorkspaceProjectBindingRequired = errors.New("workspace project binding requires workspace and project")
	ErrWorkspaceProjectBindingNotFound = errors.New("workspace project binding not found")
	ErrWorkspaceProjectBindingConflict = errors.New("workspace project binding changed")
	ErrWorkspacePromotionProtectedSDD  = errors.New("workspace project promotion is blocked by immutable SDD apply progress")
)

// WorkspacePromotionProtectedError explains why a workspace identity cannot be
// promoted without rewriting immutable apply-progress topology. Issue #724 owns
// the supersession flow that can make this transition safe.
type WorkspacePromotionProtectedError struct {
	Source string
	Target string
}

func (e *WorkspacePromotionProtectedError) Error() string {
	return fmt.Sprintf("%v from %q to %q; preserve the immutable records and complete issue #724 supersession before retrying", ErrWorkspacePromotionProtectedSDD, e.Source, e.Target)
}

func (e *WorkspacePromotionProtectedError) Unwrap() error {
	return ErrWorkspacePromotionProtectedSDD
}

// ResolveWorkspaceProjectBinding returns the canonical project currently bound
// to workspace. Workspace callers must pass their canonical local path; this
// repository also cleans it defensively so every DB entry has one stable form.
func (d *DB) ResolveWorkspaceProjectBinding(ctx context.Context, workspace string) (string, bool, error) {
	workspace = canonicalWorkspacePath(workspace)
	if workspace == "" {
		return "", false, ErrWorkspaceProjectBindingRequired
	}
	var project string
	err := d.sqlDB.QueryRowContext(ctx, `SELECT project FROM workspace_project_bindings WHERE workspace = ?`, workspace).Scan(&project)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("resolve workspace project binding: %w", err)
	}
	if target, found, err := d.ResolveAlias(ctx, project); err != nil {
		return "", false, fmt.Errorf("resolve workspace binding alias: %w", err)
	} else if found {
		project = target
	}
	return project, true, nil
}

// EnsureWorkspaceProjectBinding records the first observed canonical project for
// a workspace without overwriting a concurrent or previously durable binding.
// It returns the stored project so callers can re-resolve after a process restart.
func (d *DB) EnsureWorkspaceProjectBinding(ctx context.Context, workspace, project string) (string, bool, error) {
	workspace = canonicalWorkspacePath(workspace)
	project = canonicalProjectKey(project)
	if workspace == "" || project == "" {
		return "", false, ErrWorkspaceProjectBindingRequired
	}
	if target, found, err := d.ResolveAlias(ctx, project); err != nil {
		return "", false, fmt.Errorf("resolve workspace binding alias: %w", err)
	} else if found {
		project = target
	}
	result, err := d.sqlDB.ExecContext(ctx, `
INSERT INTO workspace_project_bindings (workspace, project, created_at, updated_at)
VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(workspace) DO NOTHING`, workspace, project)
	if err != nil {
		return "", false, fmt.Errorf("ensure workspace project binding: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return "", false, fmt.Errorf("workspace project binding rows affected: %w", err)
	}
	bound, found, err := d.ResolveWorkspaceProjectBinding(ctx, workspace)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, ErrWorkspaceProjectBindingNotFound
	}
	return bound, inserted > 0, nil
}

// PromoteWorkspaceProject promotes one existing workspace binding. It shares
// the same transaction helper as first-bind promotion so protected SDD topology
// rejects without leaving either partial migration or partial binding state.
func (d *DB) PromoteWorkspaceProject(ctx context.Context, workspace, source, target string) (bool, error) {
	return d.promoteWorkspaceProject(ctx, workspace, source, target, false)
}

// BindAndPromoteWorkspaceProject atomically records source for an unbound
// workspace and promotes it to target. It is the validator path for legacy A
// discovered just as Git B becomes usable; a protected promotion rolls the new
// binding back with the rest of the transaction.
func (d *DB) BindAndPromoteWorkspaceProject(ctx context.Context, workspace, source, target string) (bool, error) {
	return d.promoteWorkspaceProject(ctx, workspace, source, target, true)
}

func (d *DB) promoteWorkspaceProject(ctx context.Context, workspace, source, target string, bindSource bool) (bool, error) {
	workspace = canonicalWorkspacePath(workspace)
	source = canonicalProjectKey(source)
	target = canonicalProjectKey(target)
	if workspace == "" || source == "" || target == "" {
		return false, ErrWorkspaceProjectBindingRequired
	}
	if source == target {
		return false, ErrGovernanceProjectMergeInvalid
	}
	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin workspace project promotion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if bindSource {
		if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_project_bindings (workspace, project, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) ON CONFLICT(workspace) DO NOTHING`, workspace, source); err != nil {
			return false, fmt.Errorf("bind workspace project before promotion: %w", err)
		}
	}
	changed, err := d.promoteWorkspaceProjectTx(ctx, tx, workspace, source, target)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit workspace project promotion: %w", err)
	}
	return changed, nil
}

func (d *DB) promoteWorkspaceProjectTx(ctx context.Context, tx *sql.Tx, workspace, source, target string) (bool, error) {
	var bound string
	err := tx.QueryRowContext(ctx, `SELECT project FROM workspace_project_bindings WHERE workspace = ?`, workspace).Scan(&bound)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrWorkspaceProjectBindingNotFound
	}
	if err != nil {
		return false, fmt.Errorf("revalidate workspace project binding: %w", err)
	}
	activeBound, err := resolveWorkspaceAliasTx(ctx, tx, bound)
	if err != nil {
		return false, err
	}
	if activeBound == target {
		return false, nil
	}
	if activeBound != source {
		return false, fmt.Errorf("%w: workspace %q is bound to %q, not %q", ErrWorkspaceProjectBindingConflict, workspace, activeBound, source)
	}
	changed, err := d.MergeGovernanceProjectTx(ctx, tx, source, target, target, "workspace-validator", "workspace Git identity promotion", time.Now().UTC(), true, true)
	if err != nil {
		return false, err
	}
	// Advance direct source bindings and stale aliases whose active target was
	// source, so no workspace can return a retired project after promotion.
	if _, err := tx.ExecContext(ctx, `UPDATE workspace_project_bindings SET project = ?, updated_at = CURRENT_TIMESTAMP WHERE project = ? OR project IN (SELECT source_project FROM project_aliases WHERE target_project = ?)`, target, source, source); err != nil {
		return false, fmt.Errorf("update workspace project bindings: %w", err)
	}
	return changed, nil
}

func resolveWorkspaceAliasTx(ctx context.Context, tx *sql.Tx, project string) (string, error) {
	seen := map[string]bool{}
	for project != "" {
		if seen[project] {
			return "", ErrWorkspaceProjectBindingConflict
		}
		seen[project] = true
		var target string
		err := tx.QueryRowContext(ctx, `SELECT target_project FROM project_aliases WHERE source_project = ?`, project).Scan(&target)
		if errors.Is(err, sql.ErrNoRows) {
			return project, nil
		}
		if err != nil {
			return "", fmt.Errorf("resolve workspace binding alias: %w", err)
		}
		project = target
	}
	return "", ErrWorkspaceProjectBindingConflict
}

func canonicalWorkspacePath(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	return filepath.Clean(workspace)
}
