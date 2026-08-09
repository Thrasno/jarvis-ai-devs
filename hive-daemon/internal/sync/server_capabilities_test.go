package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The daemon already SENDS sync_capabilities and hive-api already echoes back
// what it understands, but syncResponse had no field for the echo — so the
// daemon never learned what the server supports and pushed reproject at any
// server unconditionally.
//
// That is only survivable while the API is deployed before the daemon. On the
// residual case (an API rollback after a release is out) the old server hard-errors
// on the unknown op; the mutations are never REJECTED, and only rejection drops
// them, so they resend forever and sync is dead with nothing saying why.
func TestSyncLearnsWhichOpsTheServerDeclares(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pushed":0,"pulled":[],"conflicts":0,"sync_capabilities":["mutation.reproject"]}`))
	}))
	defer server.Close()

	client := newClient(&Config{APIURL: server.URL})
	assert.False(t, client.serverSupports(syncCapabilityReproject),
		"nothing is known about the server before the first response")

	resp, err := client.sync(context.Background(), "token", "foo-bar", nil, nil, nil, nil, nil, nil, pullOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{syncCapabilityReproject}, resp.SyncCapabilities,
		"the server's declaration must survive decoding")
	assert.True(t, client.serverSupports(syncCapabilityReproject))
}

func TestSyncTreatsASilentServerAsNotSupportingReproject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// An API that predates the field says nothing at all.
		_, _ = w.Write([]byte(`{"pushed":0,"pulled":[],"conflicts":0}`))
	}))
	defer server.Close()

	client := newClient(&Config{APIURL: server.URL})
	_, err := client.sync(context.Background(), "token", "foo-bar", nil, nil, nil, nil, nil, nil, pullOptions{})
	require.NoError(t, err)

	assert.False(t, client.serverSupports(syncCapabilityReproject),
		"an old server that never declares the op must never be sent one")
}

// A server that once declared the op and then answers without the field has not
// withdrawn it: hive-api omits sync_capabilities via omitempty, and forgetting on
// every quiet response would flap the daemon between pushing and withholding.
func TestServerCapabilitiesAreNotWithdrawnByASilentResponse(t *testing.T) {
	declare := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if declare {
			_, _ = w.Write([]byte(`{"pushed":0,"pulled":[],"conflicts":0,"sync_capabilities":["mutation.reproject"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"pushed":0,"pulled":[],"conflicts":0}`))
	}))
	defer server.Close()

	client := newClient(&Config{APIURL: server.URL})
	_, err := client.sync(context.Background(), "token", "foo-bar", nil, nil, nil, nil, nil, nil, pullOptions{})
	require.NoError(t, err)
	declare = false
	_, err = client.sync(context.Background(), "token", "foo-bar", nil, nil, nil, nil, nil, nil, pullOptions{})
	require.NoError(t, err)

	assert.True(t, client.serverSupports(syncCapabilityReproject))
}

// The end-to-end shape of the wedge: a reproject sitting in the journal must not
// reach a server that never declared the op, and must not be acked either — it
// waits in the journal for a server that understands it.
func TestSyncerWithholdsReprojectFromAServerThatNeverDeclaredIt(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	reproject := db.MutationEnvelope{
		EventID: "move-me", EntityType: "memory", EntitySyncID: "memory-move", Project: "test-project",
		Op: db.MutationOpReproject, OccurredAt: now,
		Reproject: &db.MutationReprojectPayload{FromProject: "Test.Project", ToProject: "test-project"},
	}
	delete := db.MutationEnvelope{
		EventID: "delete-me", EntityType: "memory", EntitySyncID: "memory-delete", Project: "test-project",
		Op: db.MutationOpDelete, OccurredAt: now,
	}
	store := &mockSyncStore{jwt: "valid-token", pendingMutations: []db.MutationEnvelope{reproject, delete}}

	var sentOps [][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"commands": []any{}}))
			return
		}
		var req struct {
			Mutations []db.MutationEnvelope `json:"mutations"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		ops := make([]string, 0, len(req.Mutations))
		for _, mutation := range req.Mutations {
			ops = append(ops, string(mutation.Op))
		}
		sentOps = append(sentOps, ops)
		// Deliberately silent on sync_capabilities: an API that predates it.
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
			CompatibilityMode: compatibilityModeMutationV2,
			MutationResults:   []mutationResult{{EventID: delete.EventID, Applied: true}},
			Pulled:            []apiMemory{},
		}))
	}))
	defer server.Close()

	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store,
		syncDeps{now: func() time.Time { return now }, jitter: func(time.Duration) time.Duration { return 0 }})
	_, err := syncer.Sync(context.Background(), "test-project")
	require.NoError(t, err)

	require.NotEmpty(t, sentOps)
	for _, ops := range sentOps {
		assert.NotContains(t, ops, string(db.MutationOpReproject),
			"an old API hard-errors on the unknown op, and the mutation is never rejected, so it would resend forever")
		assert.Contains(t, ops, string(db.MutationOpDelete),
			"withholding the optional op must not stop the ops every server understands")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	assert.NotContains(t, store.markedMutationsSynced, reproject.EventID,
		"a withheld mutation was never sent, so it must stay pending rather than be acked")
}

func TestWithholdUnsupportedMutationsKeepsEveryOpTheServerUnderstands(t *testing.T) {
	pending := []db.MutationEnvelope{
		{EventID: "create", Op: db.MutationOpCreate},
		{EventID: "move", Op: db.MutationOpReproject},
		{EventID: "update", Op: db.MutationOpUpdate},
		{EventID: "move-again", Op: db.MutationOpReproject},
		{EventID: "delete", Op: db.MutationOpDelete},
	}

	t.Run("server declares reproject", func(t *testing.T) {
		kept, withheld := withheldUnsupportedMutations(pending, true)
		assert.Equal(t, pending, kept)
		assert.Zero(t, withheld)
	})

	t.Run("server does not declare reproject", func(t *testing.T) {
		kept, withheld := withheldUnsupportedMutations(pending, false)
		assert.Equal(t, 2, withheld)
		require.Len(t, kept, 3)
		for _, mutation := range kept {
			assert.NotEqual(t, db.MutationOpReproject, mutation.Op,
				"a reproject at a server that never declared the op is the wedge this exists to prevent")
		}
		assert.Equal(t, []string{"create", "update", "delete"},
			[]string{kept[0].EventID, kept[1].EventID, kept[2].EventID},
			"withholding one op must not reorder or drop the others")
	})
}
