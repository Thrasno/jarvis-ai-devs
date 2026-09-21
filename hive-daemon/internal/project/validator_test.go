package project_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
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

func initValidatorGitRepo(t *testing.T, dir, remoteURL string) {
	t.Helper()
	for _, args := range [][]string{{"init"}, {"remote", "add", "origin", remoteURL}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
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
			name:  "space is a substitute for a dash separator",
			known: []project.KnownProject{{Name: "jarvis-dev"}},
			input: project.WriteInput{Project: "Jarvis Dev"},
			want:  "jarvis-dev",
		},
		{
			name: "separator variants collide and require recovery",
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
func TestValidateWriteProjectBindsAndPromotesWorkspaceGitIdentity(t *testing.T) {
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	workspace := t.TempDir()

	first, err := project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: workspace})
	if err != nil {
		t.Fatalf("first workspace validation: %v", err)
	}
	if want := projectidentity.Canonical(filepath.Base(workspace)).String(); first.Project != want {
		t.Fatalf("first project = %q, want %q", first.Project, want)
	}
	if _, err := d.EnsureManualSaveSession(first.Project); err != nil {
		t.Fatalf("ensure source session: %v", err)
	}
	if err := d.CreateSession("source-session", first.Project, workspace, "dev", "test"); err != nil {
		t.Fatalf("create source session: %v", err)
	}
	if _, err := d.SaveMemory(&models.Memory{Project: first.Project, Title: "preserved", Content: "content", SessionID: "manual-save-" + first.Project}); err != nil {
		t.Fatalf("save source memory: %v", err)
	}

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	initValidatorGitRepo(t, workspace, "https://github.com/org/git-identity.git")
	promoted, err := project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: workspace, SessionID: "source-session"})
	if err != nil {
		t.Fatalf("promoted workspace validation: %v", err)
	}
	if promoted.Project != "git-identity" {
		t.Fatalf("promoted project = %q, want git-identity", promoted.Project)
	}
	bound, found, err := d.ResolveWorkspaceProjectBinding(ctx, workspace)
	if err != nil || !found || bound != "git-identity" {
		t.Fatalf("binding after promotion = (%q, %t, %v), want (git-identity, true, nil)", bound, found, err)
	}
	var sourceRows, targetRows int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = ?`, first.Project).Scan(&sourceRows); err != nil {
		t.Fatalf("count source memories: %v", err)
	}
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = 'git-identity'`).Scan(&targetRows); err != nil {
		t.Fatalf("count target memories: %v", err)
	}
	if sourceRows != 0 || targetRows != 1 {
		t.Fatalf("memory migration = source:%d target:%d, want source:0 target:1", sourceRows, targetRows)
	}
}

func TestValidateWriteProjectRecreatesFreshWorkspaceAfterPurgingPromotedTarget(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	workspace := t.TempDir()

	first, err := project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: workspace})
	if err != nil {
		t.Fatalf("initial directory validation: %v", err)
	}
	if _, err := d.EnsureManualSaveSession(first.Project); err != nil {
		t.Fatalf("ensure source session: %v", err)
	}
	if err := d.CreateSession("purge-source-session", first.Project, workspace, "dev", "test"); err != nil {
		t.Fatalf("create source session: %v", err)
	}
	if _, err := d.SaveMemory(&models.Memory{Project: first.Project, SessionID: "manual-save-" + first.Project, Title: "retired history", Content: "must purge"}); err != nil {
		t.Fatalf("save source memory: %v", err)
	}
	// An acknowledged mutation intentionally remains under the predecessor after
	// promotion, which makes this exercise the completed local-purge boundary.
	if _, err := d.RawDB().Exec(`UPDATE memory_mutations SET synced_at = CURRENT_TIMESTAMP WHERE project = ?`, first.Project); err != nil {
		t.Fatalf("acknowledge predecessor mutation: %v", err)
	}

	initValidatorGitRepo(t, workspace, "https://github.com/org/fresh-git.git")
	promoted, err := project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: workspace, SessionID: "purge-source-session"})
	if err != nil || promoted.Project != "fresh-git" {
		t.Fatalf("promoted validation = (%+v, %v), want fresh-git nil", promoted, err)
	}
	if _, err := d.ArchiveGovernanceProject(ctx, promoted.Project, "tester", "purge promoted workspace", time.Now().UTC()); err != nil {
		t.Fatalf("archive promoted project: %v", err)
	}
	if deleted, err := d.DeleteGovernanceProject(ctx, promoted.Project, "tester", "purge promoted workspace"); err != nil || deleted == 0 {
		t.Fatalf("purge promoted project = (%d, %v), want positive nil", deleted, err)
	}

	for _, check := range []struct {
		label string
		query string
		args  []any
	}{
		{"identities", `SELECT COUNT(*) FROM project_identities WHERE project_key IN (?, 'fresh-git')`, []any{first.Project}},
		{"aliases", `SELECT COUNT(*) FROM project_aliases WHERE source_project IN (?, 'fresh-git') OR target_project IN (?, 'fresh-git')`, []any{first.Project, first.Project}},
		{"governance", `SELECT COUNT(*) FROM hive_project_governance WHERE project IN (?, 'fresh-git') OR merge_target IN (?, 'fresh-git')`, []any{first.Project, first.Project}},
		{"mutations", `SELECT COUNT(*) FROM memory_mutations WHERE project IN (?, 'fresh-git')`, []any{first.Project}},
		{"project state", `SELECT COUNT(*) FROM memories WHERE project IN (?, 'fresh-git') UNION ALL SELECT COUNT(*) FROM sessions WHERE project IN (?, 'fresh-git') UNION ALL SELECT COUNT(*) FROM user_prompts WHERE project IN (?, 'fresh-git') UNION ALL SELECT COUNT(*) FROM workspace_project_bindings WHERE project IN (?, 'fresh-git')`, []any{first.Project, first.Project, first.Project, first.Project}},
	} {
		label := check.label
		rows, err := d.RawDB().Query(check.query, check.args...)
		if err != nil {
			t.Fatalf("read purged %s: %v", label, err)
		}
		for rows.Next() {
			var count int
			if err := rows.Scan(&count); err != nil {
				_ = rows.Close()
				t.Fatalf("scan purged %s: %v", label, err)
			}
			if count != 0 {
				_ = rows.Close()
				t.Fatalf("purged %s count = %d, want 0", label, count)
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close purged %s rows: %v", label, err)
		}
	}

	fresh, err := project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: workspace})
	if err != nil || fresh.Project != "fresh-git" {
		t.Fatalf("fresh no-project validation = (%+v, %v), want fresh-git nil", fresh, err)
	}
	bound, found, err := d.ResolveWorkspaceProjectBinding(ctx, workspace)
	if err != nil || !found || bound != "fresh-git" {
		t.Fatalf("fresh workspace binding = (%q, %t, %v), want fresh-git true nil", bound, found, err)
	}
	// The normal resolver supplied the freshly discovered Git key without an
	// explicit project. Its following write creates only fresh B-owned state.
	if _, err := d.SaveMemoryWithManualSession(&models.Memory{Project: fresh.Project, Title: "fresh history", Content: "new"}); err != nil {
		t.Fatalf("save fresh resolved project: %v", err)
	}
	for label, query := range map[string]string{
		"retired identity": `SELECT COUNT(*) FROM project_identities WHERE project_key = ?`,
		"fresh identity":   `SELECT COUNT(*) FROM project_identities WHERE project_key = 'fresh-git'`,
		"aliases":          `SELECT COUNT(*) FROM project_aliases`,
		"governance":       `SELECT COUNT(*) FROM hive_project_governance`,
		"mutations":        `SELECT COUNT(*) FROM memory_mutations`,
		"memories":         `SELECT COUNT(*) FROM memories`,
		"sessions":         `SELECT COUNT(*) FROM sessions`,
		"prompts":          `SELECT COUNT(*) FROM user_prompts`,
	} {
		var count int
		args := []any{}
		if label == "retired identity" {
			args = append(args, first.Project)
		}
		if err := d.RawDB().QueryRow(query, args...).Scan(&count); err != nil {
			t.Fatalf("count fresh %s: %v", label, err)
		}
		want := 0
		if label == "fresh identity" || label == "memories" || label == "sessions" || label == "mutations" {
			want = 1
		}
		if count != want {
			t.Fatalf("fresh %s count = %d, want %d", label, count, want)
		}
	}
}

func TestValidateWriteProjectAdoptsHistoricalDirectoryBeforeGitPromotion(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	workspace := t.TempDir()
	if err := d.CreateSession("legacy-session", "legacy-directory", workspace, "dev", "test"); err != nil {
		t.Fatalf("create historical session: %v", err)
	}
	initValidatorGitRepo(t, workspace, "https://github.com/org/git-target.git")

	result, err := project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: workspace, SessionID: "legacy-session"})
	if err != nil {
		t.Fatalf("validate historical workspace: %v", err)
	}
	if result.Project != "git-target" {
		t.Fatalf("project = %q, want git-target", result.Project)
	}
	alias, found, err := d.ResolveAlias(ctx, "legacy-directory")
	if err != nil || !found || alias != "git-target" {
		t.Fatalf("historical alias = (%q, %t, %v), want (git-target, true, nil)", alias, found, err)
	}
	var legacyRows, targetRows int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE project = 'legacy-directory'`).Scan(&legacyRows); err != nil {
		t.Fatalf("count legacy sessions: %v", err)
	}
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE project = 'git-target'`).Scan(&targetRows); err != nil {
		t.Fatalf("count target sessions: %v", err)
	}
	if legacyRows != 0 || targetRows != 1 {
		t.Fatalf("session migration = legacy:%d target:%d, want legacy:0 target:1", legacyRows, targetRows)
	}
}

func TestValidateWriteProjectRejectsUnrelatedSessionBeforePromotionMutation(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	workspace := t.TempDir()
	if _, _, err := d.EnsureWorkspaceProjectBinding(ctx, workspace, "source"); err != nil {
		t.Fatalf("bind source workspace: %v", err)
	}
	if err := d.CreateSession("unrelated-session", "unrelated", workspace, "dev", "test"); err != nil {
		t.Fatalf("create unrelated session: %v", err)
	}
	initValidatorGitRepo(t, workspace, "https://github.com/org/target.git")

	_, err = project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: workspace, SessionID: "unrelated-session"})
	var validationErr *project.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != project.CodeProjectSessionMismatch {
		t.Fatalf("validation error = %v, want typed session mismatch", err)
	}
	bound, found, resolveErr := d.ResolveWorkspaceProjectBinding(ctx, workspace)
	if resolveErr != nil || !found || bound != "source" {
		t.Fatalf("binding after rejected validation = (%q, %t, %v), want (source, true, nil)", bound, found, resolveErr)
	}
	if _, found, err := d.ResolveAlias(ctx, "source"); err != nil || found {
		t.Fatalf("source alias after rejected validation = (%t, %v), want (false, nil)", found, err)
	}
	var governanceRows int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM hive_project_governance`).Scan(&governanceRows); err != nil {
		t.Fatalf("count governance rows: %v", err)
	}
	if governanceRows != 0 {
		t.Fatalf("governance rows after rejected validation = %d, want 0", governanceRows)
	}
}

func TestValidateWriteProjectPromotesSiblingBindingsAcrossRestart(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hive.db")
	d, err := hivedb.Open(path)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	gitWorkspace := t.TempDir()
	nonGitSibling := t.TempDir()
	if _, _, err := d.EnsureWorkspaceProjectBinding(ctx, gitWorkspace, "source"); err != nil {
		t.Fatalf("bind Git workspace: %v", err)
	}
	if _, _, err := d.EnsureWorkspaceProjectBinding(ctx, nonGitSibling, "source"); err != nil {
		t.Fatalf("bind non-Git sibling: %v", err)
	}
	initValidatorGitRepo(t, gitWorkspace, "https://github.com/org/target.git")
	if result, err := project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: gitWorkspace}); err != nil || result.Project != "target" {
		t.Fatalf("promote Git workspace = (%+v, %v), want target nil", result, err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("close promoted DB: %v", err)
	}
	d, err = hivedb.Open(path)
	if err != nil {
		t.Fatalf("reopen DB: %v", err)
	}

	result, err := project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: nonGitSibling})
	if err != nil || result.Project != "target" {
		t.Fatalf("non-Git sibling after restart = (%+v, %v), want target nil", result, err)
	}
	bound, found, err := d.ResolveWorkspaceProjectBinding(ctx, nonGitSibling)
	if err != nil || !found || bound != "target" {
		t.Fatalf("sibling binding after restart = (%q, %t, %v), want (target, true, nil)", bound, found, err)
	}
}

func TestValidateWriteProjectRejectsHistoricalUnboundBasenameAsExplicitEvidence(t *testing.T) {
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	workspace := filepath.Join(t.TempDir(), "basename-a")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatalf("make workspace: %v", err)
	}
	if err := d.CreateSession("historical-session", "historical-b", workspace, "dev", "test"); err != nil {
		t.Fatalf("seed historical project: %v", err)
	}

	_, err = project.ValidateWriteProject(ctx, d, project.WriteInput{Project: filepath.Base(workspace), Directory: workspace})
	var validationErr *project.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != project.CodeProjectIdentityMismatch {
		t.Fatalf("historical basename conflict = %v, want typed project identity mismatch", err)
	}
}

func TestValidateWriteProjectResolvesInitialAliasBeforeSessionValidation(t *testing.T) {
	for _, tt := range []struct {
		name           string
		sessionProject string
		wantErr        bool
	}{
		{name: "active target session is accepted", sessionProject: "new"},
		{name: "retired source session is rejected", sessionProject: "old", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			d, err := hivedb.Open(":memory:")
			if err != nil {
				t.Fatalf("open DB: %v", err)
			}
			t.Cleanup(func() { _ = d.Close() })
			if _, err := d.RawDB().Exec(`INSERT INTO project_aliases (source_project, target_project, scope, reason) VALUES ('old', 'new', 'local', 'test')`); err != nil {
				t.Fatalf("seed alias: %v", err)
			}
			workspace := filepath.Join(t.TempDir(), "old")
			if err := os.Mkdir(workspace, 0o755); err != nil {
				t.Fatalf("make workspace: %v", err)
			}
			// Seed the historical pre-WU-04 row directly: supported ingress now
			// redirects old to new, so CreateSession can no longer create the
			// retired-session fixture this validator guard must reject.
			if _, err := d.RawDB().Exec(`INSERT INTO sessions (id, sync_id, project, directory, dev_id, client) VALUES (?, ?, ?, ?, 'dev', 'test')`, "candidate-session", "candidate-sync", tt.sessionProject, filepath.Join(t.TempDir(), "other")); err != nil {
				t.Fatalf("seed session: %v", err)
			}

			result, err := project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: workspace, SessionID: "candidate-session"})
			if tt.wantErr {
				var validationErr *project.ValidationError
				if !errors.As(err, &validationErr) || validationErr.Code != project.CodeProjectSessionMismatch {
					t.Fatalf("retired alias session error = %v, want typed session mismatch", err)
				}
				return
			}
			if err != nil || result.Project != "new" {
				t.Fatalf("active alias session result = (%+v, %v), want new nil", result, err)
			}
		})
	}
}

func TestValidateWriteProjectResolvesAliasesBeforeBindingOrReturning(t *testing.T) {
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if _, err := d.RawDB().Exec(`INSERT INTO project_aliases (source_project, target_project, scope, reason) VALUES ('old', 'new', 'local', 'test')`); err != nil {
		t.Fatalf("seed alias: %v", err)
	}
	derivedOld := filepath.Join(t.TempDir(), "old")
	if err := os.Mkdir(derivedOld, 0o755); err != nil {
		t.Fatalf("make derived workspace: %v", err)
	}
	result, err := project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: derivedOld})
	if err != nil || result.Project != "new" {
		t.Fatalf("derived alias result = (%+v, %v), want new nil", result, err)
	}
	var stored string
	if err := d.RawDB().QueryRow(`SELECT project FROM workspace_project_bindings WHERE workspace = ?`, derivedOld).Scan(&stored); err != nil {
		t.Fatalf("read initial binding: %v", err)
	}
	if stored != "new" {
		t.Fatalf("initial binding = %q, want new", stored)
	}

	persisted := t.TempDir()
	if _, err := d.RawDB().Exec(`INSERT INTO workspace_project_bindings (workspace, project) VALUES (?, 'old')`, persisted); err != nil {
		t.Fatalf("seed stale binding: %v", err)
	}
	result, err = project.ValidateWriteProject(ctx, d, project.WriteInput{Project: "new", Directory: persisted})
	if err != nil || result.Project != "new" {
		t.Fatalf("persisted alias result = (%+v, %v), want new nil", result, err)
	}
}

func TestValidateWriteProjectRejectsBasenameAsExplicitEvidenceForBoundWorkspace(t *testing.T) {
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	workspace := t.TempDir()
	if _, _, err := d.EnsureWorkspaceProjectBinding(ctx, workspace, "bound-project"); err != nil {
		t.Fatalf("seed binding: %v", err)
	}

	_, err = project.ValidateWriteProject(ctx, d, project.WriteInput{Project: filepath.Base(workspace), Directory: workspace})
	var validationErr *project.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != project.CodeProjectIdentityMismatch {
		t.Fatalf("basename conflict = %v, want typed project identity mismatch", err)
	}
}

func TestValidateWriteProjectRollsBackInitialBindingWhenProtectedPromotionFails(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	ctx := context.Background()
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	workspace := t.TempDir()
	if err := d.CreateSession("legacy-session", "legacy", workspace, "dev", "test"); err != nil {
		t.Fatalf("seed legacy session: %v", err)
	}
	if _, err := d.RawDB().Exec(`INSERT INTO sdd_apply_heads (project, change_name, snapshot_memory_id, generation, revision, digest) VALUES ('legacy', 'change', 1, 1, 1, 'digest')`); err != nil {
		t.Fatalf("seed protected head: %v", err)
	}
	initValidatorGitRepo(t, workspace, "https://github.com/org/git-target.git")

	_, err = project.ValidateWriteProject(ctx, d, project.WriteInput{Directory: workspace, SessionID: "legacy-session"})
	if !errors.Is(err, hivedb.ErrWorkspacePromotionProtectedSDD) {
		t.Fatalf("protected validation error = %v, want protected SDD error", err)
	}
	if _, found, resolveErr := d.ResolveWorkspaceProjectBinding(ctx, workspace); resolveErr != nil || found {
		t.Fatalf("binding after protected validator rollback = (%t, %v), want false nil", found, resolveErr)
	}
	var aliases, governance int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM project_aliases WHERE source_project = 'legacy'`).Scan(&aliases); err != nil {
		t.Fatalf("count aliases: %v", err)
	}
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM hive_project_governance WHERE project = 'legacy'`).Scan(&governance); err != nil {
		t.Fatalf("count governance: %v", err)
	}
	if aliases != 0 || governance != 0 {
		t.Fatalf("protected validator rollback persisted alias:%d governance:%d, want zero", aliases, governance)
	}
}

func TestValidateWriteProjectRejectsExplicitConflictForUnboundWorkspace(t *testing.T) {
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	_, err = project.ValidateWriteProject(context.Background(), d, project.WriteInput{Project: "other-project", Directory: t.TempDir()})
	var validationErr *project.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want ValidationError", err, err)
	}
	if validationErr.Code != project.CodeProjectIdentityMismatch {
		t.Fatalf("error code = %q, want project identity mismatch", validationErr.Code)
	}
}

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

func TestValidateWriteProject_AliasSourceCorroboratesMatchingDirectory(t *testing.T) {
	t.Parallel()

	directory := filepath.Join(t.TempDir(), "alias-source")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatalf("mkdir directory: %v", err)
	}
	store := &fakeStore{
		known:   []project.KnownProject{{Name: "alias-target"}},
		aliases: map[string]string{"alias-source": "alias-target"},
	}

	result, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{
		Project:   "alias-source",
		Directory: directory,
	})
	if err != nil {
		t.Fatalf("ValidateWriteProject: %v", err)
	}
	if result.Project != "alias-target" {
		t.Fatalf("resolved project = %q, want alias-target", result.Project)
	}
}

func TestValidateWriteProject_DirectoryMatchWinsOverFreshDerivation(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	store := &fakeStore{known: []project.KnownProject{{
		Name:      "legacy-project-name",
		Directory: directory,
	}}}

	result, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{Directory: directory})
	if err != nil {
		t.Fatalf("ValidateWriteProject: %v", err)
	}
	if result.Project != "legacy-project-name" {
		t.Fatalf("resolved project = %q, want legacy-project-name", result.Project)
	}
}

func TestValidateWriteProject_ExplicitDirectoryBindingPrecedence(t *testing.T) {
	t.Parallel()

	t.Run("matching explicit project accepts historical directory identity", func(t *testing.T) {
		directory := t.TempDir()
		store := &fakeStore{known: []project.KnownProject{{Name: "legacy-project", Directory: directory}}}
		result, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{
			Project:   "Legacy Project",
			Directory: directory,
		})
		if err != nil {
			t.Fatalf("ValidateWriteProject: %v", err)
		}
		if result.Project != "legacy-project" {
			t.Fatalf("resolved project = %q, want legacy-project", result.Project)
		}
	})

	t.Run("alias source redirects to historical directory target", func(t *testing.T) {
		directory := t.TempDir()
		store := &fakeStore{
			known:   []project.KnownProject{{Name: "legacy-project", Directory: directory}},
			aliases: map[string]string{"old-project": "legacy-project"},
		}
		result, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{
			Project:   "old-project",
			Directory: directory,
		})
		if err != nil {
			t.Fatalf("ValidateWriteProject: %v", err)
		}
		if result.Project != "legacy-project" {
			t.Fatalf("resolved project = %q, want legacy-project", result.Project)
		}
	})

	t.Run("directory binding disambiguates explicit name", func(t *testing.T) {
		directory := t.TempDir()
		store := &fakeStore{known: []project.KnownProject{
			{Name: "legacy-project", Directory: directory},
			{Name: "legacy_project", Directory: filepath.Join(t.TempDir(), "other")},
		}}
		result, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{
			Project:   "legacy project",
			Directory: directory,
		})
		if err != nil {
			t.Fatalf("ValidateWriteProject: %v", err)
		}
		if result.Project != "legacy-project" {
			t.Fatalf("resolved project = %q, want directory-bound legacy-project", result.Project)
		}
		if len(store.createdTokens) != 0 {
			t.Fatalf("created recovery tokens = %d, want 0", len(store.createdTokens))
		}
	})

	t.Run("different explicit project rejects historical directory identity", func(t *testing.T) {
		directory := t.TempDir()
		store := &fakeStore{known: []project.KnownProject{{Name: "legacy-project", Directory: directory}}}
		_, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{
			Project:   "other-project",
			Directory: directory,
		})
		var validationErr *project.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("error = %T %v, want ValidationError", err, err)
		}
		if validationErr.Code != project.CodeProjectIdentityMismatch {
			t.Fatalf("error code = %q, want %q", validationErr.Code, project.CodeProjectIdentityMismatch)
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

func TestValidateWriteProject_ExplicitProjectAndDirectoryMismatchIsTyped(t *testing.T) {
	t.Parallel()

	store := &fakeStore{known: []project.KnownProject{{Name: "known-project"}}}
	_, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{
		Project:   "different-project",
		Directory: t.TempDir(),
	})

	var validationErr *project.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want ValidationError", err, err)
	}
	if validationErr.Code != "project_identity_mismatch" {
		t.Fatalf("error code = %q, want project_identity_mismatch", validationErr.Code)
	}
	if len(validationErr.Candidates) != 2 {
		t.Fatalf("candidates = %v, want explicit and directory identities", validationErr.Candidates)
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
