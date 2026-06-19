package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	maxSyncAttemptBatchSize        = 100
	maxSyncAttemptErrorMessageRune = 500
)

type SyncAttemptOutcome string

const (
	SyncAttemptOutcomeSuccess SyncAttemptOutcome = "success"
	SyncAttemptOutcomeFailure SyncAttemptOutcome = "failure"
)

type SyncAttemptLog struct {
	AttemptID      string
	DevID          string
	Project        string
	Client         string
	DaemonID       string
	StartedAt      time.Time
	EndedAt        time.Time
	Outcome        SyncAttemptOutcome
	HTTPStatus     int
	ErrorCode      string
	ErrorMessage   string
	RequestID      string
	SyncCountsJSON string
	MetadataJSON   string
}

func (d *DB) RecordSyncAttemptLog(ctx context.Context, log SyncAttemptLog) error {
	if strings.TrimSpace(log.AttemptID) == "" {
		return errors.New("attempt_id is required")
	}
	if strings.TrimSpace(log.DevID) == "" {
		return errors.New("dev_id is required")
	}
	if strings.TrimSpace(log.Project) == "" {
		return errors.New("project is required")
	}
	if log.Outcome != SyncAttemptOutcomeSuccess && log.Outcome != SyncAttemptOutcomeFailure {
		return fmt.Errorf("unsupported sync attempt outcome %q", log.Outcome)
	}
	if log.StartedAt.IsZero() {
		return errors.New("started_at is required")
	}
	if log.EndedAt.IsZero() {
		log.EndedAt = log.StartedAt
	}
	if strings.TrimSpace(log.SyncCountsJSON) == "" {
		log.SyncCountsJSON = "{}"
	}
	if strings.TrimSpace(log.MetadataJSON) == "" {
		log.MetadataJSON = "{}"
	}
	log.ErrorMessage = SanitizeSyncAttemptError(log.DevID, log.ErrorMessage)

	_, err := d.sqlDB.ExecContext(ctx, `
INSERT INTO sync_attempt_logs
    (attempt_id, dev_id, project, client, daemon_id, started_at, ended_at, outcome,
     http_status, error_code, error_message, request_id, sync_counts_json, metadata_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(attempt_id) DO UPDATE SET
    dev_id = excluded.dev_id,
    project = excluded.project,
    client = excluded.client,
    daemon_id = excluded.daemon_id,
    started_at = excluded.started_at,
    ended_at = excluded.ended_at,
    outcome = excluded.outcome,
    http_status = excluded.http_status,
    error_code = excluded.error_code,
    error_message = excluded.error_message,
    request_id = excluded.request_id,
    sync_counts_json = excluded.sync_counts_json,
    metadata_json = excluded.metadata_json`,
		log.AttemptID, log.DevID, log.Project, log.Client, log.DaemonID,
		formatSQLiteTime(log.StartedAt), formatSQLiteTime(log.EndedAt), string(log.Outcome),
		log.HTTPStatus, log.ErrorCode, log.ErrorMessage, log.RequestID, log.SyncCountsJSON, log.MetadataJSON,
	)
	if err != nil {
		return fmt.Errorf("record sync attempt log: %w", err)
	}
	return nil
}

func (d *DB) ListPendingSyncAttemptLogs(ctx context.Context, limit int) ([]SyncAttemptLog, error) {
	if limit <= 0 || limit > maxSyncAttemptBatchSize {
		limit = maxSyncAttemptBatchSize
	}
	rows, err := d.sqlDB.QueryContext(ctx, `
SELECT attempt_id, dev_id, project, client, daemon_id, started_at, ended_at, outcome,
       http_status, error_code, error_message, request_id, sync_counts_json, metadata_json
FROM sync_attempt_logs
WHERE delivered_at IS NULL AND trim(dev_id) != ''
ORDER BY started_at ASC, attempt_id ASC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending sync attempt logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var logs []SyncAttemptLog
	for rows.Next() {
		log, err := scanSyncAttemptLog(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (d *DB) MarkSyncAttemptLogsDelivered(ctx context.Context, ids []string, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin mark sync attempts delivered: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	formatted := formatSQLiteTime(at)
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sync_attempt_logs SET delivered_at = ? WHERE attempt_id = ?`, formatted, id); err != nil {
			return fmt.Errorf("mark sync attempt delivered %s: %w", id, err)
		}
	}
	return tx.Commit()
}

func (d *DB) DeleteSyncAttemptLogsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := d.sqlDB.ExecContext(ctx, `DELETE FROM sync_attempt_logs WHERE ended_at < ?`, formatSQLiteTime(cutoff))
	if err != nil {
		return 0, fmt.Errorf("delete old sync attempt logs: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read deleted sync attempt count: %w", err)
	}
	return deleted, nil
}

func scanSyncAttemptLog(rows *sql.Rows) (SyncAttemptLog, error) {
	var log SyncAttemptLog
	var startedAt, endedAt, outcome string
	if err := rows.Scan(
		&log.AttemptID, &log.DevID, &log.Project, &log.Client, &log.DaemonID,
		&startedAt, &endedAt, &outcome, &log.HTTPStatus, &log.ErrorCode, &log.ErrorMessage,
		&log.RequestID, &log.SyncCountsJSON, &log.MetadataJSON,
	); err != nil {
		return SyncAttemptLog{}, fmt.Errorf("scan sync attempt log: %w", err)
	}
	var err error
	log.StartedAt, err = parseTimeStr(startedAt)
	if err != nil {
		return SyncAttemptLog{}, fmt.Errorf("parse sync attempt started_at: %w", err)
	}
	log.EndedAt, err = parseTimeStr(endedAt)
	if err != nil {
		return SyncAttemptLog{}, fmt.Errorf("parse sync attempt ended_at: %w", err)
	}
	log.Outcome = SyncAttemptOutcome(outcome)
	return log, nil
}

func SanitizeSyncAttemptError(devID, message string) string {
	message = strings.ReplaceAll(message, "\r\n", "\n")
	message = strings.ReplaceAll(message, "\r", "\n")
	lines := strings.Split(message, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if trimmed == "" {
			continue
		}
		if isSensitiveHeaderLine(lower) {
			continue
		}
		if strings.Contains(lower, "request body") || strings.Contains(lower, "response body") {
			continue
		}
		if strings.HasPrefix(lower, "at ") || strings.Contains(lower, ".go:") || strings.Contains(lower, "goroutine ") {
			continue
		}
		kept = append(kept, trimmed)
	}

	cleaned := strings.Join(kept, " ")
	cleaned = emailPattern.ReplaceAllString(cleaned, "[redacted-email]")
	if strings.TrimSpace(devID) != "" {
		cleaned = strings.ReplaceAll(cleaned, devID, "[redacted-email]")
	}
	cleaned = secretAssignmentPattern.ReplaceAllString(cleaned, "$1$2 [redacted]")
	cleaned = bearerPattern.ReplaceAllString(cleaned, "Bearer [redacted]")
	cleaned = localPathPattern.ReplaceAllString(cleaned, "[redacted-path]")
	cleaned = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, cleaned)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return truncateRunes(cleaned, maxSyncAttemptErrorMessageRune)
}

func isSensitiveHeaderLine(lower string) bool {
	key, _, ok := strings.Cut(lower, ":")
	if !ok || strings.Contains(key, " ") {
		return false
	}
	key = strings.TrimSpace(key)
	return key == "authorization" || key == "proxy-authorization" || key == "cookie" || key == "set-cookie" ||
		key == "x-api-key" || key == "api-key" || key == "x-auth-token"
}

func formatSQLiteTime(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04:05")
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

var (
	emailPattern            = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	bearerPattern           = regexp.MustCompile(`(?i)Bearer\s+[^\s,;]+`)
	secretAssignmentPattern = regexp.MustCompile(`(?i)\b(token|api[_-]?key|password|secret)\s*([=:])\s*([^\s,;]+)`)
	localPathPattern        = regexp.MustCompile(`(?i)(/home/[^\s,;)]+|/Users/[^\s,;)]+|/tmp/[^\s,;)]+|[A-Z]:\\[^\s,;)]+)`)
)
