package project_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/project"
)

type fakeStore struct {
	known          []project.KnownProject
	sessionProject map[string]string
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
			name:  "known explicit project passes with canonical name",
			known: []project.KnownProject{{Name: "jarvis-dev"}},
			input: project.WriteInput{Project: "Jarvis Dev"},
			want:  "jarvis-dev",
		},
		{
			name: "normalized collision fails deterministically",
			known: []project.KnownProject{
				{Name: "jarvis-dev"},
				{Name: "jarvis_dev"},
			},
			input:    project.WriteInput{Project: "jarvis dev"},
			wantCode: project.CodeProjectAmbiguous,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := project.ValidateWriteProject(ctx, fakeStore{known: tt.known}, tt.input)
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

func TestValidateWriteProject_CanonicalPathAmbiguityFails(t *testing.T) {
	t.Parallel()

	store := fakeStore{known: []project.KnownProject{
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

	store := fakeStore{
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
