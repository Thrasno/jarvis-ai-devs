package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func identityHTTPAdvanceBody(t *testing.T, requestProject, documentProject, requestID string) ([]byte, applyprogress.Batch) {
	t.Helper()
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: documentProject, Change: "change", BatchID: "apb-a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3", Entries: []applyprogress.EvidenceEntry{{EntryID: "entry", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	require.NoError(t, err)
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: documentProject, Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: strings.Repeat("a", 64), Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	require.NoError(t, err)
	body, err := json.Marshal(map[string]any{"project": requestProject, "request_id": requestID, "snapshot": snapshot, "batches": []applyprogress.Batch{batch}})
	require.NoError(t, err)
	return body, batch
}

func serveIdentityJSON(t *testing.T, server *httpapi.Server, method, path string, body []byte, wantStatus int) map[string]any {
	t.Helper()
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(method, path, bytes.NewReader(body)))
	require.Equal(t, wantStatus, response.Code, "%s %s: %s", method, path, response.Body.String())
	var envelope map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	return envelope
}

func TestApplyProgressHTTPRetiredAliasUsesTargetAndReportsProjectKey(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "target", "sdd/change/explore", "explore")
	require.NoError(t, store.AddAlias(context.Background(), "retired", "target", "local", "test"))

	missing := serveIdentityJSON(t, server, http.MethodGet, "/sdd/changes/change/apply-progress?project=Retired", nil, http.StatusNotFound)
	assert.Equal(t, "not_found", missing["code"])
	assert.Equal(t, "target", missing["project_key"])

	body, batch := identityHTTPAdvanceBody(t, "retired", "target", "http-alias")
	advanced := serveIdentityJSON(t, server, http.MethodPost, "/sdd/changes/change/apply-progress/advance", body, http.StatusOK)
	assert.Equal(t, "committed", advanced["outcome"])
	assert.Equal(t, "target", advanced["project_key"])
	digest := advanced["state"].(map[string]any)["digest"].(string)

	stale := serveIdentityJSON(t, server, http.MethodPost, "/sdd/changes/change/apply-progress/advance", bytes.Replace(body, []byte("http-alias"), []byte("http-alias-stale"), 1), http.StatusConflict)
	assert.Equal(t, "stale", stale["code"])
	assert.Equal(t, "target", stale["project_key"])

	read := serveIdentityJSON(t, server, http.MethodGet, "/sdd/changes/change/apply-progress?project=retired", nil, http.StatusOK)
	assert.Equal(t, "target", read["project_key"])
	assert.Equal(t, digest, read["state"].(map[string]any)["digest"])

	evidence := serveIdentityJSON(t, server, http.MethodGet, "/sdd/changes/change/apply-evidence/"+batch.BatchID+"?project=retired&expected_head_digest="+digest, nil, http.StatusOK)
	assert.Equal(t, "target", evidence["project_key"])

	receipt := serveIdentityJSON(t, server, http.MethodGet, "/sdd/changes/change/apply-progress/receipts/http-alias?project=retired", nil, http.StatusOK)
	assert.Equal(t, "target", receipt["project_key"])
	assert.Equal(t, "http-alias", receipt["receipt"].(map[string]any)["request_id"])

	occupancy := serveIdentityJSON(t, server, http.MethodGet, "/sdd/changes/change/successor-occupancy?project=retired", nil, http.StatusOK)
	assert.Equal(t, "target", occupancy["project_key"])
	assert.Equal(t, true, occupancy["occupied"])

	publish := serveIdentityJSON(t, server, http.MethodPost, "/sdd/changes/change/apply-progress/publish-successor", []byte(`{"project":"retired"}`), http.StatusUnprocessableEntity)
	assert.Equal(t, "validation", publish["code"])
	assert.Equal(t, "target", publish["project_key"])

	var retiredRows int
	require.NoError(t, store.RawDB().QueryRow(`SELECT (SELECT COUNT(*) FROM memories WHERE project = 'retired') + (SELECT COUNT(*) FROM sdd_apply_heads WHERE project = 'retired') + (SELECT COUNT(*) FROM sdd_apply_receipts WHERE project = 'retired')`).Scan(&retiredRows))
	assert.Zero(t, retiredRows)
}

func TestApplyProgressHTTPSpellingsReportOneProjectKey(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "MyProject", "sdd/change/explore", "explore")
	body, _ := identityHTTPAdvanceBody(t, "MyProject", "myproject", "http-fold")
	serveIdentityJSON(t, server, http.MethodPost, "/sdd/changes/change/apply-progress/advance", body, http.StatusOK)
	for _, spelling := range []string{"MyProject", "%20MYPROJECT%20", "myproject"} {
		read := serveIdentityJSON(t, server, http.MethodGet, "/sdd/changes/change/apply-progress?project="+spelling, nil, http.StatusOK)
		assert.Equal(t, "myproject", read["project_key"], spelling)
	}
}

func TestApplyProgressHTTPProjectKeyBoundaries(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "retired", "notes", "plain")
	require.NoError(t, store.AddAlias(context.Background(), "retired", "unknown-target", "local", "test"))

	gated := serveIdentityJSON(t, server, http.MethodGet, "/sdd/changes/change/apply-progress?project=retired", nil, http.StatusNotFound)
	assert.Equal(t, "project_not_found", gated["code"])
	assert.Equal(t, "unknown-target", gated["project_key"])

	unresolved := serveIdentityJSON(t, server, http.MethodGet, "/sdd/changes/change/apply-progress", nil, http.StatusUnprocessableEntity)
	assert.Equal(t, "validation", unresolved["code"])
	assert.NotContains(t, unresolved, "project_key", "a request that never resolved a project reports no key")
}

func TestSDDStoreBindingHTTPRetiredAliasBindsTarget(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	require.NoError(t, store.AddAlias(context.Background(), "retired", "target", "local", "test"))
	path := "/sdd/changes/change/store-binding/adopt"
	body := []byte(`{"project":"Retired","mode":"hive","provenance":"governance"}`)

	missing := serveIdentityJSON(t, server, http.MethodGet, "/sdd/changes/change/store-binding?project=retired", nil, http.StatusNotFound)
	assert.Equal(t, map[string]any{"code": "not_found"}, missing)

	created := serveIdentityJSON(t, server, http.MethodPost, path, body, http.StatusCreated)
	assert.NotContains(t, created, "project_key")
	assert.Equal(t, "target", created["binding"].(map[string]any)["project"])
	assert.Equal(t, true, created["created"])

	replayed := serveIdentityJSON(t, server, http.MethodPost, path, body, http.StatusOK)
	assert.Equal(t, false, replayed["created"])
	assert.Equal(t, created["binding"], replayed["binding"])

	conflict := serveIdentityJSON(t, server, http.MethodPost, path, []byte(`{"project":"retired","mode":"hybrid","provenance":"other"}`), http.StatusConflict)
	assert.Equal(t, "binding_conflict", conflict["code"])
	assert.NotContains(t, conflict, "project_key")
	assert.Equal(t, created["binding"], conflict["existing"])
	assert.Equal(t, "target", conflict["requested"].(map[string]any)["project"])

	read := serveIdentityJSON(t, server, http.MethodGet, "/sdd/changes/change/store-binding?project=RETIRED", nil, http.StatusOK)
	assert.NotContains(t, read, "project_key")
	assert.Equal(t, created["binding"], read["binding"], "a conflict must leave the stored binding unchanged")

	var retiredRows int
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM sdd_store_bindings WHERE project = 'retired'`).Scan(&retiredRows))
	assert.Zero(t, retiredRows)
}

func TestSDDStoreBindingHTTPBlockedProjectRefusesOnlyNewAdoption(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	path := "/sdd/changes/existing/store-binding/adopt"
	body := []byte(`{"project":"project","mode":"hive","provenance":"governance"}`)
	created := serveIdentityJSON(t, server, http.MethodPost, path, body, http.StatusCreated)
	assert.Equal(t, "project", created["binding"].(map[string]any)["project"])
	_, err := store.RawDB().Exec(`INSERT INTO project_blocks (canonical_project_key, project, command_id, blocked_at) VALUES ('project', 'project', 'block-http-binding', CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	replayed := serveIdentityJSON(t, server, http.MethodPost, path, body, http.StatusOK)
	assert.Equal(t, false, replayed["created"])

	blocked := serveIdentityJSON(t, server, http.MethodPost, "/sdd/changes/fresh/store-binding/adopt", body, http.StatusLocked)
	assert.Equal(t, map[string]any{"code": "project_blocked"}, blocked)
	_, found, err := store.GetSDDStoreBinding(context.Background(), "project", "fresh")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestSDDStoreBindingHTTPWithoutAliasProjectsCanonicalKey(t *testing.T) {
	_, server := newSDDHTTPServer(t)
	created := serveIdentityJSON(t, server, http.MethodPost, "/sdd/changes/change/store-binding/adopt", []byte(`{"project":" Project ","mode":"hybrid","provenance":"source"}`), http.StatusCreated)
	assert.NotContains(t, created, "project_key")
	assert.Equal(t, "project", created["binding"].(map[string]any)["project"])
	read := serveIdentityJSON(t, server, http.MethodGet, "/sdd/changes/change/store-binding?project=PROJECT", nil, http.StatusOK)
	assert.NotContains(t, read, "project_key")
	assert.Equal(t, "project", read["binding"].(map[string]any)["project"])
	assert.Equal(t, string(db.SDDStoreModeHybrid), read["binding"].(map[string]any)["mode"])
}

func sortedJSONKeys(t *testing.T, raw any) []string {
	t.Helper()
	object, ok := raw.(map[string]any)
	require.True(t, ok, "value %#v is not a JSON object", raw)
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// TestSDDStoreBindingHTTPKeepsReleasedClosedResponseShape pins every store-binding
// response to the exact key set master emitted. Released CLI decoders reject
// unknown fields, so an additive field here breaks installed clients.
func TestSDDStoreBindingHTTPKeepsReleasedClosedResponseShape(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	require.NoError(t, store.AddAlias(context.Background(), "retired", "target", "local", "test"))
	bindingKeys := []string{"change", "created_at", "mode", "project", "provenance", "schema_version"}
	requestedKeys := []string{"change", "mode", "project", "provenance", "schema_version"}
	adopt := "/sdd/changes/change/store-binding/adopt"
	body := []byte(`{"project":"retired","mode":"hive","provenance":"governance"}`)

	for _, tt := range []struct {
		name   string
		method string
		path   string
		body   []byte
		status int
		keys   []string
		setup  func(t *testing.T)
	}{
		{name: "get missing", method: http.MethodGet, path: "/sdd/changes/change/store-binding?project=retired", status: http.StatusNotFound, keys: []string{"code"}},
		{name: "adopt created", method: http.MethodPost, path: adopt, body: body, status: http.StatusCreated, keys: []string{"binding", "created"}},
		{name: "adopt replay", method: http.MethodPost, path: adopt, body: body, status: http.StatusOK, keys: []string{"binding", "created"}},
		{name: "get found", method: http.MethodGet, path: "/sdd/changes/change/store-binding?project=retired", status: http.StatusOK, keys: []string{"binding"}},
		{name: "adopt conflict", method: http.MethodPost, path: adopt, body: []byte(`{"project":"retired","mode":"hybrid","provenance":"other"}`), status: http.StatusConflict, keys: []string{"code", "existing", "requested"}},
		{name: "adopt validation", method: http.MethodPost, path: adopt, body: []byte(`{"project":"retired","mode":"none","provenance":"source"}`), status: http.StatusUnprocessableEntity, keys: []string{"code"}},
		{name: "adopt blocked", method: http.MethodPost, path: "/sdd/changes/fresh/store-binding/adopt", body: body, status: http.StatusLocked, keys: []string{"code"}, setup: func(t *testing.T) {
			_, err := store.RawDB().Exec(`INSERT INTO project_blocks (canonical_project_key, project, command_id, blocked_at) VALUES ('target', 'target', 'block-shape', CURRENT_TIMESTAMP)`)
			require.NoError(t, err)
		}},
		{name: "zero timestamp unavailable", method: http.MethodGet, path: "/sdd/changes/zero-time/store-binding?project=retired", status: http.StatusServiceUnavailable, keys: []string{"code"}, setup: func(t *testing.T) {
			_, err := store.RawDB().Exec(`INSERT INTO sdd_store_bindings (project, change_name, schema_version, mode, provenance, created_at) VALUES ('target', 'zero-time', '1', 'hive', 'source', '0001-01-01 00:00:00')`)
			require.NoError(t, err)
		}},
		{name: "get unavailable", method: http.MethodGet, path: "/sdd/changes/change/store-binding?project=retired", status: http.StatusServiceUnavailable, keys: []string{"code"}, setup: func(t *testing.T) {
			require.NoError(t, store.Close())
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			envelope := serveIdentityJSON(t, server, tt.method, tt.path, tt.body, tt.status)
			assert.Equal(t, tt.keys, sortedJSONKeys(t, envelope))
			if binding, ok := envelope["binding"]; ok {
				assert.Equal(t, bindingKeys, sortedJSONKeys(t, binding))
				assert.Equal(t, "target", binding.(map[string]any)["project"], "the binding project carries the effective key")
			}
			if existing, ok := envelope["existing"]; ok {
				assert.Equal(t, bindingKeys, sortedJSONKeys(t, existing))
				assert.Equal(t, requestedKeys, sortedJSONKeys(t, envelope["requested"]))
			}
		})
	}
}
