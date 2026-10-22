package mcp_test

import (
	"context"
	"strings"
	"testing"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
)

func TestSddApplyProgressMCPRetiredAliasUsesTargetAndReportsProjectKey(t *testing.T) {
	store, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.SaveMemoryWithManualSession(&models.Memory{Project: "target", Title: "target", Content: "registered"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddAlias(context.Background(), "retired", "target", "local", "test"); err != nil {
		t.Fatal(err)
	}
	session := connectTestServer(t, store)

	body := decodeJSONResponse(t, callTool(t, session, "sdd_apply_progress_get", map[string]any{"project": "Retired", "change": "change"}))
	if body["code"] != "not_found" || body["project_key"] != "target" {
		t.Fatalf("missing head response = %#v", body)
	}

	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "target", Change: "change", BatchID: "apb-a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4", Entries: []applyprogress.EvidenceEntry{{EntryID: "entry", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "target", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: strings.Repeat("a", 64), Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	advance := map[string]any{"project": "retired", "change": "change", "request_id": "mcp-alias", "snapshot": snapshot, "batches": []applyprogress.Batch{batch}}
	body = decodeJSONResponse(t, callTool(t, session, "sdd_apply_progress_advance", advance))
	if body["outcome"] != "committed" || body["project_key"] != "target" {
		t.Fatalf("advance response = %#v", body)
	}
	advance["request_id"] = "mcp-alias-stale"
	body = decodeJSONResponse(t, callTool(t, session, "sdd_apply_progress_advance", advance))
	if body["outcome"] != "conflict" || body["project_key"] != "target" {
		t.Fatalf("stale response = %#v", body)
	}

	body = decodeJSONResponse(t, callTool(t, session, "sdd_apply_progress_get", map[string]any{"project": "retired", "change": "change"}))
	state, ok := body["state"].(map[string]any)
	if body["outcome"] != "committed" || body["project_key"] != "target" || !ok || state["digest"] != snapshot.Digest {
		t.Fatalf("read response = %#v", body)
	}
	body = decodeJSONResponse(t, callTool(t, session, "sdd_apply_evidence_get", map[string]any{"project": "retired", "change": "change", "batch_id": batch.BatchID, "expected_head_digest": snapshot.Digest}))
	if body["outcome"] != "committed" || body["project_key"] != "target" {
		t.Fatalf("evidence response = %#v", body)
	}

	var retiredRows int
	if err := store.RawDB().QueryRow(`SELECT (SELECT COUNT(*) FROM memories WHERE project = 'retired') + (SELECT COUNT(*) FROM sdd_apply_heads WHERE project = 'retired') + (SELECT COUNT(*) FROM sdd_apply_receipts WHERE project = 'retired')`).Scan(&retiredRows); err != nil {
		t.Fatal(err)
	}
	if retiredRows != 0 {
		t.Fatalf("retired rows = %d, want 0", retiredRows)
	}
}

func TestSddApplyProgressMCPWithoutAliasReportsCanonicalKey(t *testing.T) {
	store, err := hivedb.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.SaveMemoryWithManualSession(&models.Memory{Project: "MyProject", Title: "project", Content: "registered"}); err != nil {
		t.Fatal(err)
	}
	session := connectTestServer(t, store)
	for _, spelling := range []string{"MyProject", " MYPROJECT ", "myproject"} {
		body := decodeJSONResponse(t, callTool(t, session, "sdd_apply_progress_get", map[string]any{"project": spelling, "change": "change"}))
		if body["code"] != "not_found" || body["project_key"] != "myproject" {
			t.Fatalf("%q response = %#v", spelling, body)
		}
	}
	body := decodeJSONResponse(t, callTool(t, session, "sdd_apply_progress_get", map[string]any{"project": "missing", "change": "change"}))
	if body["code"] != "project_not_found" || body["project_key"] != "missing" {
		t.Fatalf("unknown project response = %#v", body)
	}
	body = decodeJSONResponse(t, callTool(t, session, "sdd_apply_progress_get", map[string]any{"project": true, "change": "change"}))
	if _, present := body["project_key"]; present || body["code"] != "validation" {
		t.Fatalf("unresolved request response = %#v", body)
	}
}
