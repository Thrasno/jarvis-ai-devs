package db_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

func TestArchiveGovernanceProjectPreservesLifecycleStateAndWaitsForLifecycleLease(t *testing.T) {
	ctx := context.Background()
	d := openGovernanceTestDB(t)
	const project = "archive-project"
	saveGovernanceTestMemory(t, d, project, "archive target")
	mustPurgeExec(t, d, `INSERT INTO workspace_project_bindings (workspace, project) VALUES ('/work/archive', ?)`, project)
	mustPurgeExec(t, d, `INSERT INTO project_aliases (source_project, target_project, scope, reason) VALUES ('archive-old', ?, 'local', 'test')`, project)
	mustPurgeExec(t, d, `INSERT INTO sdd_store_bindings (project, change_name, schema_version, mode, provenance) VALUES (?, 'change', '1', 'hive', 'test')`, project)
	mustPurgeExec(t, d, `INSERT INTO project_blocks (canonical_project_key, project, command_id, blocked_at) VALUES (?, ?, 'command', CURRENT_TIMESTAMP)`, project, project)
	mustPurgeExec(t, d, `INSERT INTO memory_mutations (event_id, entity_sync_id, project, op) VALUES ('archive-event', 'archive-sync', ?, 'save')`, project)
	mustPurgeExec(t, d, `INSERT INTO memory_remote_presence (entity_sync_id, confirmed_at, source) VALUES ('archive-sync', CURRENT_TIMESTAMP, 'mutation_accept')`)
	mustPurgeExec(t, d, `INSERT INTO sdd_apply_heads (project, change_name, snapshot_memory_id, generation, revision, digest) SELECT ?, 'change', id, 1, 1, 'digest' FROM memories WHERE project = ? LIMIT 1`, project, project)
	mustPurgeExec(t, d, `INSERT INTO sdd_apply_receipts (request_id, project, change_name, payload_sha256, response_json) VALUES ('archive-apply-receipt', ?, 'change', 'digest', '{}')`, project)

	before := purgeTableCounts(t, d, project)
	release := hivedb.AcquireProjectLifecycleRead(project)
	done := make(chan error, 1)
	go func() {
		_, err := d.ArchiveGovernanceProject(ctx, project, "actor", "archive", time.Now().UTC())
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("archive completed while lifecycle read lease was held: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("ArchiveGovernanceProject: %v", err)
	}

	after := purgeTableCounts(t, d, project)
	for table, count := range before {
		if table == "governance" {
			continue
		}
		if after[table] != count {
			t.Fatalf("archive changed %s rows: before=%d after=%d", table, count, after[table])
		}
	}
	var archived int
	if err := d.RawDB().QueryRow(`SELECT COUNT(*) FROM hive_project_governance WHERE project = ? AND archived_at IS NOT NULL`, project).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived != 1 {
		t.Fatalf("archive governance row count = %d, want 1", archived)
	}
	for table, query := range map[string]string{
		"archive mutation": `SELECT COUNT(*) FROM memory_mutations WHERE event_id = 'archive-event'`,
		"archive evidence": `SELECT COUNT(*) FROM memory_remote_presence WHERE entity_sync_id = 'archive-sync'`,
		"archive SDD head": `SELECT COUNT(*) FROM sdd_apply_heads WHERE project = 'archive-project'`,
		"archive receipt":  `SELECT COUNT(*) FROM sdd_apply_receipts WHERE request_id = 'archive-apply-receipt'`,
	} {
		assertPurgeCount(t, d, table, query, 1)
	}
}

func TestDeleteGovernanceProjectPurgesCompleteLocalStateAndPreservesUnrelatedRows(t *testing.T) {
	ctx := context.Background()
	d := openGovernanceTestDB(t)
	const project = "purge-project"
	const predecessor = "purge-directory-project"
	const keep = "keep-project"
	workspace := t.TempDir() + "/workspace"
	seedWorkspaceSourceState(t, d, predecessor)
	mustPurgeExec(t, d, `UPDATE memory_mutations SET synced_at = CURRENT_TIMESTAMP WHERE event_id = ?`, "workspace-mutation-"+predecessor)
	if _, inserted, err := d.EnsureWorkspaceProjectBinding(ctx, workspace, predecessor); err != nil || !inserted {
		t.Fatalf("seed predecessor workspace binding = (%t, %v), want inserted", inserted, err)
	}
	if changed, err := d.PromoteWorkspaceProject(ctx, workspace, predecessor, project); err != nil || !changed {
		t.Fatalf("PromoteWorkspaceProject = (%t, %v), want true nil", changed, err)
	}
	mustPurgeExec(t, d, `INSERT INTO memory_remote_presence (entity_sync_id, confirmed_at, source) VALUES (?, CURRENT_TIMESTAMP, 'mutation_accept')`, "workspace-memory-"+predecessor)
	mustPurgeExec(t, d, `INSERT INTO memory_mutation_dispatches (event_id, dispatched_at) VALUES (?, CURRENT_TIMESTAMP)`, "workspace-mutation-"+predecessor)
	targetMemoryID := saveGovernanceTestMemory(t, d, project, "purge target")
	keepMemoryID := saveGovernanceTestMemory(t, d, keep, "keep target")
	if err := d.RecordSyncFailure(project, time.Now().UTC(), 1, time.Now().UTC(), fmt.Errorf("purge sync state")); err != nil {
		t.Fatalf("seed sync state: %v", err)
	}

	mustPurgeExec(t, d, `INSERT INTO user_prompts (sync_id, project, content) VALUES ('purge-prompt', ?, 'prompt')`, project)
	mustPurgeExec(t, d, `INSERT INTO memory_prompt_links (memory_id, prompt_id) SELECT ?, id FROM user_prompts WHERE sync_id = 'purge-prompt'`, targetMemoryID)
	mustPurgeExec(t, d, `INSERT INTO memory_mutations (event_id, entity_sync_id, project, op) VALUES ('purge-event', 'purge-mutation-sync', ?, 'save')`, project)
	mustPurgeExec(t, d, `INSERT INTO mutation_receipts (request_id, operation, target_id, project, entity_sync_id, event_id, local_status, shared_status) VALUES ('purge-receipt', 'delete', ?, ?, 'purge-receipt-sync', 'purge-receipt-event', 'committed', 'pending')`, targetMemoryID, project)
	mustPurgeExec(t, d, `INSERT INTO mutation_cursors (consumer, project) VALUES ('consumer', ?)`, project)
	mustPurgeExec(t, d, `INSERT INTO pull_cursors (consumer, project, channel) VALUES ('consumer', ?, 'memories')`, project)
	mustPurgeExec(t, d, `INSERT INTO sync_attempt_logs (attempt_id, project, started_at, ended_at, outcome) VALUES ('purge-attempt', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'success')`, project)
	mustPurgeExec(t, d, `INSERT INTO passive_observations (project, content) VALUES (?, 'observation')`, project)
	mustPurgeExec(t, d, `INSERT INTO import_runs (id, source_system) VALUES ('purge-import-run', 'test')`)
	mustPurgeExec(t, d, `INSERT INTO import_source_aliases (source_system, source_table, source_id, source_project, hive_table, hive_pk, hive_sync_id, run_id) VALUES ('test', 'source', 'purge', ?, 'memories', '1', 'purge-import-sync', 'purge-import-run')`, project)
	mustPurgeExec(t, d, `INSERT INTO project_blocks (canonical_project_key, project, command_id, blocked_at) VALUES (?, ?, 'purge-block', CURRENT_TIMESTAMP)`, project, project)
	mustPurgeExec(t, d, `INSERT INTO project_quarantine_archives (canonical_project_key, project, command_id) VALUES (?, ?, 'purge-quarantine')`, project, project)
	mustPurgeExec(t, d, `INSERT INTO hive_warnings (severity, source, message) VALUES ('warn', ?, 'warning')`, project)
	mustPurgeExec(t, d, `INSERT INTO workspace_project_bindings (workspace, project) VALUES ('/work/purge', ?)`, project)
	mustPurgeExec(t, d, `INSERT INTO project_aliases (source_project, target_project, scope, reason) VALUES ('purge-source', ?, 'local', 'test'), (?, 'other-project', 'local', 'test'), ('keep-source', ?, 'local', 'test')`, project, project, keep)
	mustPurgeExec(t, d, `INSERT INTO sdd_store_bindings (project, change_name, schema_version, mode, provenance) VALUES (?, 'change', '1', 'hive', 'test')`, project)
	mustPurgeExec(t, d, `INSERT INTO sdd_apply_heads (project, change_name, snapshot_memory_id, generation, revision, digest) VALUES (?, 'change', ?, 1, 1, 'digest')`, project, targetMemoryID)
	mustPurgeExec(t, d, `INSERT INTO sdd_apply_receipts (request_id, project, change_name, payload_sha256, response_json) VALUES ('purge-apply-receipt', ?, 'change', 'digest', '{}')`, project)
	mustPurgeExec(t, d, `INSERT INTO recovery_tokens (token, reason, requested_project, selected_project, candidates_json, context_hash, created_at, expires_at) VALUES ('purge-requested', 'test', ?, '', '[]', 'hash', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, project)
	mustPurgeExec(t, d, `INSERT INTO recovery_tokens (token, reason, requested_project, selected_project, candidates_json, context_hash, created_at, expires_at) VALUES ('purge-candidate', 'test', 'other', ?, '[{"project":"purge-project"}]', 'hash', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, project)

	for _, syncID := range []string{"purge-mutation-sync", "purge-receipt-sync"} {
		mustPurgeExec(t, d, `INSERT INTO memory_remote_presence (entity_sync_id, confirmed_at, source) VALUES (?, CURRENT_TIMESTAMP, 'mutation_accept')`, syncID)
		mustPurgeExec(t, d, `INSERT INTO memory_local_origins (entity_sync_id, recorded_at, source) VALUES (?, CURRENT_TIMESTAMP, 'local_save')`, syncID)
		mustPurgeExec(t, d, `INSERT INTO memory_entity_dispatches (dispatch_id, entity_sync_id, dispatched_at) VALUES ('purge-dispatch', ?, CURRENT_TIMESTAMP)`, syncID)
	}
	for _, eventID := range []string{"purge-event", "purge-receipt-event"} {
		mustPurgeExec(t, d, `INSERT INTO memory_mutation_outcomes (event_id, entity_sync_id, outcome, terminal_at) VALUES (?, 'purge-mutation-sync', 'accepted', CURRENT_TIMESTAMP)`, eventID)
		mustPurgeExec(t, d, `INSERT INTO memory_mutation_dispatches (event_id, dispatched_at) VALUES (?, CURRENT_TIMESTAMP)`, eventID)
	}
	mustPurgeExec(t, d, `INSERT INTO memories (sync_id, project, topic_key, title, content, session_id) VALUES ('purge-sdd-document', ?, 'sdd/change/apply-evidence/batch', 'sdd evidence', '{"schema":"jarvis.sdd-apply-v2"}', 'manual-save-purge-project')`, project)

	// Matching rows for another project prove that purge scope is not global.
	mustPurgeExec(t, d, `INSERT INTO memory_mutations (event_id, entity_sync_id, project, op) VALUES ('keep-event', 'keep-sync', ?, 'save')`, keep)
	mustPurgeExec(t, d, `INSERT INTO memory_remote_presence (entity_sync_id, confirmed_at, source) VALUES ('keep-sync', CURRENT_TIMESTAMP, 'mutation_accept')`)
	mustPurgeExec(t, d, `INSERT INTO memory_mutation_dispatches (event_id, dispatched_at) VALUES ('keep-event', CURRENT_TIMESTAMP)`)

	archiveGovernanceProjectForTest(t, d, project)
	// The usual canonical count includes A's redirect/governance rows through B;
	// its acknowledged mutation and retained identity are additional predecessors.
	expectedDeleted := purgeDeleteRowsForProject(t, d, project) + 5 // A mutation, dispatch, identity, and two recovery tokens.
	deleted, err := d.DeleteGovernanceProject(ctx, project, "tester", "complete purge")
	if err != nil {
		t.Fatalf("DeleteGovernanceProject: %v", err)
	}
	if deleted == 0 {
		t.Fatal("DeleteGovernanceProject returned zero while project traces existed")
	}
	if deleted != expectedDeleted {
		t.Fatalf("predecessor purge rows deleted = %d, want exact DELETE count %d", deleted, expectedDeleted)
	}

	for table, query := range map[string]string{
		"memories":                 `SELECT COUNT(*) FROM memories WHERE project IN ('purge-project', 'purge-directory-project')`,
		"sessions":                 `SELECT COUNT(*) FROM sessions WHERE project IN ('purge-project', 'purge-directory-project')`,
		"prompts":                  `SELECT COUNT(*) FROM user_prompts WHERE project IN ('purge-project', 'purge-directory-project')`,
		"prompt links":             `SELECT COUNT(*) FROM memory_prompt_links WHERE memory_id = ` + fmt.Sprint(targetMemoryID),
		"mutations":                `SELECT COUNT(*) FROM memory_mutations WHERE project IN ('purge-project', 'purge-directory-project')`,
		"mutation receipts":        `SELECT COUNT(*) FROM mutation_receipts WHERE project IN ('purge-project', 'purge-directory-project')`,
		"cursors":                  `SELECT COUNT(*) FROM mutation_cursors WHERE project = 'purge-project' OR project IN (SELECT project FROM pull_cursors WHERE project = 'purge-project')`,
		"sync state":               `SELECT COUNT(*) FROM sync_state WHERE project = 'purge-project'`,
		"sync attempts":            `SELECT COUNT(*) FROM sync_attempt_logs WHERE project = 'purge-project'`,
		"passive observations":     `SELECT COUNT(*) FROM passive_observations WHERE project = 'purge-project'`,
		"import aliases":           `SELECT COUNT(*) FROM import_source_aliases WHERE source_project = 'purge-project'`,
		"blocks":                   `SELECT COUNT(*) FROM project_blocks WHERE canonical_project_key = 'purge-project' OR project = 'purge-project'`,
		"quarantine":               `SELECT COUNT(*) FROM project_quarantine_archives WHERE canonical_project_key = 'purge-project' OR project = 'purge-project'`,
		"warnings":                 `SELECT COUNT(*) FROM hive_warnings WHERE source = 'purge-project'`,
		"workspace bindings":       `SELECT COUNT(*) FROM workspace_project_bindings WHERE project IN ('purge-project', 'purge-directory-project')`,
		"aliases":                  `SELECT COUNT(*) FROM project_aliases WHERE source_project IN ('purge-project', 'purge-directory-project') OR target_project IN ('purge-project', 'purge-directory-project')`,
		"identity":                 `SELECT COUNT(*) FROM project_identities WHERE project_key IN ('purge-project', 'purge-directory-project')`,
		"governance":               `SELECT COUNT(*) FROM hive_project_governance WHERE project IN ('purge-project', 'purge-directory-project') OR merge_target IN ('purge-project', 'purge-directory-project')`,
		"SDD bindings":             `SELECT COUNT(*) FROM sdd_store_bindings WHERE project = 'purge-project'`,
		"SDD heads":                `SELECT COUNT(*) FROM sdd_apply_heads WHERE project = 'purge-project'`,
		"SDD receipts":             `SELECT COUNT(*) FROM sdd_apply_receipts WHERE project = 'purge-project'`,
		"recovery token requested": `SELECT COUNT(*) FROM recovery_tokens WHERE token = 'purge-requested'`,
		"recovery token candidate": `SELECT COUNT(*) FROM recovery_tokens WHERE token = 'purge-candidate'`,
		"remote presence":          `SELECT COUNT(*) FROM memory_remote_presence WHERE entity_sync_id LIKE 'purge-%' OR entity_sync_id = 'workspace-memory-purge-directory-project'`,
		"local origins":            `SELECT COUNT(*) FROM memory_local_origins WHERE entity_sync_id LIKE 'purge-%' OR entity_sync_id = 'workspace-memory-purge-directory-project'`,
		"entity dispatches":        `SELECT COUNT(*) FROM memory_entity_dispatches WHERE entity_sync_id LIKE 'purge-%' OR entity_sync_id = 'workspace-memory-purge-directory-project'`,
		"mutation outcomes":        `SELECT COUNT(*) FROM memory_mutation_outcomes WHERE event_id LIKE 'purge-%' OR entity_sync_id LIKE 'purge-%' OR event_id = 'workspace-mutation-purge-directory-project' OR entity_sync_id = 'workspace-memory-purge-directory-project'`,
		"mutation dispatches":      `SELECT COUNT(*) FROM memory_mutation_dispatches WHERE event_id LIKE 'purge-%' OR event_id = 'workspace-mutation-purge-directory-project'`,
	} {
		assertPurgeCount(t, d, table, query, 0)
	}
	assertPurgeCount(t, d, "unrelated memory", `SELECT COUNT(*) FROM memories WHERE id = `+fmt.Sprint(keepMemoryID), 1)
	assertPurgeCount(t, d, "unrelated alias", `SELECT COUNT(*) FROM project_aliases WHERE source_project = 'keep-source' AND target_project = 'keep-project'`, 1)
	assertPurgeCount(t, d, "unrelated evidence", `SELECT COUNT(*) FROM memory_remote_presence WHERE entity_sync_id = 'keep-sync'`, 1)
	second, err := d.DeleteGovernanceProject(ctx, project, "tester", "repeat purge")
	if err != nil || second != 0 {
		t.Fatalf("repeated purge = (%d, %v), want (0, nil)", second, err)
	}
	// A later directory observation and Git promotion must start from a blank
	// local identity, not recover the retired predecessor's acknowledged history.
	if _, err := d.SaveMemoryWithManualSession(&models.Memory{Project: predecessor, Title: "fresh directory state", Content: "fresh"}); err != nil {
		t.Fatalf("recreate directory project: %v", err)
	}
	bound, inserted, err := d.EnsureWorkspaceProjectBinding(ctx, workspace, predecessor)
	if err != nil || !inserted || bound != predecessor {
		t.Fatalf("fresh directory binding = (%q, %t, %v), want (%q, true, nil)", bound, inserted, err, predecessor)
	}
	if changed, err := d.PromoteWorkspaceProject(ctx, workspace, predecessor, project); err != nil || !changed {
		t.Fatalf("fresh Git promotion = (%t, %v), want true nil", changed, err)
	}
	assertPurgeCount(t, d, "retired predecessor mutation", `SELECT COUNT(*) FROM memory_mutations WHERE event_id = 'workspace-mutation-purge-directory-project'`, 0)
	assertPurgeCount(t, d, "retired predecessor evidence", `SELECT COUNT(*) FROM memory_remote_presence WHERE entity_sync_id = 'workspace-memory-purge-directory-project'`, 0)
	mustPurgeExec(t, d, `INSERT INTO workspace_project_bindings (workspace, project) VALUES ('/work/leftover', 'orphan-project')`)
	if rows, err := d.DeleteGovernanceProject(ctx, "orphan-project", "tester", "trace check"); !errors.Is(err, hivedb.ErrGovernanceProjectNotArchived) || rows != 0 {
		t.Fatalf("purge with remaining local trace = (%d, %v), want archived guard", rows, err)
	}

	mustPurgeExec(t, d, `INSERT INTO sdd_apply_receipts (request_id, project, change_name, payload_sha256, response_json) VALUES ('keep-apply-receipt', 'keep-project', 'change', 'digest', '{}')`)
	if _, err := d.RawDB().Exec(`DELETE FROM sdd_apply_receipts WHERE request_id = 'keep-apply-receipt'`); err == nil {
		t.Fatal("receipt delete protection trigger was not restored after purge")
	}
}

func TestDeleteGovernanceProjectRollsBackOnFailureAndAllowsFreshRecreation(t *testing.T) {
	ctx := context.Background()
	d := openGovernanceTestDB(t)
	const project = "current-project"
	const predecessor = "old-project"
	saveGovernanceTestMemory(t, d, "Old.Project", "old state")
	workspace := t.TempDir() + "/workspace"
	mustPurgeExec(t, d, `UPDATE memory_mutations SET synced_at = CURRENT_TIMESTAMP WHERE project = ?`, predecessor)
	if _, inserted, err := d.EnsureWorkspaceProjectBinding(ctx, workspace, "Old.Project"); err != nil || !inserted {
		t.Fatalf("seed workspace binding = (%t, %v), want inserted binding", inserted, err)
	}
	if changed, err := d.PromoteWorkspaceProject(ctx, workspace, predecessor, project); err != nil || !changed {
		t.Fatalf("promote rollback predecessor = (%t, %v), want true nil", changed, err)
	}
	mustPurgeExec(t, d, `INSERT INTO sdd_apply_receipts (request_id, project, change_name, payload_sha256, response_json) VALUES ('rollback-apply-receipt', ?, 'change', 'digest', '{}')`, project)
	archiveGovernanceProjectForTest(t, d, project)
	// memories are deleted after the receipt-trigger exception, so this forces a
	// rollback after the temporary trigger removal and recreation has completed.
	mustPurgeExec(t, d, `CREATE TRIGGER fail_purge_memory BEFORE DELETE ON memories WHEN OLD.project = 'current-project' BEGIN SELECT RAISE(ABORT, 'forced purge failure'); END`)
	if _, err := d.DeleteGovernanceProject(ctx, project, "tester", "forced failure"); err == nil {
		t.Fatal("purge succeeded despite forced failure")
	}
	assertPurgeCount(t, d, "rollback current memory", `SELECT COUNT(*) FROM memories WHERE project = 'current-project'`, 1)
	assertPurgeCount(t, d, "rollback predecessor mutation", `SELECT COUNT(*) FROM memory_mutations WHERE project = 'old-project'`, 1)
	assertPurgeCount(t, d, "rollback receipt", `SELECT COUNT(*) FROM sdd_apply_receipts WHERE request_id = 'rollback-apply-receipt'`, 1)
	assertPurgeCount(t, d, "rollback predecessor governance", `SELECT COUNT(*) FROM hive_project_governance WHERE project = 'old-project' AND merge_target = 'current-project'`, 1)
	assertPurgeCount(t, d, "rollback predecessor identity", `SELECT COUNT(*) FROM project_identities WHERE project_key = 'old-project'`, 1)
	if _, err := d.RawDB().Exec(`DELETE FROM sdd_apply_receipts WHERE request_id = 'rollback-apply-receipt'`); err == nil {
		t.Fatal("receipt delete protection trigger was not restored after rollback")
	}
	mustPurgeExec(t, d, `DROP TRIGGER fail_purge_memory`)

	release := hivedb.AcquireProjectLifecycleRead(project)
	done := make(chan error, 1)
	go func() {
		_, err := d.DeleteGovernanceProject(ctx, project, "tester", "serialized purge")
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("purge completed while lifecycle read lease was held: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("DeleteGovernanceProject: %v", err)
	}
	if _, err := d.SaveMemoryWithManualSession(&models.Memory{Project: "Old Project", Title: "fresh state", Content: "fresh"}); err != nil {
		t.Fatalf("recreate project: %v", err)
	}
	bound, inserted, err := d.EnsureWorkspaceProjectBinding(ctx, workspace, "Old Project")
	if err != nil || !inserted || bound != "old-project" {
		t.Fatalf("recreated workspace binding = (%q, %t, %v), want fresh old-project binding", bound, inserted, err)
	}
	var spelling string
	if err := d.RawDB().QueryRow(`SELECT first_spelling FROM project_identities WHERE project_key = 'old-project'`).Scan(&spelling); err != nil {
		t.Fatal(err)
	}
	if spelling != "Old Project" {
		t.Fatalf("recreated identity spelling = %q, want fresh spelling", spelling)
	}
	rows, err := d.DeleteGovernanceProject(ctx, predecessor, "tester", "repeat")
	if err == nil || rows != 0 {
		t.Fatalf("live recreated project purge = (%d, %v), want zero rows and archived guard", rows, err)
	}
}

func TestDeleteGovernanceProjectPurgesTransitiveRetiredPredecessorsWithoutFollowingOutboundTargets(t *testing.T) {
	ctx := context.Background()
	d := openGovernanceTestDB(t)
	const first = "chain-directory"
	const second = "chain-git"
	const canonical = "chain-current"
	const unrelated = "chain-unrelated"
	workspace := t.TempDir() + "/workspace"

	seedWorkspaceSourceState(t, d, first)
	mustPurgeExec(t, d, `UPDATE memory_mutations SET synced_at = CURRENT_TIMESTAMP WHERE event_id = ?`, "workspace-mutation-"+first)
	if _, inserted, err := d.EnsureWorkspaceProjectBinding(ctx, workspace, first); err != nil || !inserted {
		t.Fatalf("seed first binding = (%t, %v), want inserted", inserted, err)
	}
	if changed, err := d.PromoteWorkspaceProject(ctx, workspace, first, second); err != nil || !changed {
		t.Fatalf("promote first to second = (%t, %v), want true nil", changed, err)
	}
	// Chained aliases are intentionally rejected by the promotion path. Model
	// the legacy B→C governance redirect that still must be followed by purge.
	saveGovernanceTestMemory(t, d, canonical, "canonical state")
	mustPurgeExec(t, d, `INSERT INTO hive_project_governance (project, merge_target, merged_at, merged_by, merge_reason) VALUES (?, ?, CURRENT_TIMESTAMP, 'tester', 'legacy chain')`, second, canonical)
	mustPurgeExec(t, d, `INSERT INTO project_aliases (source_project, target_project, scope, reason) VALUES (?, ?, 'local', 'legacy chain')`, second, canonical)
	// An outbound malformed alias is itself deleted, but its unrelated target is
	// never added to the retired predecessor closure.
	mustPurgeExec(t, d, `INSERT INTO project_aliases (source_project, target_project, scope, reason) VALUES (?, ?, 'local', 'test')`, canonical, unrelated)
	saveGovernanceTestMemory(t, d, unrelated, "unrelated state")
	archiveGovernanceProjectForTest(t, d, canonical)

	deleted, err := d.DeleteGovernanceProject(ctx, canonical, "tester", "transitive purge")
	if err != nil || deleted == 0 {
		t.Fatalf("DeleteGovernanceProject = (%d, %v), want positive nil", deleted, err)
	}
	for table, query := range map[string]string{
		"transitive mutations":  `SELECT COUNT(*) FROM memory_mutations WHERE project IN ('chain-directory', 'chain-git', 'chain-current')`,
		"transitive identities": `SELECT COUNT(*) FROM project_identities WHERE project_key IN ('chain-directory', 'chain-git', 'chain-current')`,
		"transitive aliases":    `SELECT COUNT(*) FROM project_aliases WHERE source_project IN ('chain-directory', 'chain-git', 'chain-current') OR target_project IN ('chain-directory', 'chain-git', 'chain-current')`,
		"transitive governance": `SELECT COUNT(*) FROM hive_project_governance WHERE project IN ('chain-directory', 'chain-git', 'chain-current') OR merge_target IN ('chain-directory', 'chain-git', 'chain-current')`,
		"transitive bindings":   `SELECT COUNT(*) FROM workspace_project_bindings WHERE project IN ('chain-directory', 'chain-git', 'chain-current')`,
	} {
		assertPurgeCount(t, d, table, query, 0)
	}
	assertPurgeCount(t, d, "unrelated outbound target", `SELECT COUNT(*) FROM memories WHERE project = 'chain-unrelated'`, 1)
	if repeated, err := d.DeleteGovernanceProject(ctx, canonical, "tester", "transitive repeat"); err != nil || repeated != 0 {
		t.Fatalf("repeated transitive purge = (%d, %v), want (0, nil)", repeated, err)
	}
}

func TestDeleteGovernanceProjectBatchesIndirectEvidence(t *testing.T) {
	ctx := context.Background()
	d := openGovernanceTestDB(t)
	const project = "batch-project"
	const keep = "batch-keep"
	saveGovernanceTestMemory(t, d, project, "batch target")
	saveGovernanceTestMemory(t, d, keep, "batch keeper")

	// 1,001 coordinates require three deterministic 500-value batches. The
	// target rows exercise both sync-ID and mutation-event evidence tables.
	mustPurgeExec(t, d, `WITH RECURSIVE n(value) AS (VALUES(0) UNION ALL SELECT value + 1 FROM n WHERE value < 1000)
		INSERT INTO memory_mutations (event_id, entity_sync_id, project, op)
		SELECT printf('batch-event-%04d', value), printf('batch-sync-%04d', value), ?, 'save' FROM n`, project)
	mustPurgeExec(t, d, `INSERT INTO memory_remote_presence (entity_sync_id, confirmed_at, source)
		SELECT entity_sync_id, CURRENT_TIMESTAMP, 'mutation_accept' FROM memory_mutations WHERE project = ?`, project)
	mustPurgeExec(t, d, `INSERT INTO memory_mutation_outcomes (event_id, entity_sync_id, outcome, terminal_at)
		SELECT event_id, entity_sync_id, 'accepted', CURRENT_TIMESTAMP FROM memory_mutations WHERE project = ?`, project)
	mustPurgeExec(t, d, `INSERT INTO memory_mutation_dispatches (event_id, dispatched_at)
		SELECT event_id, CURRENT_TIMESTAMP FROM memory_mutations WHERE project = ?`, project)
	mustPurgeExec(t, d, `INSERT INTO memory_entity_dispatches (dispatch_id, entity_sync_id, dispatched_at)
		SELECT 'batch-dispatch', entity_sync_id, CURRENT_TIMESTAMP FROM memory_mutations WHERE project = ?`, project)
	mustPurgeExec(t, d, `INSERT INTO memory_mutations (event_id, entity_sync_id, project, op) VALUES ('batch-keep-event', 'batch-keep-sync', ?, 'save')`, keep)
	mustPurgeExec(t, d, `INSERT INTO memory_remote_presence (entity_sync_id, confirmed_at, source) VALUES ('batch-keep-sync', CURRENT_TIMESTAMP, 'mutation_accept')`)
	mustPurgeExec(t, d, `INSERT INTO memory_mutation_outcomes (event_id, entity_sync_id, outcome, terminal_at) VALUES ('batch-keep-event', 'batch-keep-sync', 'accepted', CURRENT_TIMESTAMP)`)
	mustPurgeExec(t, d, `INSERT INTO memory_mutation_dispatches (event_id, dispatched_at) VALUES ('batch-keep-event', CURRENT_TIMESTAMP)`)
	mustPurgeExec(t, d, `INSERT INTO memory_entity_dispatches (dispatch_id, entity_sync_id, dispatched_at) VALUES ('batch-keep-dispatch', 'batch-keep-sync', CURRENT_TIMESTAMP)`)

	archiveGovernanceProjectForTest(t, d, project)
	if _, err := d.DeleteGovernanceProject(ctx, project, "tester", "batch evidence purge"); err != nil {
		t.Fatalf("DeleteGovernanceProject: %v", err)
	}
	for table, query := range map[string]string{
		"remote presence":     `SELECT COUNT(*) FROM memory_remote_presence WHERE entity_sync_id LIKE 'batch-sync-%'`,
		"mutation outcomes":   `SELECT COUNT(*) FROM memory_mutation_outcomes WHERE event_id LIKE 'batch-event-%'`,
		"mutation dispatches": `SELECT COUNT(*) FROM memory_mutation_dispatches WHERE event_id LIKE 'batch-event-%'`,
		"entity dispatches":   `SELECT COUNT(*) FROM memory_entity_dispatches WHERE entity_sync_id LIKE 'batch-sync-%'`,
	} {
		assertPurgeCount(t, d, table, query, 0)
	}
	for table, query := range map[string]string{
		"remote presence":     `SELECT COUNT(*) FROM memory_remote_presence WHERE entity_sync_id = 'batch-keep-sync'`,
		"mutation outcomes":   `SELECT COUNT(*) FROM memory_mutation_outcomes WHERE event_id = 'batch-keep-event'`,
		"mutation dispatches": `SELECT COUNT(*) FROM memory_mutation_dispatches WHERE event_id = 'batch-keep-event'`,
		"entity dispatches":   `SELECT COUNT(*) FROM memory_entity_dispatches WHERE entity_sync_id = 'batch-keep-sync'`,
	} {
		assertPurgeCount(t, d, "unrelated "+table, query, 1)
	}
}

func TestDeleteGovernanceProjectRowsDeletedExcludesRelocationProvenanceUpdates(t *testing.T) {
	ctx := context.Background()
	d := openGovernanceTestDB(t)
	const project = "count-project"
	const keep = "count-keep"
	saveGovernanceTestMemory(t, d, project, "count target")
	if err := d.CreateSession("count-keep-session", keep, "/work/keep", "dev", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SavePrompt(ctx, keep, "keep prompt"); err != nil {
		t.Fatal(err)
	}
	mustPurgeExec(t, d, `UPDATE sessions SET sync_from_project = ? WHERE id = 'count-keep-session'`, project)
	mustPurgeExec(t, d, `UPDATE user_prompts SET sync_from_project = ? WHERE project = ?`, project, keep)
	archiveGovernanceProjectForTest(t, d, project)

	expected := purgeDeleteRowsForProject(t, d, project)
	deleted, err := d.DeleteGovernanceProject(ctx, project, "tester", "count purge")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != expected {
		t.Fatalf("rows deleted = %d, want exact DELETE count %d", deleted, expected)
	}
	assertPurgeCount(t, d, "surviving reverse provenance session", `SELECT COUNT(*) FROM sessions WHERE id = 'count-keep-session' AND sync_from_project = ''`, 1)
	assertPurgeCount(t, d, "surviving reverse provenance prompt", `SELECT COUNT(*) FROM user_prompts WHERE project = 'count-keep' AND sync_from_project = ''`, 1)
}

func TestDeleteGovernanceProjectPurgesOnlyRecoveryTokensWithExactCoordinates(t *testing.T) {
	ctx := context.Background()
	d := openGovernanceTestDB(t)
	const project = "token-project"
	saveGovernanceTestMemory(t, d, project, "token target")
	for _, token := range []struct {
		name       string
		requested  string
		selected   string
		candidates string
	}{
		{"selected-only", "other", project, "[]"},
		{"candidate-only", "other", "", `[{"project":"token-project"}]`},
		{"unrelated-candidate", "other", "", `[{"project":"other-project"}]`},
		{"substring-only", "other", "", `[{"project":"token-project-suffix"}]`},
	} {
		mustPurgeExec(t, d, `INSERT INTO recovery_tokens (token, reason, requested_project, selected_project, candidates_json, context_hash, created_at, expires_at) VALUES (?, 'test', ?, ?, ?, 'hash', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, token.name, token.requested, token.selected, token.candidates)
	}
	archiveGovernanceProjectForTest(t, d, project)
	if _, err := d.DeleteGovernanceProject(ctx, project, "tester", "token purge"); err != nil {
		t.Fatal(err)
	}
	assertPurgeCount(t, d, "selected-only token", `SELECT COUNT(*) FROM recovery_tokens WHERE token = 'selected-only'`, 0)
	assertPurgeCount(t, d, "candidate-only token", `SELECT COUNT(*) FROM recovery_tokens WHERE token = 'candidate-only'`, 0)
	assertPurgeCount(t, d, "unrelated-candidate token", `SELECT COUNT(*) FROM recovery_tokens WHERE token = 'unrelated-candidate'`, 1)
	assertPurgeCount(t, d, "substring-only token", `SELECT COUNT(*) FROM recovery_tokens WHERE token = 'substring-only'`, 1)

	// An already-purged canonical key must not report idempotent success once a
	// directly attributable local trace appears again.
	mustPurgeExec(t, d, `INSERT INTO mutation_cursors (consumer, project) VALUES ('trace-consumer', 'token-project')`)
	if rows, err := d.DeleteGovernanceProject(ctx, project, "tester", "direct trace"); !errors.Is(err, hivedb.ErrGovernanceProjectNotArchived) || rows != 0 {
		t.Fatalf("already-purged project with direct trace = (%d, %v), want archived guard", rows, err)
	}
}

func purgeDeleteRowsForProject(t *testing.T, d *hivedb.DB, project string) int {
	t.Helper()
	queries := []struct {
		query string
		args  []any
	}{
		{`SELECT COUNT(*) FROM memory_entity_dispatches WHERE entity_sync_id IN (SELECT sync_id FROM memories WHERE project = ? UNION SELECT entity_sync_id FROM memory_mutations WHERE project = ? UNION SELECT entity_sync_id FROM mutation_receipts WHERE project = ?)`, []any{project, project, project}},
		{`SELECT COUNT(*) FROM memory_remote_presence WHERE entity_sync_id IN (SELECT sync_id FROM memories WHERE project = ? UNION SELECT entity_sync_id FROM memory_mutations WHERE project = ? UNION SELECT entity_sync_id FROM mutation_receipts WHERE project = ?)`, []any{project, project, project}},
		{`SELECT COUNT(*) FROM memory_local_origins WHERE entity_sync_id IN (SELECT sync_id FROM memories WHERE project = ? UNION SELECT entity_sync_id FROM memory_mutations WHERE project = ? UNION SELECT entity_sync_id FROM mutation_receipts WHERE project = ?)`, []any{project, project, project}},
		{`SELECT COUNT(*) FROM memory_mutation_outcomes WHERE event_id IN (SELECT event_id FROM memory_mutations WHERE project = ? UNION SELECT event_id FROM mutation_receipts WHERE project = ?) OR entity_sync_id IN (SELECT sync_id FROM memories WHERE project = ? UNION SELECT entity_sync_id FROM memory_mutations WHERE project = ? UNION SELECT entity_sync_id FROM mutation_receipts WHERE project = ?)`, []any{project, project, project, project, project}},
		{`SELECT COUNT(*) FROM memory_mutation_dispatches WHERE event_id IN (SELECT event_id FROM memory_mutations WHERE project = ? UNION SELECT event_id FROM mutation_receipts WHERE project = ?)`, []any{project, project}},
		{`SELECT COUNT(*) FROM memory_prompt_links WHERE memory_id IN (SELECT id FROM memories WHERE project = ?) OR prompt_id IN (SELECT id FROM user_prompts WHERE project = ?)`, []any{project, project}},
		{`SELECT COUNT(*) FROM memory_mutations WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM mutation_receipts WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM mutation_cursors WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM pull_cursors WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM sync_state WHERE project = ? AND project != '__auth__'`, []any{project}},
		{`SELECT COUNT(*) FROM sync_attempt_logs WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM passive_observations WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM import_source_aliases WHERE source_project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM project_blocks WHERE canonical_project_key = ? OR project = ?`, []any{project, project}},
		{`SELECT COUNT(*) FROM project_quarantine_archives WHERE canonical_project_key = ? OR project = ?`, []any{project, project}},
		{`SELECT COUNT(*) FROM hive_warnings WHERE source = ?`, []any{project}},
		{`SELECT COUNT(*) FROM workspace_project_bindings WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM project_aliases WHERE source_project = ? OR target_project = ?`, []any{project, project}},
		{`SELECT COUNT(*) FROM sdd_store_bindings WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM sdd_apply_heads WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM sdd_apply_receipts WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM memories WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM user_prompts WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM sessions WHERE project = ?`, []any{project}},
		{`SELECT COUNT(*) FROM hive_project_governance WHERE project = ? OR merge_target = ?`, []any{project, project}},
		{`SELECT COUNT(*) FROM project_identities WHERE project_key = ?`, []any{project}},
	}
	var total int
	for _, check := range queries {
		var count int
		if err := d.RawDB().QueryRow(check.query, check.args...).Scan(&count); err != nil {
			t.Fatal(err)
		}
		total += count
	}
	return total
}

func mustPurgeExec(t *testing.T, d *hivedb.DB, query string, args ...any) {
	t.Helper()
	if _, err := d.RawDB().Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func assertPurgeCount(t *testing.T, d *hivedb.DB, label, query string, want int) {
	t.Helper()
	var got int
	if err := d.RawDB().QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", label, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", label, got, want)
	}
}

func purgeTableCounts(t *testing.T, d *hivedb.DB, project string) map[string]int {
	t.Helper()
	queries := map[string]string{
		"identity":   `SELECT COUNT(*) FROM project_identities WHERE project_key = ?`,
		"bindings":   `SELECT COUNT(*) FROM workspace_project_bindings WHERE project = ?`,
		"aliases":    `SELECT COUNT(*) FROM project_aliases WHERE source_project = ? OR target_project = ?`,
		"sdd":        `SELECT COUNT(*) FROM sdd_store_bindings WHERE project = ?`,
		"blocks":     `SELECT COUNT(*) FROM project_blocks WHERE canonical_project_key = ?`,
		"memories":   `SELECT COUNT(*) FROM memories WHERE project = ?`,
		"governance": `SELECT COUNT(*) FROM hive_project_governance WHERE project = ?`,
	}
	counts := make(map[string]int, len(queries))
	for table, query := range queries {
		args := []any{project}
		if table == "aliases" {
			args = []any{project, project}
		}
		var count int
		if err := d.RawDB().QueryRow(query, args...).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = count
	}
	return counts
}
