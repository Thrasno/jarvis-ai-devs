package sync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyncDeclaresTheOpsThisBuildCanApply pins the client half of the capability
// handshake. The server withholds every mutation op the client did not declare,
// because delivering one the client cannot apply would abort its batch before
// the cursor advanced. Declaring mutation.reproject is therefore a promise about
// ApplyRemoteMutation, and it may only be made now that it is true.
func TestSyncDeclaresTheOpsThisBuildCanApply(t *testing.T) {
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
	_, err := client.sync(context.Background(), "token", "foo-bar", nil, nil, nil, nil, nil, nil, pullOptions{})
	require.NoError(t, err)

	declared, ok := body["sync_capabilities"].([]any)
	require.True(t, ok, "request declared no sync_capabilities: %v", body)
	assert.Contains(t, declared, "mutation.reproject",
		"the exact string the server matches on — a near miss silently loses every reproject forever")
}
