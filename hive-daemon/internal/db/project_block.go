package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

const (
	ProjectBlockAckApplied = "applied"
	ProjectBlockAckFailed  = "failed"
	ProjectBlockAckSkipped = "skipped"
)

var ErrProjectBlocked = errors.New("project is blocked")

var projectBlockSeparatorRun = regexp.MustCompile(`[^a-z0-9.]+`)

type ProjectBlockCommand struct {
	CommandID           string    `json:"command_id"`
	AckToken            string    `json:"ack_token"`
	Project             string    `json:"project"`
	CanonicalProjectKey string    `json:"canonical_project_key"`
	Reason              string    `json:"reason"`
	Action              string    `json:"action"`
	Generation          int64     `json:"generation"`
	BlockedAt           time.Time `json:"blocked_at"`
}

type ProjectBlock struct {
	CommandID           string
	AckToken            string
	Project             string
	CanonicalProjectKey string
	Reason              string
	Action              string
	Generation          int64
	Blocked             bool
	BlockedAt           time.Time
	AckPending          bool
	AckStatus           string
	AckWarning          string
	AckAppliedAt        time.Time
}

type ProjectBlockAck struct {
	CommandID           string    `json:"command_id"`
	CanonicalProjectKey string    `json:"canonical_project_key"`
	AckToken            string    `json:"ack_token"`
	Status              string    `json:"status"`
	Warning             string    `json:"warning,omitempty"`
	AppliedAt           time.Time `json:"applied_at"`
}

type ProjectQuarantineResult struct {
	Project string
	Mutated bool
	Warning string
}

func (d *DB) RecordProjectBlock(ctx context.Context, cmd ProjectBlockCommand) (ProjectBlock, error) {
	canonical := canonicalProjectKey(cmd.CanonicalProjectKey)
	if canonical == "" {
		canonical = canonicalProjectKey(cmd.Project)
	}
	ackToken := strings.TrimSpace(cmd.AckToken)
	if strings.TrimSpace(cmd.CommandID) == "" || ackToken == "" || canonical == "" {
		return ProjectBlock{}, fmt.Errorf("project block command requires command_id, ack_token, and canonical project key")
	}
	projectName := strings.TrimSpace(cmd.Project)
	if projectName == "" {
		projectName = canonical
	}
	blockedAt := cmd.BlockedAt.UTC()
	if blockedAt.IsZero() {
		blockedAt = time.Now().UTC()
	}
	blockedAtStr := formatDBTime(blockedAt)
	action := strings.TrimSpace(cmd.Action)
	if action == "" {
		action = "block"
	}
	if action != "block" && action != "unblock" {
		return ProjectBlock{}, fmt.Errorf("project block command has unsupported action %q", action)
	}
	generation := cmd.Generation
	if generation < 1 {
		generation = 1
	}
	blocked := action == "block"
	_, err := d.sqlDB.ExecContext(ctx, `
	INSERT INTO project_blocks (canonical_project_key, project, command_id, ack_token, reason, action, generation, blocked, blocked_at, ack_pending, ack_status, ack_warning, ack_applied_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, '', '', NULL, CURRENT_TIMESTAMP)
ON CONFLICT(canonical_project_key) DO UPDATE SET
    project = excluded.project,
    command_id = excluded.command_id,
	ack_token = excluded.ack_token,
    reason = excluded.reason,
	    action = excluded.action,
	    generation = excluded.generation,
	    blocked = excluded.blocked,
    blocked_at = excluded.blocked_at,
    ack_pending = CASE WHEN project_blocks.command_id = excluded.command_id THEN project_blocks.ack_pending ELSE 1 END,
    ack_status = CASE WHEN project_blocks.command_id = excluded.command_id THEN project_blocks.ack_status ELSE '' END,
    ack_warning = CASE WHEN project_blocks.command_id = excluded.command_id THEN project_blocks.ack_warning ELSE '' END,
    ack_applied_at = CASE WHEN project_blocks.command_id = excluded.command_id THEN project_blocks.ack_applied_at ELSE NULL END,
	    updated_at = CURRENT_TIMESTAMP
	WHERE excluded.generation > project_blocks.generation`, canonical, projectName, cmd.CommandID, ackToken, cmd.Reason, action, generation, blocked, blockedAtStr)
	if err != nil {
		return ProjectBlock{}, fmt.Errorf("record project block: %w", err)
	}
	return d.GetProjectBlock(ctx, canonical)
}

func (d *DB) GetProjectBlock(ctx context.Context, project string) (ProjectBlock, error) {
	canonical := canonicalProjectKey(project)
	if canonical == "" {
		return ProjectBlock{}, sql.ErrNoRows
	}
	return d.getProjectBlockByCanonicalKey(ctx, canonical)
}

func (d *DB) getProjectBlockByCanonicalKey(ctx context.Context, canonical string) (ProjectBlock, error) {
	var (
		block                         ProjectBlock
		blockedAtStr, ackAppliedAtStr sql.NullString
		ackPendingInt, blockedInt     int
	)
	err := d.sqlDB.QueryRowContext(ctx, `
	SELECT command_id, ack_token, project, canonical_project_key, reason, action, generation, blocked, blocked_at, ack_pending, ack_status, ack_warning, ack_applied_at
FROM project_blocks
WHERE canonical_project_key = ?`, canonical).Scan(
		&block.CommandID, &block.AckToken, &block.Project, &block.CanonicalProjectKey, &block.Reason, &block.Action, &block.Generation, &blockedInt, &blockedAtStr,
		&ackPendingInt, &block.AckStatus, &block.AckWarning, &ackAppliedAtStr,
	)
	if err != nil {
		return ProjectBlock{}, err
	}
	block.AckPending = ackPendingInt != 0
	block.Blocked = blockedInt != 0
	if blockedAtStr.Valid {
		block.BlockedAt, _ = parseTimeStr(blockedAtStr.String)
	}
	if ackAppliedAtStr.Valid && ackAppliedAtStr.String != "" {
		block.AckAppliedAt, _ = parseTimeStr(ackAppliedAtStr.String)
	}
	return block, nil
}

func (d *DB) IsProjectBlocked(ctx context.Context, project string) (bool, error) {
	block, err := d.GetProjectBlock(ctx, project)
	return projectBlocked(block, err)
}

func (d *DB) isCanonicalProjectBlocked(ctx context.Context, canonical string) (bool, error) {
	block, err := d.getProjectBlockByCanonicalKey(ctx, canonical)
	return projectBlocked(block, err)
}

func projectBlocked(block ProjectBlock, err error) (bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check project block: %w", err)
	}
	return block.Blocked, nil
}

func (d *DB) RecordProjectBlockAck(ctx context.Context, ack ProjectBlockAck) (ProjectBlockAck, error) {
	canonical := canonicalProjectKey(ack.CanonicalProjectKey)
	ackToken := strings.TrimSpace(ack.AckToken)
	if strings.TrimSpace(ack.CommandID) == "" || ackToken == "" || canonical == "" {
		return ProjectBlockAck{}, fmt.Errorf("project block ack requires command_id, ack_token, and canonical project key")
	}
	status, _, ok := normalizeProjectBlockAckStatus(ack.Status, ack.Warning)
	if !ok {
		return ProjectBlockAck{}, fmt.Errorf("invalid project block ack status %q", ack.Status)
	}
	appliedAt := ack.AppliedAt.UTC()
	if appliedAt.IsZero() {
		appliedAt = time.Now().UTC()
	}
	result, err := d.sqlDB.ExecContext(ctx, `
UPDATE project_blocks
SET ack_pending = 0,
    ack_status = ?,
    ack_warning = ?,
    ack_applied_at = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE canonical_project_key = ? AND command_id = ? AND ack_token = ?`, status, ack.Warning, formatDBTime(appliedAt), canonical, ack.CommandID, ackToken)
	if err != nil {
		return ProjectBlockAck{}, fmt.Errorf("record project block ack: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ProjectBlockAck{}, sql.ErrNoRows
	}
	ack.CanonicalProjectKey = canonical
	ack.AckToken = ackToken
	ack.Status = status
	ack.AppliedAt = appliedAt
	return ack, nil
}

func (d *DB) RecordPendingProjectBlockAck(ctx context.Context, ack ProjectBlockAck) error {
	canonical := canonicalProjectKey(ack.CanonicalProjectKey)
	ackToken := strings.TrimSpace(ack.AckToken)
	if strings.TrimSpace(ack.CommandID) == "" || ackToken == "" || canonical == "" {
		return fmt.Errorf("pending project block ack requires command_id, ack_token, and canonical project key")
	}
	status, _, ok := normalizeProjectBlockAckStatus(ack.Status, ack.Warning)
	if !ok {
		return fmt.Errorf("invalid project block ack status %q", ack.Status)
	}
	appliedAt := ack.AppliedAt.UTC()
	if appliedAt.IsZero() {
		appliedAt = time.Now().UTC()
	}
	result, err := d.sqlDB.ExecContext(ctx, `
UPDATE project_blocks
SET ack_pending = 1,
    ack_status = ?,
    ack_warning = ?,
    ack_applied_at = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE canonical_project_key = ? AND command_id = ? AND ack_token = ?`, status, ack.Warning, formatDBTime(appliedAt), canonical, ack.CommandID, ackToken)
	if err != nil {
		return fmt.Errorf("record pending project block ack: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) ListPendingProjectBlockAcks(ctx context.Context, limit int) ([]ProjectBlockAck, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.sqlDB.QueryContext(ctx, `
SELECT command_id, canonical_project_key, ack_token, ack_status, ack_warning, ack_applied_at
FROM project_blocks
WHERE ack_pending = 1
ORDER BY blocked_at ASC, canonical_project_key ASC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending project block acks: %w", err)
	}
	defer rows.Close()

	acks := make([]ProjectBlockAck, 0)
	for rows.Next() {
		ack := ProjectBlockAck{AppliedAt: time.Now().UTC()}
		var appliedAt sql.NullString
		if err := rows.Scan(&ack.CommandID, &ack.CanonicalProjectKey, &ack.AckToken, &ack.Status, &ack.Warning, &appliedAt); err != nil {
			return nil, fmt.Errorf("scan pending project block ack: %w", err)
		}
		status, warning, ok := normalizeProjectBlockAckStatus(ack.Status, ack.Warning)
		if !ok {
			status = ProjectBlockAckFailed
			warning = strings.TrimSpace(ack.Warning)
			if warning == "" {
				warning = "pending project block ack missing durable status"
			}
		}
		ack.Status = status
		ack.Warning = warning
		if appliedAt.Valid && appliedAt.String != "" {
			ack.AppliedAt, _ = parseTimeStr(appliedAt.String)
		}
		acks = append(acks, ack)
	}
	return acks, rows.Err()
}

func normalizeProjectBlockAckStatus(status, warning string) (string, string, bool) {
	normalized := strings.TrimSpace(status)
	switch normalized {
	case ProjectBlockAckApplied, ProjectBlockAckFailed, ProjectBlockAckSkipped:
		return normalized, strings.TrimSpace(warning), true
	case "":
		return ProjectBlockAckFailed, strings.TrimSpace(warning), false
	default:
		return "", strings.TrimSpace(warning), false
	}
}

func (d *DB) QuarantineBlockedProject(ctx context.Context, projectName, actorID, reason string, at time.Time) (ProjectQuarantineResult, error) {
	block, err := d.GetProjectBlock(ctx, projectName)
	if err != nil {
		return ProjectQuarantineResult{}, err
	}
	var mutated bool
	if block.Blocked {
		mutated, err = d.ArchiveGovernanceProject(ctx, block.Project, actorID, reason, at)
		if err != nil {
			return ProjectQuarantineResult{}, err
		}
		if mutated {
			if _, err := d.sqlDB.ExecContext(ctx, `
				INSERT INTO project_quarantine_archives (canonical_project_key, project, command_id)
				VALUES (?, ?, ?)
				ON CONFLICT(canonical_project_key) DO UPDATE SET project = excluded.project, command_id = excluded.command_id`,
				block.CanonicalProjectKey, block.Project, block.CommandID); err != nil {
				return ProjectQuarantineResult{}, fmt.Errorf("record quarantine-owned archive: %w", err)
			}
		}
	} else {
		mutated, err = d.restoreQuarantineOwnedArchive(ctx, block)
		if err != nil {
			return ProjectQuarantineResult{}, err
		}
	}
	result := ProjectQuarantineResult{Project: block.Project, Mutated: mutated}
	if warning := d.blockedProjectRootWarning(ctx, block.Project); warning != "" {
		result.Warning = warning
		_, _ = d.SaveHiveWarning(HiveWarningInput{Severity: "warning", Source: "project-block:" + block.Project, Message: warning})
	}
	return result, nil
}

func (d *DB) restoreQuarantineOwnedArchive(ctx context.Context, block ProjectBlock) (bool, error) {
	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin restore quarantine archive: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var project string
	err = tx.QueryRowContext(ctx, `SELECT project FROM project_quarantine_archives WHERE canonical_project_key = ?`, block.CanonicalProjectKey).Scan(&project)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read quarantine-owned archive: %w", err)
	}
	if project != block.Project {
		return false, fmt.Errorf("quarantine-owned archive project mismatch")
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE hive_project_governance
		SET archived_at = NULL, archived_by = '', archive_reason = ''
		WHERE project = ? AND merged_at IS NULL`, project)
	if err != nil {
		return false, fmt.Errorf("restore quarantine-owned archive: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_quarantine_archives WHERE canonical_project_key = ?`, block.CanonicalProjectKey); err != nil {
		return false, fmt.Errorf("delete quarantine-owned archive marker: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit restore quarantine archive: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("restore quarantine-owned archive rows affected: %w", err)
	}
	return rows > 0, nil
}

func (d *DB) blockedProjectRootWarning(ctx context.Context, projectName string) string {
	var directory string
	_ = d.sqlDB.QueryRowContext(ctx, `
SELECT directory FROM sessions
WHERE project = ? AND TRIM(directory) != ''
ORDER BY started_at DESC, id DESC
LIMIT 1`, projectName).Scan(&directory)
	homeDir, _ := os.UserHomeDir()
	warning, ok := project.BlockedProjectUnsafeRootWarning(directory, homeDir)
	if !ok {
		return ""
	}
	return warning
}

func (d *DB) ensureProjectWritable(ctx context.Context, project string) error {
	blocked, err := d.IsProjectBlocked(ctx, project)
	if err != nil {
		return err
	}
	if blocked {
		return ErrProjectBlocked
	}
	return nil
}

func ensureProjectWritableInTx(tx *sql.Tx, project string) error {
	canonical := canonicalProjectKey(project)
	if canonical == "" {
		return nil
	}
	var exists int
	err := tx.QueryRow(`SELECT 1 FROM project_blocks WHERE canonical_project_key = ? AND blocked = 1 LIMIT 1`, canonical).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check project block: %w", err)
	}
	return ErrProjectBlocked
}

func (d *DB) blockedProjectKeys(ctx context.Context) (map[string]struct{}, error) {
	rows, err := d.sqlDB.QueryContext(ctx, `SELECT canonical_project_key FROM project_blocks WHERE blocked = 1`)
	if err != nil {
		return nil, fmt.Errorf("list blocked project keys: %w", err)
	}
	defer rows.Close()
	keys := map[string]struct{}{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan blocked project key: %w", err)
		}
		keys[key] = struct{}{}
	}
	return keys, rows.Err()
}

func formatDBTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}
