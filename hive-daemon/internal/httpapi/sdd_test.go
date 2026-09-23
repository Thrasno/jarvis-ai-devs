package httpapi_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var updateApplyProgressFixtures = flag.Bool("update", false, "update shared apply-progress wire fixtures")

func TestApplyProgressPublishSuccessorHTTPRejectsInvalidInputs(t *testing.T) {
	_, server := newSDDHTTPServer(t)
	for _, tt := range []struct {
		name, path, body string
		status           int
		code             string
	}{
		{"missing project", "/sdd/changes/change/apply-progress/publish-successor", `{}`, 422, "validation"},
		{"unknown field", "/sdd/changes/change/apply-progress/publish-successor", `{"project":"project","snapshot":{}}`, 422, "validation"},
		{"malformed JSON", "/sdd/changes/change/apply-progress/publish-successor", `{"project":`, 422, "validation"},
		{"trailing JSON", "/sdd/changes/change/apply-progress/publish-successor", `{"project":"project"}{}`, 422, "validation"},
		{"missing change", "/sdd/changes/%20/apply-progress/publish-successor", `{"project":"project"}`, 422, "validation"},
		{"unknown project", "/sdd/changes/change/apply-progress/publish-successor", `{"project":"unknown"}`, 404, "project_not_found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body)))
			require.Equal(t, tt.status, response.Code, response.Body.String())
			var envelope map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
			assert.Equal(t, tt.code, envelope["code"])
		})
	}
}

func TestApplyProgressPublishSuccessorHTTPRejectsUnsealedPredecessor(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "project", "sdd/change/explore", "explore")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/sdd/changes/change/apply-progress/publish-successor", strings.NewReader(`{"project":"project"}`)))
	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())

	var fixture struct {
		State struct {
			Snapshot applyprogress.Snapshot `json:"snapshot"`
		} `json:"state"`
	}
	require.NoError(t, json.Unmarshal(sharedApplyProgressGetFixture(t), &fixture))
	_, err := store.AdvanceApplyProgress(db.ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "publish-http-base", Snapshot: fixture.State.Snapshot, Batches: sharedApplyProgressBatches(t)})
	require.NoError(t, err)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/sdd/changes/change/apply-progress/publish-successor", strings.NewReader(`{"project":"project"}`)))
	require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
}

func TestApplyProgressPublishSuccessorSQLiteHTTP(t *testing.T) {
	for _, blockedBeforePublish := range []bool{false, true} {
		name := "committed replay blocked"
		if blockedBeforePublish {
			name = "blocked fresh publication"
		}
		t.Run(name, func(t *testing.T) {
			store, server := newSDDHTTPServer(t)
			saveSDDHTTPMemory(t, store, "project", "sdd/change/explore", "explore")
			var fixture struct {
				State struct {
					Snapshot applyprogress.Snapshot `json:"snapshot"`
				} `json:"state"`
			}
			require.NoError(t, json.Unmarshal(sharedApplyProgressGetFixture(t), &fixture))
			base, err := store.AdvanceApplyProgress(db.ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "publish-http-base", Snapshot: fixture.State.Snapshot, Batches: sharedApplyProgressBatches(t)})
			require.NoError(t, err)
			tasks := "- [ ] 1.1 successor task\n"
			saveSDDHTTPMemory(t, store, "project", "sdd/change/tasks", tasks)
			parsed, err := applyprogress.ParseTasksMarkdown(tasks)
			require.NoError(t, err)
			_, manifest, err := applyprogress.TaskManifest(parsed.Tasks)
			require.NoError(t, err)
			seal := base.State.Snapshot
			seal.Schema, seal.Status = applyprogress.SupersessionSnapshotSchema, applyprogress.StatusSuperseded
			seal.Revision++
			seal.PreviousDigest = base.State.Digest
			seal.StreamSHA256, seal.NextEntryIndex, seal.NextEntryID = "", 0, ""
			seal.SealIntent = &applyprogress.SealIntent{SuccessorProject: "project", SuccessorChange: "next", SuccessorManifestSHA256: manifest, Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "publish-http-seal"}
			seal, _, err = applyprogress.SealSnapshot(seal)
			require.NoError(t, err)
			_, err = store.AdvanceApplyProgress(db.ApplyProgressAdvance{Project: "project", Change: "change", RequestID: "publish-http-seal", ExpectedGeneration: base.State.Generation, ExpectedRevision: base.State.Revision, ExpectedDigest: base.State.Digest, Snapshot: seal, Batches: []applyprogress.Batch{}})
			require.NoError(t, err)
			publish := func() *httptest.ResponseRecorder {
				response := httptest.NewRecorder()
				server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/sdd/changes/change/apply-progress/publish-successor", strings.NewReader(`{"project":"project"}`)))
				return response
			}
			block := func() {
				_, err := store.RecordProjectBlock(t.Context(), db.ProjectBlockCommand{Project: "project", CommandID: "block-http", AckToken: "ack-http", Reason: "test", Action: "block", Generation: 1})
				require.NoError(t, err)
			}
			if blockedBeforePublish {
				block()
			}
			first := publish()
			if blockedBeforePublish {
				require.Equal(t, http.StatusLocked, first.Code, first.Body.String())
				var envelope map[string]any
				require.NoError(t, json.Unmarshal(first.Body.Bytes(), &envelope))
				assert.Equal(t, "project_blocked", envelope["code"])
				_, err := store.GetApplyProgress("project", "next")
				require.ErrorIs(t, err, db.ErrApplyProgressNotFound)
			} else {
				require.Equal(t, http.StatusOK, first.Code, first.Body.String())
				var result struct {
					Outcome string                  `json:"outcome"`
					State   db.ApplyProgressState   `json:"state"`
					Receipt db.ApplyProgressReceipt `json:"receipt"`
				}
				require.NoError(t, json.Unmarshal(first.Body.Bytes(), &result))
				assert.Equal(t, "committed", result.Outcome)
				assert.Equal(t, uint64(1), result.State.Generation)
				assert.Equal(t, uint64(1), result.State.Revision)
				require.NotNil(t, result.State.Snapshot.Supersedes)
				assert.Equal(t, seal.Digest, result.State.Snapshot.Supersedes.SealDigest)
				assert.Equal(t, "change", result.State.Snapshot.Supersedes.Change)
				require.NotEmpty(t, result.Receipt.RequestID)
				before, err := store.GetApplyProgress("project", "next")
				require.NoError(t, err)
				block()
				replayed := publish()
				require.Equal(t, http.StatusOK, replayed.Code, replayed.Body.String())
				assert.JSONEq(t, first.Body.String(), replayed.Body.String())
				stored, err := store.GetApplyProgress("project", "next")
				require.NoError(t, err)
				assert.Equal(t, before, stored, "blocked replay must not mutate the head")
			}
			advance := httptest.NewRecorder()
			body, err := json.Marshal(db.ApplyProgressAdvance{Project: "project", Change: "other", RequestID: "blocked-http-advance", Snapshot: fixture.State.Snapshot, Batches: sharedApplyProgressBatches(t)})
			require.NoError(t, err)
			server.ServeHTTP(advance, httptest.NewRequest(http.MethodPost, "/sdd/changes/change/apply-progress/advance", bytes.NewReader(body)))
			require.Equal(t, http.StatusLocked, advance.Code, advance.Body.String())
			var blocked map[string]any
			require.NoError(t, json.Unmarshal(advance.Body.Bytes(), &blocked))
			assert.Equal(t, "project_blocked", blocked["code"])
		})
	}
}

func TestSDDStoreBindingHTTPCreatedReplayConflictAndProjection(t *testing.T) {
	_, server := newSDDHTTPServer(t)
	path := "/sdd/changes/change%25_/store-binding/adopt"
	body := `{"project":"project","mode":"hive","provenance":"governance"}`

	created := httptest.NewRecorder()
	server.ServeHTTP(created, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var createdEnvelope struct {
		Binding struct {
			Project       string `json:"project"`
			Change        string `json:"change"`
			SchemaVersion string `json:"schema_version"`
			Mode          string `json:"mode"`
			Provenance    string `json:"provenance"`
			CreatedAt     string `json:"created_at"`
		} `json:"binding"`
		Created *bool `json:"created"`
	}
	require.NoError(t, json.NewDecoder(created.Body).Decode(&createdEnvelope))
	require.NotNil(t, createdEnvelope.Created)
	assert.True(t, *createdEnvelope.Created)
	assert.Equal(t, "project", createdEnvelope.Binding.Project)
	assert.Equal(t, "change%_", createdEnvelope.Binding.Change)
	assert.Equal(t, "1", createdEnvelope.Binding.SchemaVersion)
	assert.Equal(t, "hive", createdEnvelope.Binding.Mode)
	assert.Equal(t, "governance", createdEnvelope.Binding.Provenance)
	parsedCreatedAt, err := time.Parse(time.RFC3339, createdEnvelope.Binding.CreatedAt)
	require.NoError(t, err)
	assert.Equal(t, createdEnvelope.Binding.CreatedAt, parsedCreatedAt.UTC().Format(time.RFC3339))

	replayed := httptest.NewRecorder()
	server.ServeHTTP(replayed, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	require.Equal(t, http.StatusOK, replayed.Code, replayed.Body.String())
	var replayedEnvelope struct {
		Binding map[string]any `json:"binding"`
		Created *bool          `json:"created"`
	}
	require.NoError(t, json.NewDecoder(replayed.Body).Decode(&replayedEnvelope))
	require.NotNil(t, replayedEnvelope.Created)
	assert.False(t, *replayedEnvelope.Created)
	assert.Equal(t, "hive", replayedEnvelope.Binding["mode"])

	get := httptest.NewRecorder()
	server.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/sdd/changes/change%25_/store-binding?project=project", nil))
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())
	var getEnvelope map[string]any
	require.NoError(t, json.NewDecoder(get.Body).Decode(&getEnvelope))
	assert.NotContains(t, getEnvelope, "created")
	assert.Equal(t, "change%_", getEnvelope["binding"].(map[string]any)["change"])

	conflict := httptest.NewRecorder()
	server.ServeHTTP(conflict, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"project":"project","mode":"hybrid","provenance":"other"}`)))
	require.Equal(t, http.StatusConflict, conflict.Code, conflict.Body.String())
	var conflictEnvelope map[string]any
	require.NoError(t, json.NewDecoder(conflict.Body).Decode(&conflictEnvelope))
	assert.Equal(t, "binding_conflict", conflictEnvelope["code"])
	existing := conflictEnvelope["existing"].(map[string]any)
	assert.Equal(t, "project", existing["project"])
	assert.Equal(t, "change%_", existing["change"])
	assert.Equal(t, "1", existing["schema_version"])
	assert.Equal(t, "hive", existing["mode"])
	assert.Equal(t, "governance", existing["provenance"])
	assert.Equal(t, createdEnvelope.Binding.CreatedAt, existing["created_at"])
	requested := conflictEnvelope["requested"].(map[string]any)
	assert.Equal(t, "project", requested["project"])
	assert.Equal(t, "change%_", requested["change"])
	assert.Equal(t, "1", requested["schema_version"])
	assert.Equal(t, "hybrid", requested["mode"])
	assert.Equal(t, "other", requested["provenance"])
	assert.NotContains(t, requested, "created_at")
}

func TestSDDStoreBindingHTTPRejectsMissingAndInvalidRequests(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	missing := httptest.NewRecorder()
	server.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/sdd/changes/change/store-binding?project=project", nil))
	require.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())
	assertSDDStoreBindingCode(t, missing, "not_found")

	for _, tt := range []struct {
		name, method, path, body string
	}{
		{name: "invalid mode", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"project":"project","mode":"none","provenance":"source"}`},
		{name: "blank provenance", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"project":"project","mode":"hive","provenance":" "}`},
		{name: "blank project", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"project":" ","mode":"hive","provenance":"source"}`},
		{name: "invalid change", method: http.MethodPost, path: "/sdd/changes/a%5Cb/store-binding/adopt", body: `{"project":"project","mode":"hive","provenance":"source"}`},
		{name: "caller schema", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"project":"project","mode":"hive","provenance":"source","schema_version":"2"}`},
		{name: "caller change", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"project":"project","change":"other","mode":"hive","provenance":"source"}`},
		{name: "duplicate project", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"project":"alpha","project":"beta","mode":"hive","provenance":"source"}`},
		{name: "duplicate mode", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"project":"project","mode":"hive","mode":"hybrid","provenance":"source"}`},
		{name: "duplicate provenance", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"project":"project","mode":"hive","provenance":"first","provenance":"second"}`},
		{name: "case alias", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"Project":"project","mode":"hive","provenance":"source"}`},
		{name: "null project", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"project":null,"mode":"hive","provenance":"source"}`},
		{name: "numeric mode", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"project":"project","mode":1,"provenance":"source"}`},
		{name: "boolean provenance", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"project":"project","mode":"hive","provenance":true}`},
		{name: "missing query project", method: http.MethodGet, path: "/sdd/changes/change/store-binding"},
		{name: "duplicate query project", method: http.MethodGet, path: "/sdd/changes/change/store-binding?project=alpha&project=beta"},
		{name: "trailing json", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: `{"project":"project","mode":"hive","provenance":"source"} {}`},
		{name: "oversized body", method: http.MethodPost, path: "/sdd/changes/change/store-binding/adopt", body: fmt.Sprintf(`{"project":"project","mode":"hive","provenance":%q}`, strings.Repeat("x", 64<<10))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
			assertSDDStoreBindingCode(t, response, "validation")
		})
	}

	require.NoError(t, store.Close())
	unavailable := httptest.NewRecorder()
	server.ServeHTTP(unavailable, httptest.NewRequest(http.MethodGet, "/sdd/changes/change/store-binding?project=project", nil))
	require.Equal(t, http.StatusServiceUnavailable, unavailable.Code, unavailable.Body.String())
	var unavailableEnvelope map[string]any
	require.NoError(t, json.NewDecoder(unavailable.Body).Decode(&unavailableEnvelope))
	assert.Equal(t, map[string]any{"code": "unavailable"}, unavailableEnvelope)
}

func TestSDDStoreBindingHTTPRejectsZeroTimestampProjection(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	_, err := store.RawDB().Exec(`INSERT INTO sdd_store_bindings (project, change_name, schema_version, mode, provenance, created_at) VALUES ('project', 'zero-time', '1', 'hive', 'source', '0001-01-01 00:00:00')`)
	require.NoError(t, err)

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sdd/changes/zero-time/store-binding?project=project", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
	assertSDDStoreBindingCode(t, response, "unavailable")

	conflict := httptest.NewRecorder()
	server.ServeHTTP(conflict, httptest.NewRequest(http.MethodPost, "/sdd/changes/zero-time/store-binding/adopt", strings.NewReader(`{"project":"project","mode":"hybrid","provenance":"other"}`)))
	require.Equal(t, http.StatusServiceUnavailable, conflict.Code, conflict.Body.String())
	assertSDDStoreBindingCode(t, conflict, "unavailable")
}

func assertSDDStoreBindingCode(t *testing.T, response *httptest.ResponseRecorder, want string) {
	t.Helper()
	var envelope map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&envelope))
	assert.Equal(t, want, envelope["code"])
}

func TestSDDArtifactsHTTPUsesSQLiteProjection(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "project name", "sdd/change%_/explore", "old")
	saveSDDHTTPMemory(t, store, "project name", "sdd/change%_/explore", "new")
	saveSDDHTTPMemory(t, store, "project name", "sdd/change%_/proposal", "proposal")

	request := httptest.NewRequest(http.MethodGet, "/sdd/changes/change%25_/artifacts?project="+url.QueryEscape("project name"), nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Artifacts []governance.SDDArtifact `json:"artifacts"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Len(t, body.Artifacts, 2)
	assert.Equal(t, "explore", body.Artifacts[0].Artifact)
	assert.Equal(t, "new", body.Artifacts[0].Content)
	assert.Equal(t, "proposal", body.Artifacts[1].Artifact)
}

func TestSDDChangesHTTPReturnsOpaqueKeysetPages(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	for _, change := range []string{"alpha", "bravo", "charlie"} {
		saveSDDHTTPMemory(t, store, "project", "sdd/"+change+"/explore", change)
	}
	first := httptest.NewRecorder()
	server.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/sdd/changes?project=project&limit=2", nil))
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var page governance.SDDChangePage
	require.NoError(t, json.NewDecoder(first.Body).Decode(&page))
	assert.Equal(t, []string{"alpha", "bravo"}, page.Changes)
	assert.NotEmpty(t, page.NextCursor)

	second := httptest.NewRecorder()
	path := "/sdd/changes?project=project&limit=2&cursor=" + url.QueryEscape(page.NextCursor)
	server.ServeHTTP(second, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	page = governance.SDDChangePage{}
	require.NoError(t, json.NewDecoder(second.Body).Decode(&page))
	assert.Equal(t, []string{"charlie"}, page.Changes)
	assert.Empty(t, page.NextCursor)
}

func TestSDDHTTPValidation(t *testing.T) {
	_, server := newSDDHTTPServer(t)
	tests := []struct {
		name string
		path string
	}{
		{name: "missing project artifacts", path: "/sdd/changes/change/artifacts"},
		{name: "missing project changes", path: "/sdd/changes?limit=2"},
		{name: "invalid limit", path: "/sdd/changes?project=project&limit=0"},
		{name: "invalid cursor", path: "/sdd/changes?project=project&limit=2&cursor=bad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
			assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		})
	}
}

func TestApplyProgressHTTPReceiptIdentityIsBoundAndDoesNotExposeState(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "project", "sdd/change/explore", "explore")
	var fixture struct {
		State struct {
			Snapshot applyprogress.Snapshot `json:"snapshot"`
			Batches  []applyprogress.Batch  `json:"batches"`
		} `json:"state"`
	}
	require.NoError(t, json.Unmarshal(sharedApplyProgressGetFixture(t), &fixture))
	fixture.State.Batches = sharedApplyProgressBatches(t)
	advance, err := json.Marshal(map[string]any{"project": "project", "request_id": "receipt-http", "snapshot": fixture.State.Snapshot, "batches": fixture.State.Batches})
	require.NoError(t, err)
	committed := httptest.NewRecorder()
	server.ServeHTTP(committed, httptest.NewRequest(http.MethodPost, "/sdd/changes/change/apply-progress/advance", bytes.NewReader(advance)))
	require.Equal(t, http.StatusOK, committed.Code, committed.Body.String())

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sdd/changes/change/apply-progress/receipts/receipt-http?project=project", nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	receipt, ok := body["receipt"].(map[string]any)
	require.True(t, ok, "response=%#v", body)
	assert.Equal(t, "receipt-http", receipt["request_id"])
	assert.NotEmpty(t, receipt["payload_sha256"])
	assert.NotContains(t, body, "state")

	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sdd/changes/other-change/apply-progress/receipts/receipt-http?project=project", nil))
	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
}

func TestApplyProgressHTTPAdvancesAndMatchesSharedWireFixtures(t *testing.T) {
	store, handler := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "project", "sdd/change/explore", "explore")
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	var fixture struct {
		State struct {
			Snapshot applyprogress.Snapshot `json:"snapshot"`
		} `json:"state"`
	}
	require.NoError(t, json.Unmarshal(sharedApplyProgressGetFixture(t), &fixture))
	batches := sharedApplyProgressBatches(t)
	advance, err := json.Marshal(map[string]any{"project": "project", "request_id": "http-request", "snapshot": fixture.State.Snapshot, "batches": batches})
	require.NoError(t, err)
	response, err := server.Client().Post(server.URL+"/sdd/changes/change/apply-progress/advance", "application/json", bytes.NewReader(advance))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())

	progressResponse, err := server.Client().Get(server.URL + "/sdd/changes/change/apply-progress?project=project")
	require.NoError(t, err)
	progressBody := requireHTTPBody(t, progressResponse)
	require.Equal(t, http.StatusOK, progressResponse.StatusCode)
	updateSharedApplyProgressFixture(t, "apply-progress-get.json", progressBody)
	require.Equal(t, sharedApplyProgressGetFixture(t), progressBody)

	for _, batch := range batches {
		evidenceResponse, err := server.Client().Get(server.URL + "/sdd/changes/change/apply-evidence/" + batch.BatchID + "?project=project&expected_head_digest=" + fixture.State.Snapshot.Digest)
		require.NoError(t, err)
		evidenceBody := requireHTTPBody(t, evidenceResponse)
		require.Equal(t, http.StatusOK, evidenceResponse.StatusCode)
		updateSharedApplyProgressFixture(t, "apply-evidence-"+batch.BatchID+".json", evidenceBody)
		require.Equal(t, sharedApplyProgressEvidenceFixture(t, batch.BatchID), evidenceBody)
	}

	staleAdvance := bytes.Replace(advance, []byte("http-request"), []byte("http-stale"), 1)
	staleResponse, err := server.Client().Post(server.URL+"/sdd/changes/change/apply-progress/advance", "application/json", bytes.NewReader(staleAdvance))
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, staleResponse.StatusCode)
	require.NoError(t, staleResponse.Body.Close())
}

func TestApplyProgressHTTPRejectsUnprovenImportedEvidenceWithTypedLegacyMigration(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "project", "sdd/change/explore", "explore")
	var fixture struct {
		State struct {
			Snapshot applyprogress.Snapshot `json:"snapshot"`
			Batches  []applyprogress.Batch  `json:"batches"`
		} `json:"state"`
	}
	require.NoError(t, json.Unmarshal(sharedApplyProgressGetFixture(t), &fixture))
	fixture.State.Batches = sharedApplyProgressBatches(t)
	fixture.State.Batches[0].Entries[0].Kind = applyprogress.EvidenceImported
	var err error
	fixture.State.Batches[0], _, err = applyprogress.SealBatch(fixture.State.Batches[0])
	require.NoError(t, err)
	fixture.State.Snapshot.Batches[0].SHA256 = fixture.State.Batches[0].SHA256
	fixture.State.Snapshot, _, err = applyprogress.SealSnapshot(fixture.State.Snapshot)
	require.NoError(t, err)
	body, err := json.Marshal(map[string]any{
		"project": "project", "request_id": "http-unproven-import", "legacy_source_sha256": strings.Repeat("a", 64),
		"snapshot": fixture.State.Snapshot, "batches": fixture.State.Batches,
	})
	require.NoError(t, err)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/sdd/changes/change/apply-progress/advance", bytes.NewReader(body)))
	require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
	var result map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	assert.Equal(t, "invalid", result["outcome"])
	assert.Equal(t, "legacy_migration", result["code"])
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sdd/changes/change/apply-progress?project=project", nil))
	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
}

// sharedApplyProgressGetFixture resolves from this source file, not the test process
// working directory, because Go runs packages from different module subdirectories.
func sharedApplyProgressGetFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(sharedApplyProgressFixturePath(t, "apply-progress-get.json"))
	require.NoError(t, err)
	return data
}

func updateSharedApplyProgressFixture(t *testing.T, name string, data []byte) {
	t.Helper()
	if *updateApplyProgressFixtures {
		require.NoError(t, os.WriteFile(sharedApplyProgressFixturePath(t, name), data, 0o600))
	}
}

func sharedApplyProgressFixturePath(t *testing.T, name string) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve fixture source path")
	return filepath.Join(filepath.Dir(source), "..", "..", "..", "testdata", "sdd-progress", name)
}

func TestApplyProgressHTTPGetDistinguishesAbsentHeadAndProject(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "project", "sdd/change/explore", "explore")
	for _, tt := range []struct {
		name, project, wantCode string
	}{
		{name: "head not found", project: "project", wantCode: "not_found"},
		{name: "project not found", project: "missing", wantCode: "project_not_found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sdd/changes/change/apply-progress?project="+tt.project, nil))
			require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
			var envelope map[string]any
			require.NoError(t, json.NewDecoder(response.Body).Decode(&envelope))
			require.Equal(t, "not_found", envelope["outcome"])
			require.Equal(t, tt.wantCode, envelope["code"])
			require.NotContains(t, envelope["recovery"], "retry the same request ID")
		})
	}
}

func TestApplyProgressHTTPSnapshotCapacityUsesSharedRecovery(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "project", "sdd/change/explore", "explore")
	snapshot := applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "project", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: strings.Repeat("a", 64), Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}}
	for i := 0; i < 500; i++ {
		snapshot.Batches = append(snapshot.Batches, applyprogress.BatchRef{BatchID: fmt.Sprintf("apb-%032x", i+1), SHA256: strings.Repeat("a", 64)})
	}
	body, err := json.Marshal(map[string]any{"project": "project", "request_id": "snapshot-capacity", "snapshot": snapshot, "batches": []applyprogress.Batch{}})
	require.NoError(t, err)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/sdd/changes/change/apply-progress/advance", bytes.NewReader(body)))
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code, response.Body.String())
	var envelope struct {
		Outcome  string `json:"outcome"`
		Code     string `json:"code"`
		Recovery string `json:"recovery"`
		Capacity struct {
			CurrentRunes   int `json:"current_runes"`
			ProjectedRunes int `json:"projected_runes"`
			CeilingRunes   int `json:"ceiling_runes"`
		} `json:"capacity"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&envelope))
	assert.Equal(t, string(applyprogress.PlanSnapshotCapacityExhausted), envelope.Outcome)
	assert.Equal(t, string(applyprogress.PlanSnapshotCapacityExhausted), envelope.Code)
	assert.Equal(t, applyprogress.SnapshotCapacityRecovery, envelope.Recovery)
	assert.Zero(t, envelope.Capacity.CurrentRunes)
	assert.Greater(t, envelope.Capacity.ProjectedRunes, applyprogress.MaxDocumentRunes)
	assert.Equal(t, applyprogress.MaxDocumentRunes, envelope.Capacity.CeilingRunes)
}

func TestApplyProgressHTTPCapacityIncludesProtocolDiagnostics(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "project", "sdd/change/explore", "explore")
	batch := applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "project", Change: "change", BatchID: "apb-67676767676767676767676767676767", Entries: []applyprogress.EvidenceEntry{{EntryID: "entry", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: strings.Repeat("界", 40_000), Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}}
	_, _, err := applyprogress.SealBatch(batch)
	var capacity *applyprogress.CapacityError
	if !errors.As(err, &capacity) {
		t.Fatalf("SealBatch() error = %v, want CapacityError", err)
	}
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "project", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: strings.Repeat("a", 64), Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
	require.NoError(t, err)
	body, err := json.Marshal(map[string]any{"project": "project", "request_id": "http-capacity", "snapshot": snapshot, "batches": []applyprogress.Batch{batch}})
	require.NoError(t, err)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/sdd/changes/change/apply-progress/advance", bytes.NewReader(body)))
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code, response.Body.String())
	var envelope struct {
		Outcome  string `json:"outcome"`
		Code     string `json:"code"`
		Capacity struct {
			Document string `json:"document"`
			Runes    int    `json:"runes"`
			Limit    int    `json:"limit"`
		} `json:"capacity"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&envelope))
	assert.Equal(t, "invalid", envelope.Outcome)
	assert.Equal(t, "capacity", envelope.Code)
	assert.Equal(t, capacity.Document, envelope.Capacity.Document)
	assert.Equal(t, capacity.Runes, envelope.Capacity.Runes)
	assert.Equal(t, applyprogress.MaxDocumentRunes, envelope.Capacity.Limit)
}

func TestApplyProgressHTTPErrorEnvelopes(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	for _, tt := range []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
		want     string
	}{
		{name: "body capacity", method: http.MethodPost, path: "/sdd/changes/change/apply-progress/advance", body: strings.Repeat(" ", 1<<20+1), wantCode: http.StatusRequestEntityTooLarge, want: "capacity"},
		{name: "invalid payload", method: http.MethodPost, path: "/sdd/changes/change/apply-progress/advance", body: "{", wantCode: http.StatusUnprocessableEntity, want: "validation"},
		{name: "missing project", method: http.MethodPost, path: "/sdd/changes/change/apply-progress/advance", body: `{}`, wantCode: http.StatusUnprocessableEntity, want: "validation"},
		{name: "missing project read", method: http.MethodGet, path: "/sdd/changes/change/apply-progress", wantCode: http.StatusUnprocessableEntity, want: "validation"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body)))
			require.Equal(t, tt.wantCode, response.Code, response.Body.String())
			var envelope map[string]any
			require.NoError(t, json.NewDecoder(response.Body).Decode(&envelope))
			require.Equal(t, tt.want, envelope["code"])
			require.NotEmpty(t, envelope["recovery"])
		})
	}

	require.NoError(t, store.Close())
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sdd/changes/change/apply-progress?project=project", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
	var unavailable map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&unavailable))
	require.Equal(t, "unavailable", unavailable["code"])
}

func sharedApplyProgressBatches(t *testing.T) []applyprogress.Batch {
	t.Helper()
	batches := make([]applyprogress.Batch, 0, 2)
	for _, batchID := range []string{"apb-00000000000000000000000000000001", "apb-00000000000000000000000000000002"} {
		var envelope struct {
			Batch applyprogress.Batch `json:"batch"`
		}
		require.NoError(t, json.Unmarshal(sharedApplyProgressEvidenceFixture(t, batchID), &envelope))
		batches = append(batches, envelope.Batch)
	}
	return batches
}

func sharedApplyProgressEvidenceFixture(t *testing.T, batchID string) []byte {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve fixture source path")
	fixture := filepath.Join(filepath.Dir(source), "..", "..", "..", "testdata", "sdd-progress", "apply-evidence-"+batchID+".json")
	data, err := os.ReadFile(fixture)
	require.NoError(t, err)
	return data
}

func requireHTTPBody(t *testing.T, response *http.Response) []byte {
	t.Helper()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	return data
}

func TestSuccessorOccupancyHTTPProjectionAndValidation(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "project", "sdd/occupied/explore", "secret-content")
	// Snapshot all durable occupancy sources and governance registration before
	// each request: even an unsuccessful GET must not create a reservation.
	rowCounts := func(t *testing.T) map[string]int {
		t.Helper()
		counts := make(map[string]int)
		for _, table := range []string{"memories", "sdd_apply_heads", "sdd_apply_receipts", "sdd_store_bindings", "project_identities"} {
			var count int
			err := store.RawDB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
			require.NoError(t, err, "count %s", table)
			counts[table] = count
		}
		return counts
	}
	for _, tt := range []struct {
		name, path string
		status     int
		occupied   bool
		category   string
	}{
		{"free", "/sdd/changes/free/successor-occupancy?project=project", http.StatusOK, false, ""},
		{"occupied", "/sdd/changes/occupied/successor-occupancy?project=project", http.StatusOK, true, "memory"},
		{"missing project", "/sdd/changes/free/successor-occupancy", http.StatusUnprocessableEntity, false, ""},
		{"unknown project", "/sdd/changes/free/successor-occupancy?project=unknown", http.StatusNotFound, false, ""},
		{"invalid change", "/sdd/changes/%5C/successor-occupancy?project=project", http.StatusUnprocessableEntity, false, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := rowCounts(t)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
			assert.Equal(t, before, rowCounts(t), "GET must not mutate occupancy or project registration")
			require.Equal(t, tt.status, response.Code, response.Body.String())
			if tt.status != http.StatusOK {
				return
			}
			var body map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			require.Len(t, body, 2)
			assert.Equal(t, tt.occupied, body["occupied"])
			assert.Equal(t, tt.category, body["category"])
			assert.NotContains(t, response.Body.String(), "secret-content")
		})
	}
}

func newSDDHTTPServer(t *testing.T) (*db.DB, *httpapi.Server) {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "hive.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	service := governance.NewService(store)
	return store, httpapi.NewServerWithGovernance("127.0.0.1:0", store, service)
}

func saveSDDHTTPMemory(t *testing.T, store *db.DB, project, topic, content string) {
	t.Helper()
	_, err := store.SaveMemoryWithManualSession(&models.Memory{Project: project, TopicKey: &topic, Title: topic, Content: content})
	require.NoError(t, err)
}
