package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
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
