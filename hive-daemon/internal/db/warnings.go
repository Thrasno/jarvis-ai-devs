package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type HiveWarning struct {
	ID              int64
	CreatedAt       time.Time
	Severity        string
	Source          string
	Message         string
	ResolutionState string
	ResolvedAt      *time.Time
}

type HiveWarningInput struct {
	Severity string
	Source   string
	Message  string
}

type HiveWarningFilter struct {
	ResolutionState string
}

func (d *DB) SaveHiveWarning(input HiveWarningInput) (HiveWarning, error) {
	input.Severity = strings.TrimSpace(input.Severity)
	input.Source = strings.TrimSpace(input.Source)
	input.Message = strings.TrimSpace(input.Message)
	if input.Severity == "" {
		return HiveWarning{}, fmt.Errorf("severity is required")
	}
	if input.Source == "" {
		return HiveWarning{}, fmt.Errorf("source is required")
	}
	if input.Message == "" {
		return HiveWarning{}, fmt.Errorf("message is required")
	}

	res, err := d.sqlDB.Exec(`
INSERT INTO hive_warnings (severity, source, message)
VALUES (?, ?, ?)`, input.Severity, input.Source, input.Message)
	if err != nil {
		return HiveWarning{}, fmt.Errorf("save hive warning: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return HiveWarning{}, fmt.Errorf("read hive warning id: %w", err)
	}
	return d.getHiveWarning(id)
}

func (d *DB) ListHiveWarnings(filter HiveWarningFilter) ([]HiveWarning, error) {
	q := `
SELECT id, created_at, severity, source, message, resolution_state, resolved_at
FROM hive_warnings`
	args := []any{}
	if strings.TrimSpace(filter.ResolutionState) != "" {
		q += ` WHERE resolution_state = ?`
		args = append(args, strings.TrimSpace(filter.ResolutionState))
	}
	q += ` ORDER BY created_at DESC, id DESC`

	rows, err := d.sqlDB.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list hive warnings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var warnings []HiveWarning
	for rows.Next() {
		warning, err := scanHiveWarning(rows)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warning)
	}
	return warnings, rows.Err()
}

func (d *DB) ResolveHiveWarning(id int64, resolvedAt time.Time) error {
	if resolvedAt.IsZero() {
		resolvedAt = time.Now().UTC()
	}
	res, err := d.sqlDB.Exec(`
UPDATE hive_warnings
SET resolution_state = 'resolved', resolved_at = ?
WHERE id = ?`, resolvedAt.UTC().Format("2006-01-02 15:04:05"), id)
	if err != nil {
		return fmt.Errorf("resolve hive warning: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) getHiveWarning(id int64) (HiveWarning, error) {
	row := d.sqlDB.QueryRow(`
SELECT id, created_at, severity, source, message, resolution_state, resolved_at
FROM hive_warnings
WHERE id = ?`, id)
	return scanHiveWarning(row)
}

type hiveWarningScanner interface {
	Scan(dest ...any) error
}

func scanHiveWarning(scanner hiveWarningScanner) (HiveWarning, error) {
	var warning HiveWarning
	var createdAt string
	var resolvedAt sql.NullString
	if err := scanner.Scan(&warning.ID, &createdAt, &warning.Severity, &warning.Source, &warning.Message, &warning.ResolutionState, &resolvedAt); err != nil {
		return HiveWarning{}, fmt.Errorf("scan hive warning: %w", err)
	}
	parsedCreatedAt, err := parseTimeStr(createdAt)
	if err != nil {
		return HiveWarning{}, fmt.Errorf("parse hive warning created_at: %w", err)
	}
	warning.CreatedAt = parsedCreatedAt
	if resolvedAt.Valid && resolvedAt.String != "" {
		parsedResolvedAt, err := parseTimeStr(resolvedAt.String)
		if err != nil {
			return HiveWarning{}, fmt.Errorf("parse hive warning resolved_at: %w", err)
		}
		warning.ResolvedAt = &parsedResolvedAt
	}
	return warning, nil
}
