package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestApplyProgressHTTPAdvancesAndReadsCurrentState(t *testing.T) {
	store, server := newSDDHTTPServer(t)
	saveSDDHTTPMemory(t, store, "project", "sdd/change/explore", "explore")
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "project", Change: "change", BatchID: "apb-66666666666666666666666666666666", Entries: []applyprogress.EvidenceEntry{{EntryID: "entry", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	require.NoError(t, err)
	snapshot, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "project", Change: "change", Generation: 1, Revision: 1, TaskManifestSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	require.NoError(t, err)
	body, err := json.Marshal(map[string]any{"project": "project", "request_id": "http-request", "snapshot": snapshot, "batches": []applyprogress.Batch{batch}})
	require.NoError(t, err)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/sdd/changes/change/apply-progress/advance", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sdd/changes/change/apply-progress?project=project", nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	stale, err := json.Marshal(map[string]any{"project": "project", "request_id": "http-stale", "snapshot": snapshot, "batches": []applyprogress.Batch{batch}})
	require.NoError(t, err)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/sdd/changes/change/apply-progress/advance", bytes.NewReader(stale)))
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	var conflict map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&conflict))
	require.Equal(t, "stale", conflict["code"])
	require.NotEmpty(t, conflict["recovery"])
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

func newSDDHTTPServer(t *testing.T) (*db.DB, *httpapi.Server) {
	t.Helper()
	store, err := db.Open(":memory:")
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
