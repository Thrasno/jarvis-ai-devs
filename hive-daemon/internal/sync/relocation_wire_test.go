package sync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

// TestSyncSendsPendingRelocationAsFromProject pins the wire half of the identity
// migration: a row the server already holds under the old spelling only moves if
// the push names that spelling. from_project goes out verbatim — it is the
// literal the server stores, not a name to canonicalize, and canonicalizing it
// would make it equal to the target and match nothing.
func TestSyncSendsPendingRelocationAsFromProject(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pushed":0,"pulled":[],"conflicts":0}`))
	}))
	defer server.Close()

	client := newClient(&Config{APIURL: server.URL})
	sessions := []*models.Session{
		{ID: "moved", SyncID: "moved-sync", Project: "foo-bar", DevID: "dev", Client: "test", StartedAt: time.Now(), SyncFromProject: "Foo.Bar"},
		{ID: "fresh", SyncID: "fresh-sync", Project: "foo-bar", DevID: "dev", Client: "test", StartedAt: time.Now()},
	}
	prompts := []*models.Prompt{
		{SyncID: "moved-prompt", Project: "foo-bar", Content: "c", CreatedAt: time.Now(), SyncFromProject: "Foo.Bar"},
		{SyncID: "fresh-prompt", Project: "foo-bar", Content: "c", CreatedAt: time.Now()},
	}

	_, err := client.sync(context.Background(), "token", "foo-bar", sessions, nil, prompts, nil, nil, nil, pullOptions{})
	require.NoError(t, err)

	assert.Equal(t, "Foo.Bar", fieldOf(t, body, "sessions", 0, "from_project"))
	assert.Equal(t, "Foo.Bar", fieldOf(t, body, "prompts", 0, "from_project"))

	// An ordinary push must stay a plain idempotent re-push: an empty
	// from_project matches nothing server-side, so it must be omitted entirely
	// rather than sent as "".
	assert.Nil(t, fieldOf(t, body, "sessions", 1, "from_project"), "a row with no pending relocation must send no from_project")
	assert.Nil(t, fieldOf(t, body, "prompts", 1, "from_project"), "a row with no pending relocation must send no from_project")
}

func fieldOf(t *testing.T, body map[string]any, collection string, index int, field string) any {
	t.Helper()
	rows, ok := body[collection].([]any)
	require.True(t, ok, "request has no %s array: %v", collection, body)
	require.Greater(t, len(rows), index, "%s has %d rows", collection, len(rows))
	row, ok := rows[index].(map[string]any)
	require.True(t, ok, "%s[%d] is not an object", collection, index)
	return row[field]
}
