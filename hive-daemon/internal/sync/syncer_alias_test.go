package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/stretchr/testify/require"
)

type aliasAwareSyncStore struct {
	*mockSyncStore
	aliases map[string]string
}

func (s *aliasAwareSyncStore) ResolveAlias(_ context.Context, project string) (string, bool, error) {
	target, found := s.aliases[project]
	return target, found, nil
}

func TestSyncerHoldsProjectLifecycleLeaseAcrossNetworkCycle(t *testing.T) {
	store := &aliasAwareSyncStore{mockSyncStore: &mockSyncStore{jwt: "cached-token"}, aliases: map[string]string{}}
	started := make(chan struct{})
	allowResponse := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveProjectBlockInbox(t, w, r) {
			return
		}
		require.Equal(t, "/sync", r.URL.Path)
		close(started)
		<-allowResponse
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{}))
	}))
	defer server.Close()
	syncer := New(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password"}, store)
	done := make(chan error, 1)
	go func() { _, err := syncer.Sync(context.Background(), "source"); done <- err }()
	<-started
	writerDone := make(chan struct{})
	go func() {
		release := db.AcquireProjectLifecycleWrite("source", "target")
		release()
		close(writerDone)
	}()
	select {
	case <-writerDone:
		t.Fatal("promotion writer lease bypassed active sync network cycle")
	case <-time.After(30 * time.Millisecond):
	}
	close(allowResponse)
	require.NoError(t, <-done)
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("promotion writer lease did not resume after sync completed")
	}
}

func TestSyncerResolvesRetiredProjectBeforeSyncEntry(t *testing.T) {
	store := &aliasAwareSyncStore{mockSyncStore: &mockSyncStore{jwt: "cached-token"}, aliases: map[string]string{"retired": "active"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveProjectBlockInbox(t, w, r) {
			return
		}
		require.Equal(t, "/sync", r.URL.Path)
		var request syncRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "active", request.Project)
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{}))
	}))
	defer server.Close()

	syncer := New(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password"}, store)
	result, err := syncer.Sync(context.Background(), "retired")
	require.NoError(t, err)
	require.Equal(t, "active", result.Project)
	require.Equal(t, []string{"active"}, store.recordAttemptCalls)
	require.Equal(t, []string{"active"}, store.recordSuccessCalls)
}
