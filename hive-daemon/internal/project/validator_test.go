package project_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

type fakeStore struct {
	known           []project.KnownProject
	sessionProject  map[string]string
	createdTokens   []project.TokenRequest
	tokenCandidates []project.Candidate
	consumeFn       func(context.Context, project.TokenValidation) error
	aliases         map[string]string // source -> target
}

func (f fakeStore) KnownProjects(context.Context) ([]project.KnownProject, error) {
	return f.known, nil
}

func (f fakeStore) SessionProject(_ context.Context, sessionID string) (string, error) {
	if f.sessionProject == nil {
		return "", project.ErrSessionNotFound
	}
	projectName, ok := f.sessionProject[sessionID]
	if !ok {
		return "", project.ErrSessionNotFound
	}
	return projectName, nil
}

func (f *fakeStore) CreateRecoveryToken(_ context.Context, req project.TokenRequest) (string, error) {
	f.createdTokens = append(f.createdTokens, req)
	return "recovery-123", nil
}

func (f fakeStore) ConsumeRecoveryToken(ctx context.Context, validation project.TokenValidation) error {
	if err := f.ValidateRecoveryToken(ctx, validation); err != nil {
		return err
	}
	if f.consumeFn != nil {
		return f.consumeFn(ctx, validation)
	}
	return nil
}

func (f fakeStore) ValidateRecoveryToken(_ context.Context, validation project.TokenValidation) error {
	if f.tokenCandidates != nil {
		if !candidateIncludesProject(f.tokenCandidates, validation.SelectedProject) {
			return project.ErrRecoveryTokenNotCandidate
		}
	}
	return nil
}

func (f fakeStore) ResolveAlias(_ context.Context, source string) (string, bool, error) {
	if f.aliases == nil {
		return "", false, nil
	}
	target, ok := f.aliases[source]
	return target, ok, nil
}

func candidateIncludesProject(candidates []project.Candidate, selected string) bool {
	for _, candidate := range candidates {
		if candidate.Project == selected {
			return true
		}
	}
	return false
}

func TestValidateWriteProject_ExplicitProjectResolution(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tests := []struct {
		name     string
		known    []project.KnownProject
		input    project.WriteInput
		want     string
		wantCode project.ErrorCode
	}{
		{
			name:     "unknown explicit project fails",
			known:    []project.KnownProject{{Name: "jarvis-dev"}},
			input:    project.WriteInput{Project: "ghost-project"},
			wantCode: project.CodeProjectUnknown,
		},
		{
			name:  "known explicit project matches with unicode case folding",
			known: []project.KnownProject{{Name: "Straße"}},
			input: project.WriteInput{Project: "STRASSE"},
			want:  "Straße",
		},
		{
			name:     "space is not a substitute for a dash separator",
			known:    []project.KnownProject{{Name: "jarvis-dev"}},
			input:    project.WriteInput{Project: "Jarvis Dev"},
			wantCode: project.CodeProjectUnknown,
		},
		{
			name: "separator variants do not collide",
			known: []project.KnownProject{
				{Name: "jarvis-dev"},
				{Name: "jarvis_dev"},
			},
			input:    project.WriteInput{Project: "jarvis dev"},
			wantCode: project.CodeProjectUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := project.ValidateWriteProject(ctx, &fakeStore{known: tt.known}, tt.input)
			if tt.wantCode != "" {
				var validationErr *project.ValidationError
				if !errors.As(err, &validationErr) {
					t.Fatalf("error = %T %v, want ValidationError", err, err)
				}
				if validationErr.Code != tt.wantCode {
					t.Fatalf("error code = %q, want %q", validationErr.Code, tt.wantCode)
				}
				if result.Project != "" {
					t.Fatalf("result project = %q, want empty on failure", result.Project)
				}
				return
			}

			if err != nil {
				t.Fatalf("ValidateWriteProject returned error: %v", err)
			}
			if result.Project != tt.want {
				t.Fatalf("resolved project = %q, want %q", result.Project, tt.want)
			}
		})
	}
}

// TestValidateWriteProject_AliasResolution verifies that a source project that
// has an active alias is transparently redirected to the target project.
func TestValidateWriteProject_AliasResolution(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("aliased source resolves to target", func(t *testing.T) {
		t.Parallel()
		// "Bar" is the real project; "Foo" has an alias pointing to "Bar".
		// KnownProjects returns only "Bar" (aliased source is hidden).
		store := &fakeStore{
			known:   []project.KnownProject{{Name: "Bar"}},
			aliases: map[string]string{"Foo": "Bar"},
		}
		result, err := project.ValidateWriteProject(ctx, store, project.WriteInput{Project: "Foo"})
		if err != nil {
			t.Fatalf("ValidateWriteProject: %v", err)
		}
		if result.Project != "Bar" {
			t.Fatalf("resolved project = %q, want Bar", result.Project)
		}
	})

	t.Run("non-aliased project resolves normally", func(t *testing.T) {
		t.Parallel()
		store := &fakeStore{
			known:   []project.KnownProject{{Name: "Baz"}},
			aliases: map[string]string{},
		}
		result, err := project.ValidateWriteProject(ctx, store, project.WriteInput{Project: "Baz"})
		if err != nil {
			t.Fatalf("ValidateWriteProject: %v", err)
		}
		if result.Project != "Baz" {
			t.Fatalf("resolved project = %q, want Baz", result.Project)
		}
	})

	t.Run("unknown project with no alias returns error", func(t *testing.T) {
		t.Parallel()
		store := &fakeStore{
			known:   []project.KnownProject{{Name: "Bar"}},
			aliases: map[string]string{},
		}
		_, err := project.ValidateWriteProject(ctx, store, project.WriteInput{Project: "Ghost"})
		var validationErr *project.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("error = %T %v, want ValidationError", err, err)
		}
		if validationErr.Code != project.CodeProjectUnknown {
			t.Fatalf("error code = %q, want %q", validationErr.Code, project.CodeProjectUnknown)
		}
	})
}

func TestValidateWriteProject_CanonicalPathAmbiguityFails(t *testing.T) {
	t.Parallel()

	store := &fakeStore{known: []project.KnownProject{
		{Name: "alpha", Directory: "/tmp/worktree"},
		{Name: "beta", Directory: "/tmp/worktree/../worktree"},
	}}

	_, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{Directory: "/tmp/worktree"})
	var validationErr *project.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want ValidationError", err, err)
	}
	if validationErr.Code != project.CodeProjectAmbiguous {
		t.Fatalf("error code = %q, want %q", validationErr.Code, project.CodeProjectAmbiguous)
	}
	if len(validationErr.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(validationErr.Candidates))
	}
}

func TestValidateWriteProject_SessionProjectMismatchFails(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		known:          []project.KnownProject{{Name: "alpha"}, {Name: "beta"}},
		sessionProject: map[string]string{"sess-1": "alpha"},
	}

	_, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{Project: "beta", SessionID: "sess-1"})
	var validationErr *project.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want ValidationError", err, err)
	}
	if validationErr.Code != project.CodeProjectSessionMismatch {
		t.Fatalf("error code = %q, want %q", validationErr.Code, project.CodeProjectSessionMismatch)
	}
}

func TestValidateWriteProject_AmbiguityIssuesRecoveryToken(t *testing.T) {
	store := &fakeStore{known: []project.KnownProject{
		{Name: "jarvis-dev", Directory: "/repo/jarvis"},
		{Name: "JARVIS-DEV", Directory: "/repo/upper"},
	}}
	now := time.Date(2026, 5, 9, 18, 0, 0, 0, time.UTC)

	_, err := project.ValidateWriteProjectWithConfig(context.Background(), store, project.WriteInput{Project: "Jarvis-Dev", SessionID: "sess-1"}, project.ValidationConfig{
		Now:      func() time.Time { return now },
		TokenTTL: 15 * time.Minute,
	})

	var validationErr *project.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want ValidationError", err, err)
	}
	if validationErr.Code != project.CodeProjectAmbiguous {
		t.Fatalf("error code = %q, want %q", validationErr.Code, project.CodeProjectAmbiguous)
	}
	if validationErr.RecoveryToken != "recovery-123" {
		t.Fatalf("recovery token = %q, want recovery-123", validationErr.RecoveryToken)
	}
	if !validationErr.ExpiresAt.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("expires_at = %s, want %s", validationErr.ExpiresAt, now.Add(15*time.Minute))
	}
	if len(store.createdTokens) != 1 {
		t.Fatalf("created token requests = %d, want 1", len(store.createdTokens))
	}
	if store.createdTokens[0].RequestedProject != "Jarvis-Dev" {
		t.Fatalf("requested project = %q, want Jarvis-Dev", store.createdTokens[0].RequestedProject)
	}
}

func TestValidateWriteProject_RecoveryTokenRetryConsumesCandidate(t *testing.T) {
	var consumed project.TokenValidation
	store := &fakeStore{
		known: []project.KnownProject{{Name: "jarvis-dev"}},
		consumeFn: func(_ context.Context, validation project.TokenValidation) error {
			consumed = validation
			return nil
		},
	}

	result, err := project.ValidateWriteProjectWithConfig(context.Background(), store, project.WriteInput{
		Project:             "jarvis-dev",
		SessionID:           "sess-1",
		RecoveryToken:       "recovery-123",
		ProjectChoiceReason: "jarvis dev",
	}, project.ValidationConfig{})
	if err != nil {
		t.Fatalf("ValidateWriteProjectWithConfig: %v", err)
	}
	if result.Project != "jarvis-dev" {
		t.Fatalf("result project = %q, want jarvis-dev", result.Project)
	}
	if consumed.Token != "recovery-123" || consumed.SelectedProject != "jarvis-dev" {
		t.Fatalf("consumed token = %+v, want token recovery-123 selected jarvis-dev", consumed)
	}
}

func TestValidateWriteProject_RecoveryTokenRetryUsesExactCandidateWhenKnownProjectsNormalizeToSameName(t *testing.T) {
	var consumed project.TokenValidation
	store := &fakeStore{
		known: []project.KnownProject{
			{Name: "jarvis-dev"},
			{Name: "jarvis_dev"},
		},
		tokenCandidates: []project.Candidate{{Project: "jarvis-dev"}, {Project: "jarvis_dev"}},
		consumeFn: func(_ context.Context, validation project.TokenValidation) error {
			consumed = validation
			return nil
		},
	}

	result, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{
		Project:             "jarvis_dev",
		RecoveryToken:       "recovery-123",
		ProjectChoiceReason: "jarvis dev",
	})
	if err != nil {
		t.Fatalf("ValidateWriteProject: %v", err)
	}
	if result.Project != "jarvis_dev" {
		t.Fatalf("result project = %q, want jarvis_dev", result.Project)
	}
	if consumed.SelectedProject != "jarvis_dev" {
		t.Fatalf("consumed selected project = %q, want jarvis_dev", consumed.SelectedProject)
	}
}

func TestValidateWriteProject_RecoveryTokenRetryWrongExactCandidateFails(t *testing.T) {
	store := &fakeStore{
		known:           []project.KnownProject{{Name: "jarvis-dev"}, {Name: "jarvis_dev"}, {Name: "other-project"}},
		tokenCandidates: []project.Candidate{{Project: "jarvis-dev"}, {Project: "jarvis_dev"}},
	}

	_, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{
		Project:             "other-project",
		RecoveryToken:       "recovery-123",
		ProjectChoiceReason: "jarvis dev",
	})

	var validationErr *project.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want ValidationError", err, err)
	}
	if validationErr.Code != project.CodeRecoveryTokenNotCandidate {
		t.Fatalf("error code = %q, want %q", validationErr.Code, project.CodeRecoveryTokenNotCandidate)
	}
}

func TestValidateWriteProject_RecoveryTokenRetrySessionMismatchFailsBeforeConsume(t *testing.T) {
	consumed := false
	store := &fakeStore{
		known:           []project.KnownProject{{Name: "alpha"}, {Name: "beta"}, {Name: "beta.project"}},
		sessionProject:  map[string]string{"sess-1": "alpha"},
		tokenCandidates: []project.Candidate{{Project: "beta"}, {Project: "beta.project"}},
		consumeFn: func(context.Context, project.TokenValidation) error {
			consumed = true
			return nil
		},
	}

	_, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{
		Project:             "beta",
		SessionID:           "sess-1",
		RecoveryToken:       "recovery-123",
		ProjectChoiceReason: "ambiguous project",
	})

	var validationErr *project.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want ValidationError", err, err)
	}
	if validationErr.Code != project.CodeProjectSessionMismatch {
		t.Fatalf("error code = %q, want %q", validationErr.Code, project.CodeProjectSessionMismatch)
	}
	if consumed {
		t.Fatal("recovery token was consumed before session/project mismatch failed")
	}
}

func TestValidateWriteProject_RecoveryTokenFailuresMapToValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		code project.ErrorCode
	}{
		{name: "unknown", err: project.ErrRecoveryTokenInvalid, code: project.CodeRecoveryTokenInvalid},
		{name: "expired", err: project.ErrRecoveryTokenExpired, code: project.CodeRecoveryTokenExpired},
		{name: "wrong context", err: project.ErrRecoveryTokenWrongContext, code: project.CodeRecoveryTokenWrongContext},
		{name: "not candidate", err: project.ErrRecoveryTokenNotCandidate, code: project.CodeRecoveryTokenNotCandidate},
		{name: "consumed", err: project.ErrRecoveryTokenConsumed, code: project.CodeRecoveryTokenConsumed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{
				known: []project.KnownProject{{Name: "jarvis-dev"}},
				consumeFn: func(context.Context, project.TokenValidation) error {
					return tt.err
				},
			}

			_, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{
				Project:             "jarvis-dev",
				RecoveryToken:       "token",
				ProjectChoiceReason: "jarvis dev",
			})
			var validationErr *project.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T %v, want ValidationError", err, err)
			}
			if validationErr.Code != tt.code {
				t.Fatalf("error code = %q, want %q", validationErr.Code, tt.code)
			}
		})
	}
}

func TestParseRecoveryTokenTTL_DefaultValidAndInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    time.Duration
		wantErr bool
	}{
		{name: "default", value: "", want: 15 * time.Minute},
		{name: "custom", value: "30m", want: 30 * time.Minute},
		{name: "invalid", value: "soon", wantErr: true},
		{name: "non positive", value: "0s", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := project.ParseRecoveryTokenTTL(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRecoveryTokenTTL: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ttl = %s, want %s", got, tt.want)
			}
		})
	}
}
