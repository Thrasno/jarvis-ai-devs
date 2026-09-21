package db_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
)

func TestWorkspaceProjectBindingPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.db")
	workspace := filepath.Join(t.TempDir(), "workspace")

	first, err := hivedb.Open(path)
	if err != nil {
		t.Fatalf("open first DB: %v", err)
	}
	bound, inserted, err := first.EnsureWorkspaceProjectBinding(context.Background(), workspace, "Directory Identity")
	if err != nil {
		t.Fatalf("ensure binding: %v", err)
	}
	if !inserted || bound != "directory-identity" {
		t.Fatalf("first binding = (%q, %t), want (directory-identity, true)", bound, inserted)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first DB: %v", err)
	}

	restarted, err := hivedb.Open(path)
	if err != nil {
		t.Fatalf("open restarted DB: %v", err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	bound, found, err := restarted.ResolveWorkspaceProjectBinding(context.Background(), workspace)
	if err != nil {
		t.Fatalf("resolve restarted binding: %v", err)
	}
	if !found || bound != "directory-identity" {
		t.Fatalf("restarted binding = (%q, %t), want (directory-identity, true)", bound, found)
	}

	var tableCount int
	if err := restarted.RawDB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'workspace_project_bindings'`).Scan(&tableCount); err != nil {
		t.Fatalf("read binding schema: %v", err)
	}
	if tableCount != 1 {
		t.Fatalf("binding schema table count = %d, want 1", tableCount)
	}
}

func TestPromoteWorkspaceProjectMovesCoreStateAndRetriesAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hive.db")
	d, err := hivedb.Open(path)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	workspace := filepath.Join(t.TempDir(), "workspace")
	sibling := filepath.Join(t.TempDir(), "sibling")
	seedWorkspaceSourceState(t, d, "directory-project")
	if _, _, err := d.EnsureWorkspaceProjectBinding(ctx, workspace, "directory-project"); err != nil {
		t.Fatalf("ensure source binding: %v", err)
	}
	if _, _, err := d.EnsureWorkspaceProjectBinding(ctx, sibling, "directory-project"); err != nil {
		t.Fatalf("ensure sibling binding: %v", err)
	}

	changed, err := d.PromoteWorkspaceProject(ctx, workspace, "directory-project", "git-project")
	if err != nil || !changed {
		t.Fatalf("PromoteWorkspaceProject changed=%t err=%v, want true nil", changed, err)
	}
	assertWorkspaceProjectState(t, d, workspace, "directory-project", "git-project")
	boundSibling, foundSibling, err := d.ResolveWorkspaceProjectBinding(ctx, sibling)
	if err != nil || !foundSibling || boundSibling != "git-project" {
		t.Fatalf("sibling binding after promotion = (%q, %t, %v), want (git-project, true, nil)", boundSibling, foundSibling, err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("close promoted DB: %v", err)
	}
	d, err = hivedb.Open(path)
	if err != nil {
		t.Fatalf("reopen promoted DB: %v", err)
	}

	changed, err = d.PromoteWorkspaceProject(ctx, workspace, "directory-project", "git-project")
	if err != nil || changed {
		t.Fatalf("promotion retry changed=%t err=%v, want false nil", changed, err)
	}
}

func TestWorkspaceProjectBindingsResolveAliasesBeforePersistingOrReturning(t *testing.T) {
	ctx := context.Background()
	d := openWorkspaceBindingDB(t)
	if _, err := d.RawDB().Exec(`INSERT INTO project_aliases (source_project, target_project, scope, reason) VALUES ('old', 'new', 'local', 'test')`); err != nil {
		t.Fatalf("seed alias: %v", err)
	}
	workspace := filepath.Join(t.TempDir(), "derived-old")
	bound, inserted, err := d.EnsureWorkspaceProjectBinding(ctx, workspace, "old")
	if err != nil || !inserted || bound != "new" {
		t.Fatalf("aliased initial binding = (%q, %t, %v), want (new, true, nil)", bound, inserted, err)
	}
	var stored string
	if err := d.RawDB().QueryRow(`SELECT project FROM workspace_project_bindings WHERE workspace = ?`, workspace).Scan(&stored); err != nil {
		t.Fatalf("read initial binding: %v", err)
	}
	if stored != "new" {
		t.Fatalf("stored initial binding = %q, want new", stored)
	}

	persisted := filepath.Join(t.TempDir(), "persisted-old")
	if _, err := d.RawDB().Exec(`INSERT INTO workspace_project_bindings (workspace, project) VALUES (?, 'old')`, persisted); err != nil {
		t.Fatalf("seed stale binding: %v", err)
	}
	bound, found, err := d.ResolveWorkspaceProjectBinding(ctx, persisted)
	if err != nil || !found || bound != "new" {
		t.Fatalf("resolved stale binding = (%q, %t, %v), want (new, true, nil)", bound, found, err)
	}
}

func TestBindAndPromoteWorkspaceProjectRollsBackInitialBindingWhenProtected(t *testing.T) {
	ctx := context.Background()
	d := openWorkspaceBindingDB(t)
	workspace := filepath.Join(t.TempDir(), "legacy")
	seedWorkspaceSourceState(t, d, "legacy")
	if _, err := d.RawDB().Exec(`INSERT INTO sdd_apply_heads (project, change_name, snapshot_memory_id, generation, revision, digest) VALUES ('legacy', 'change', 1, 1, 1, 'digest')`); err != nil {
		t.Fatalf("seed protected head: %v", err)
	}

	changed, err := d.BindAndPromoteWorkspaceProject(ctx, workspace, "legacy", "git-target")
	if changed || !errors.Is(err, hivedb.ErrWorkspacePromotionProtectedSDD) {
		t.Fatalf("protected bind-and-promote = (%t, %v), want false protected error", changed, err)
	}
	if _, found, err := d.ResolveWorkspaceProjectBinding(ctx, workspace); err != nil || found {
		t.Fatalf("binding after protected rollback = (%t, %v), want false nil", found, err)
	}
	var aliases, governance int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM project_aliases WHERE source_project = 'legacy'`).Scan(&aliases); err != nil {
		t.Fatalf("count aliases: %v", err)
	}
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM hive_project_governance WHERE project = 'legacy'`).Scan(&governance); err != nil {
		t.Fatalf("count governance: %v", err)
	}
	if aliases != 0 || governance != 0 {
		t.Fatalf("protected rollback persisted alias:%d governance:%d, want zero", aliases, governance)
	}
}

func TestPromoteWorkspaceProjectRejectsChangedBindingWithoutMutation(t *testing.T) {
	ctx := context.Background()
	d := openWorkspaceBindingDB(t)
	workspace := filepath.Join(t.TempDir(), "workspace")
	seedWorkspaceSourceState(t, d, "source")
	if _, _, err := d.EnsureWorkspaceProjectBinding(ctx, workspace, "source"); err != nil {
		t.Fatalf("ensure source binding: %v", err)
	}

	changed, err := d.PromoteWorkspaceProject(ctx, workspace, "other-source", "target")
	if changed {
		t.Fatal("promotion changed a workspace whose binding no longer matches")
	}
	if !errors.Is(err, hivedb.ErrWorkspaceProjectBindingConflict) {
		t.Fatalf("promotion error = %v, want binding conflict", err)
	}
	var sourceRows int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = 'source'`).Scan(&sourceRows); err != nil {
		t.Fatalf("count source rows: %v", err)
	}
	if sourceRows != 1 {
		t.Fatalf("source rows after conflict = %d, want 1", sourceRows)
	}
}

func TestPromoteWorkspaceProjectFailsClosedForProtectedSDDProgress(t *testing.T) {
	for _, tt := range []struct {
		name string
		seed func(*testing.T, *hivedb.DB)
	}{
		{
			name: "apply head on source",
			seed: func(t *testing.T, d *hivedb.DB) {
				t.Helper()
				if _, err := d.RawDB().Exec(`INSERT INTO sdd_apply_heads (project, change_name, snapshot_memory_id, generation, revision, digest) VALUES ('source', 'change', 1, 1, 1, 'digest')`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "receipt on target",
			seed: func(t *testing.T, d *hivedb.DB) {
				t.Helper()
				if _, err := d.RawDB().Exec(`INSERT INTO sdd_apply_receipts (request_id, project, change_name, payload_sha256, response_json) VALUES ('receipt', 'target', 'change', 'digest', '{}')`); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "apply progress memory on source",
			seed: func(t *testing.T, d *hivedb.DB) {
				t.Helper()
				if _, err := d.RawDB().Exec(`INSERT INTO memories (sync_id, project, topic_key, title, content, session_id) VALUES ('progress', 'source', 'sdd/change/apply-progress/v2', 'immutable', '{}', 'manual-save-source')`); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			d := openWorkspaceBindingDB(t)
			workspace := filepath.Join(t.TempDir(), "workspace")
			seedWorkspaceSourceState(t, d, "source")
			if _, _, err := d.EnsureWorkspaceProjectBinding(ctx, workspace, "source"); err != nil {
				t.Fatalf("ensure source binding: %v", err)
			}
			tt.seed(t, d)

			changed, err := d.PromoteWorkspaceProject(ctx, workspace, "source", "target")
			if changed {
				t.Fatal("protected promotion changed state")
			}
			if !errors.Is(err, hivedb.ErrWorkspacePromotionProtectedSDD) {
				t.Fatalf("promotion error = %v, want protected SDD sentinel", err)
			}
			var protected *hivedb.WorkspacePromotionProtectedError
			if !errors.As(err, &protected) {
				t.Fatalf("promotion error = %T, want typed protected error", err)
			}
			if !strings.Contains(err.Error(), "issue #724") {
				t.Fatalf("recovery message = %q, want issue #724 dependency", err)
			}
			bound, found, resolveErr := d.ResolveWorkspaceProjectBinding(ctx, workspace)
			if resolveErr != nil || !found || bound != "source" {
				t.Fatalf("binding after protected rejection = (%q, %t, %v), want source true nil", bound, found, resolveErr)
			}
		})
	}
}

func openWorkspaceBindingDB(t *testing.T) *hivedb.DB {
	t.Helper()
	d, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func seedWorkspaceSourceState(t *testing.T, d *hivedb.DB, project string) {
	t.Helper()
	if _, err := d.EnsureManualSaveSession(project); err != nil {
		t.Fatalf("ensure manual session: %v", err)
	}
	sessionID := "manual-save-" + project
	if _, err := d.SavePrompt(context.Background(), project, "source prompt"); err != nil {
		t.Fatalf("save prompt: %v", err)
	}
	if _, err := d.RawDB().Exec(`INSERT INTO memories (sync_id, project, title, content, session_id) VALUES (?, ?, 'source memory', 'content', ?)`, "workspace-memory-"+project, project, sessionID); err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	if _, err := d.RawDB().Exec(`INSERT INTO memory_mutations (event_id, entity_sync_id, project, op) VALUES (?, ?, ?, 'save')`, "workspace-mutation-"+project, "workspace-memory-"+project, project); err != nil {
		t.Fatalf("seed pending mutation: %v", err)
	}
	if _, err := d.RawDB().Exec(`INSERT INTO sync_state (project, last_error) VALUES (?, 'pending')`, project); err != nil {
		t.Fatalf("seed sync state: %v", err)
	}
}

func assertWorkspaceProjectState(t *testing.T, d *hivedb.DB, workspace, source, target string) {
	t.Helper()
	for _, table := range []string{"memories", "user_prompts", "sessions", "memory_mutations"} {
		var sourceRows, targetRows int
		if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE project = ?`, source).Scan(&sourceRows); err != nil {
			t.Fatalf("count source %s: %v", table, err)
		}
		if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE project = ?`, target).Scan(&targetRows); err != nil {
			t.Fatalf("count target %s: %v", table, err)
		}
		if sourceRows != 0 || targetRows == 0 {
			t.Fatalf("%s projects = source:%d target:%d, want source:0 target:>0", table, sourceRows, targetRows)
		}
	}
	var sourceSyncRows int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM sync_state WHERE project = ?`, source).Scan(&sourceSyncRows); err != nil {
		t.Fatalf("count source sync state: %v", err)
	}
	if sourceSyncRows != 0 {
		t.Fatalf("source sync state rows = %d, want 0", sourceSyncRows)
	}
	bound, found, err := d.ResolveWorkspaceProjectBinding(context.Background(), workspace)
	if err != nil || !found || bound != target {
		t.Fatalf("binding after promotion = (%q, %t, %v), want (%q, true, nil)", bound, found, err, target)
	}
	destination, aliased, err := d.ResolveAlias(context.Background(), source)
	if err != nil || !aliased || destination != target {
		t.Fatalf("alias after promotion = (%q, %t, %v), want (%q, true, nil)", destination, aliased, err, target)
	}
}
