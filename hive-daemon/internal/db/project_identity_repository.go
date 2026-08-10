package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

type projectIdentityExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func canonicalProjectKey(project string) string {
	return projectidentity.Canonical(project).String()
}

func registerProjectIdentity(ctx context.Context, execer projectIdentityExecer, project string) (string, error) {
	spelling := strings.TrimSpace(project)
	key := canonicalProjectKey(spelling)
	if key == "" {
		return "", fmt.Errorf("project is required")
	}
	if _, err := execer.ExecContext(ctx, `
		INSERT INTO project_identities (project_key, first_spelling, first_seen_at, first_source)
		VALUES (?, ?, CURRENT_TIMESTAMP, 'repository')
		ON CONFLICT(project_key) DO NOTHING`, key, spelling); err != nil {
		return "", fmt.Errorf("register project identity: %w", err)
	}
	return key, nil
}
