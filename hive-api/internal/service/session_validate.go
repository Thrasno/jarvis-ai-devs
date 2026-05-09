package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
)

// validateSessionAttribution resolves and validates the session_id for a memory
// being attributed to a project. R3-FIX-2 — closes the cross-project leak on the
// direct REST POST /memories path that previously bypassed resolveSessionID.
//
// Branches (mirrors syncService.resolveSessionID semantics):
//
//  1. Empty: lazy-create manual-save-{project}.
//  2. manual-save-{X}: must match the request project; lazy-create otherwise reject.
//  3. legacy-pre-lifecycle-{X}: must match the request project; otherwise reject.
//  4. Arbitrary id: GetSession; reject if not found, reject if its project differs.
//
// Returns the resolved session id (possibly the same as input, possibly newly created).
func validateSessionAttribution(
	ctx context.Context,
	sessionRepo repository.SessionRepository,
	sessionID, project string,
) (string, error) {
	if sessionID == "" {
		id, err := sessionRepo.EnsureManualSaveSession(ctx, project)
		if err != nil {
			return "", fmt.Errorf("ensure manual-save session: %w", err)
		}
		return id, nil
	}

	if strings.HasPrefix(sessionID, "manual-save-") {
		expected := "manual-save-" + project
		if sessionID != expected {
			return "", fmt.Errorf("%w: session %q vs request project %q", ErrSessionProjectMismatch, sessionID, project)
		}
		id, err := sessionRepo.EnsureManualSaveSession(ctx, project)
		if err != nil {
			return "", fmt.Errorf("ensure manual-save session: %w", err)
		}
		return id, nil
	}

	if strings.HasPrefix(sessionID, "legacy-pre-lifecycle-") {
		expected := "legacy-pre-lifecycle-" + project
		if sessionID != expected {
			return "", fmt.Errorf("%w: session %q vs request project %q", ErrSessionProjectMismatch, sessionID, project)
		}
		// fall through — verify existence below
	}

	sess, err := sessionRepo.GetSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", fmt.Errorf("%w: session %q", ErrSessionNotFound, sessionID)
		}
		return "", fmt.Errorf("get session %s: %w", sessionID, err)
	}
	if sess.Project != project {
		return "", fmt.Errorf("%w: session %q project=%q vs request project=%q",
			ErrSessionProjectMismatch, sessionID, sess.Project, project)
	}
	return sessionID, nil
}
