package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/project"
)

func (d *DB) CreateRecoveryToken(ctx context.Context, req project.TokenRequest) (string, error) {
	token := req.Token
	if token == "" {
		generated, err := randomToken()
		if err != nil {
			return "", err
		}
		token = generated
	}
	candidatesJSON, err := json.Marshal(req.Candidates)
	if err != nil {
		return "", fmt.Errorf("marshal candidates: %w", err)
	}
	_, err = d.sqlDB.ExecContext(ctx, `
		INSERT INTO recovery_tokens
		    (token, reason, requested_project, candidates_json, context_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, token, req.Reason, req.RequestedProject, string(candidatesJSON), req.ContextHash, formatTokenTime(req.CreatedAt), formatTokenTime(req.ExpiresAt))
	if err != nil {
		return "", fmt.Errorf("insert recovery token: %w", err)
	}
	return token, nil
}

func (d *DB) ConsumeRecoveryToken(ctx context.Context, validation project.TokenValidation) error {
	return d.validateRecoveryToken(ctx, validation, true)
}

func (d *DB) ValidateRecoveryToken(ctx context.Context, validation project.TokenValidation) error {
	return d.validateRecoveryToken(ctx, validation, false)
}

func (d *DB) validateRecoveryToken(ctx context.Context, validation project.TokenValidation, consume bool) error {
	now := validation.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin consume recovery token: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var candidatesJSON, contextHash, consumedAtRaw string
	var expiresAtRaw string
	err = tx.QueryRowContext(ctx, `
		SELECT candidates_json, context_hash, COALESCE(consumed_at, ''), expires_at
		FROM recovery_tokens
		WHERE token = ?
	`, validation.Token).Scan(&candidatesJSON, &contextHash, &consumedAtRaw, &expiresAtRaw)
	if err != nil {
		if err == sql.ErrNoRows {
			return project.ErrRecoveryTokenInvalid
		}
		return fmt.Errorf("select recovery token: %w", err)
	}
	if consumedAtRaw != "" {
		return project.ErrRecoveryTokenConsumed
	}
	expiresAt, err := parseTokenTime(expiresAtRaw)
	if err != nil {
		return fmt.Errorf("parse recovery token expiry: %w", err)
	}
	if !now.Before(expiresAt) {
		if _, err := tx.ExecContext(ctx, `DELETE FROM recovery_tokens WHERE expires_at <= ? AND consumed_at IS NULL`, formatTokenTime(now)); err != nil {
			return fmt.Errorf("delete expired recovery tokens: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit expired recovery token cleanup: %w", err)
		}
		return project.ErrRecoveryTokenExpired
	}
	if contextHash != validation.ContextHash {
		return project.ErrRecoveryTokenWrongContext
	}

	var candidates []project.Candidate
	if err := json.Unmarshal([]byte(candidatesJSON), &candidates); err != nil {
		return fmt.Errorf("unmarshal candidates: %w", err)
	}
	if !candidateIncludes(candidates, validation.SelectedProject) {
		return project.ErrRecoveryTokenNotCandidate
	}
	if !consume {
		return nil
	}

	res, err := tx.ExecContext(ctx, `UPDATE recovery_tokens SET consumed_at = ?, selected_project = ? WHERE token = ? AND consumed_at IS NULL`, formatTokenTime(now), validation.SelectedProject, validation.Token)
	if err != nil {
		return fmt.Errorf("consume recovery token: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("consume rows affected: %w", err)
	}
	if rows != 1 {
		return project.ErrRecoveryTokenConsumed
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit consume recovery token: %w", err)
	}
	return nil
}

func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate recovery token: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func candidateIncludes(candidates []project.Candidate, selected string) bool {
	for _, candidate := range candidates {
		if candidate.Project == selected {
			return true
		}
	}
	return false
}

func formatTokenTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTokenTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05", s)
}
