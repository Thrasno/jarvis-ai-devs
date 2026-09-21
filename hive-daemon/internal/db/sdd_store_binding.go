package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

const (
	// SDDStoreBindingSchemaVersion is the only artifact-store binding schema this
	// daemon version can create. Reads intentionally preserve other versions so a
	// caller can fail closed instead of silently rewriting future data.
	SDDStoreBindingSchemaVersion = "1"

	SDDStoreModeHive     SDDStoreMode = "hive"
	SDDStoreModeHybrid   SDDStoreMode = "hybrid"
	SDDStoreModeNone     SDDStoreMode = "none"
	SDDStoreModeOpenSpec SDDStoreMode = "openspec"
)

var (
	// ErrSDDStoreBindingInvalid identifies input that cannot establish an
	// immutable artifact-store binding safely.
	ErrSDDStoreBindingInvalid = errors.New("invalid SDD store binding")
	// ErrSDDStoreBindingConflict identifies an adoption request that differs from
	// the immutable first-writer decision.
	ErrSDDStoreBindingConflict = errors.New("SDD store binding conflict")
)

// SDDStoreMode is the durable artifact-store mode accepted by Hive.
type SDDStoreMode string

// SDDStoreBinding is the immutable artifact-store decision for one canonical
// project and validated change. All fields are returned exactly as persisted so
// identity-promotion code can compare source and target values without mutation.
type SDDStoreBinding struct {
	Project       string
	Change        string
	SchemaVersion string
	Mode          SDDStoreMode
	Provenance    string
	CreatedAt     time.Time
}

// SDDStoreBindingRequest supplies the mutable-once request fields. Schema
// version is daemon-owned to prevent callers from creating a binding this daemon
// cannot interpret.
type SDDStoreBindingRequest struct {
	Mode       SDDStoreMode
	Provenance string
}

// SDDStoreBindingConflictError retains both immutable values when automatic
// convergence is unsafe.
type SDDStoreBindingConflictError struct {
	Existing  SDDStoreBinding
	Requested SDDStoreBinding
}

func (e *SDDStoreBindingConflictError) Error() string {
	return fmt.Sprintf("%v: existing schema=%q mode=%q provenance=%q differs from requested schema=%q mode=%q provenance=%q",
		ErrSDDStoreBindingConflict,
		e.Existing.SchemaVersion, e.Existing.Mode, e.Existing.Provenance,
		e.Requested.SchemaVersion, e.Requested.Mode, e.Requested.Provenance)
}

func (e *SDDStoreBindingConflictError) Unwrap() error { return ErrSDDStoreBindingConflict }

// GetSDDStoreBinding reads one immutable binding. A missing binding is reported
// as found=false, not as an error, and this method never registers or rewrites
// project identity state.
func (d *DB) GetSDDStoreBinding(ctx context.Context, project, change string) (SDDStoreBinding, bool, error) {
	project, change, err := canonicalSDDStoreBindingKey(project, change)
	if err != nil {
		return SDDStoreBinding{}, false, err
	}
	binding, found, err := getSDDStoreBinding(ctx, d.sqlDB, project, change)
	if err != nil {
		return SDDStoreBinding{}, false, fmt.Errorf("get SDD store binding: %w", err)
	}
	return binding, found, nil
}

// AdoptSDDStoreBinding atomically establishes the first immutable binding for a
// canonical (project, change) key. Exact replays return the stored binding with
// created=false; divergent values return SDDStoreBindingConflictError and never
// overwrite the existing row.
func (d *DB) AdoptSDDStoreBinding(ctx context.Context, project, change string, request SDDStoreBindingRequest) (SDDStoreBinding, bool, error) {
	requested, err := newSDDStoreBindingRequest(project, change, request)
	if err != nil {
		return SDDStoreBinding{}, false, err
	}

	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return SDDStoreBinding{}, false, fmt.Errorf("begin SDD store binding adoption: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO sdd_store_bindings (project, change_name, schema_version, mode, provenance)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project, change_name) DO NOTHING`,
		requested.Project, requested.Change, requested.SchemaVersion, requested.Mode, requested.Provenance)
	if err != nil {
		return SDDStoreBinding{}, false, fmt.Errorf("insert SDD store binding: %w", err)
	}
	created, err := result.RowsAffected()
	if err != nil {
		return SDDStoreBinding{}, false, fmt.Errorf("inspect SDD store binding insertion: %w", err)
	}

	existing, found, err := getSDDStoreBinding(ctx, tx, requested.Project, requested.Change)
	if err != nil {
		return SDDStoreBinding{}, false, fmt.Errorf("read authoritative SDD store binding: %w", err)
	}
	if !found {
		return SDDStoreBinding{}, false, fmt.Errorf("read authoritative SDD store binding: %w", ErrSDDStoreBindingInvalid)
	}
	if err := tx.Commit(); err != nil {
		return SDDStoreBinding{}, false, fmt.Errorf("commit SDD store binding adoption: %w", err)
	}
	if !sameSDDStoreBinding(existing, requested) {
		return SDDStoreBinding{}, false, &SDDStoreBindingConflictError{Existing: existing, Requested: requested}
	}
	return existing, created == 1, nil
}

type sddStoreBindingQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getSDDStoreBinding(ctx context.Context, queryer sddStoreBindingQueryer, project, change string) (SDDStoreBinding, bool, error) {
	var binding SDDStoreBinding
	var createdAt string
	err := queryer.QueryRowContext(ctx, `
		SELECT project, change_name, schema_version, mode, provenance, created_at
		FROM sdd_store_bindings
		WHERE project = ? AND change_name = ?`, project, change).
		Scan(&binding.Project, &binding.Change, &binding.SchemaVersion, &binding.Mode, &binding.Provenance, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SDDStoreBinding{}, false, nil
	}
	if err != nil {
		return SDDStoreBinding{}, false, err
	}
	binding.CreatedAt, err = parseTimeStr(createdAt)
	if err != nil {
		return SDDStoreBinding{}, false, fmt.Errorf("parse created_at: %w", err)
	}
	return binding, true, nil
}

func newSDDStoreBindingRequest(project, change string, request SDDStoreBindingRequest) (SDDStoreBinding, error) {
	project, change, err := canonicalSDDStoreBindingKey(project, change)
	if err != nil {
		return SDDStoreBinding{}, err
	}
	if request.Mode != SDDStoreModeHive && request.Mode != SDDStoreModeHybrid {
		return SDDStoreBinding{}, fmt.Errorf("%w: unsupported mode %q", ErrSDDStoreBindingInvalid, request.Mode)
	}
	provenance := strings.TrimSpace(request.Provenance)
	if provenance == "" {
		return SDDStoreBinding{}, fmt.Errorf("%w: provenance is required", ErrSDDStoreBindingInvalid)
	}
	return SDDStoreBinding{
		Project:       project,
		Change:        change,
		SchemaVersion: SDDStoreBindingSchemaVersion,
		Mode:          request.Mode,
		Provenance:    provenance,
	}, nil
}

func canonicalSDDStoreBindingKey(project, change string) (string, string, error) {
	project = canonicalProjectKey(project)
	change = strings.TrimSpace(change)
	if project == "" {
		return "", "", fmt.Errorf("%w: project is required", ErrSDDStoreBindingInvalid)
	}
	if change == "" {
		return "", "", fmt.Errorf("%w: change is required", ErrSDDStoreBindingInvalid)
	}
	if strings.ContainsAny(change, `/\\`) || strings.IndexFunc(change, unicode.IsControl) >= 0 {
		return "", "", fmt.Errorf("%w: change contains a separator or control character", ErrSDDStoreBindingInvalid)
	}
	return project, change, nil
}

// reconcileSDDStoreBindingsTx moves immutable source bindings only after the
// protected SDD progress guard has approved the promotion. It preserves every
// stored immutable field exactly; unequal same-change rows fail before either
// source binding is removed.
func reconcileSDDStoreBindingsTx(ctx context.Context, tx *sql.Tx, source, target string) error {
	rows, err := tx.QueryContext(ctx, `SELECT change_name, schema_version, mode, provenance, created_at FROM sdd_store_bindings WHERE project = ?`, source)
	if err != nil {
		return fmt.Errorf("read source SDD store bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var sourceBindings []SDDStoreBinding
	for rows.Next() {
		var binding SDDStoreBinding
		var createdAt string
		if err := rows.Scan(&binding.Change, &binding.SchemaVersion, &binding.Mode, &binding.Provenance, &createdAt); err != nil {
			return fmt.Errorf("scan source SDD store binding: %w", err)
		}
		binding.Project = source
		binding.CreatedAt, err = parseTimeStr(createdAt)
		if err != nil {
			return fmt.Errorf("parse source SDD store binding timestamp: %w", err)
		}
		sourceBindings = append(sourceBindings, binding)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate source SDD store bindings: %w", err)
	}
	for _, binding := range sourceBindings {
		targetBinding, found, err := getSDDStoreBinding(ctx, tx, target, binding.Change)
		if err != nil {
			return fmt.Errorf("read target SDD store binding: %w", err)
		}
		requested := binding
		requested.Project = target
		if found && !sameSDDStoreBinding(targetBinding, requested) {
			return &SDDStoreBindingConflictError{Existing: targetBinding, Requested: requested}
		}
	}
	for _, binding := range sourceBindings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sdd_store_bindings (project, change_name, schema_version, mode, provenance, created_at) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(project, change_name) DO NOTHING`, target, binding.Change, binding.SchemaVersion, binding.Mode, binding.Provenance, binding.CreatedAt.UTC().Format("2006-01-02 15:04:05")); err != nil {
			return fmt.Errorf("move SDD store binding: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sdd_store_bindings WHERE project = ?`, source); err != nil {
		return fmt.Errorf("delete moved SDD store bindings: %w", err)
	}
	return nil
}

func sameSDDStoreBinding(left, right SDDStoreBinding) bool {
	return left.Project == right.Project &&
		left.Change == right.Change &&
		left.SchemaVersion == right.SchemaVersion &&
		left.Mode == right.Mode &&
		left.Provenance == right.Provenance
}
