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

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var updateApplyProgressFixtures = flag.Bool("update", false, "update shared apply-progress wire fixtures")

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
