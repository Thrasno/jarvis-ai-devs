package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
)

// TestSyncDoesNotCanonicalizeTheCallersMutations pins that the wire
// canonicalization in sync() is a projection, not an edit of its input.
//
// The envelopes handed to sync() are the caller's journal rows. sync() sends
// them under the canonical spelling, but the caller still holds the raw literals
// afterwards — it needs them to correlate the response back to the journal, and
// a retry must resend the same rows it read.
//
// The regression this guards is specific: copying the slice with append() copies
// the structs but not what their pointers reach, so writing through
// envelope.Memory.Project reached straight back into the caller's payload while
// the sibling write to envelope.Project stayed local to the copy.
func TestSyncDoesNotCanonicalizeTheCallersMutations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pushed":0,"pulled":[],"conflicts":0}`))
	}))
	defer server.Close()

	payload := &db.MutationMemoryPayload{SyncID: "mem-sync", Project: "Foo.Bar", Title: "t", Content: "c"}
	mutations := []db.MutationEnvelope{{
		EventID:      "event-1",
		EntityType:   "memory",
		EntitySyncID: "mem-sync",
		Project:      "Foo.Bar",
		Op:           db.MutationOpCreate,
		Sequence:     1,
		OccurredAt:   time.Now().UTC(),
		Memory:       payload,
	}}

	client := newClient(&Config{APIURL: server.URL})
	_, err := client.sync(context.Background(), "token", "Foo.Bar", nil, nil, nil, nil, mutations, nil, pullOptions{})
	require.NoError(t, err)

	assert.Equal(t, "Foo.Bar", mutations[0].Project, "sync must not rewrite the caller's envelope")
	assert.Equal(t, "Foo.Bar", mutations[0].Memory.Project, "sync must not rewrite the memory payload the caller still points at")
	assert.Equal(t, "Foo.Bar", payload.Project, "the caller's payload is shared by pointer and must survive the send")
}
