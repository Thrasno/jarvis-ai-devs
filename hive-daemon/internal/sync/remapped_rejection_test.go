package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/stretchr/testify/require"
)

func TestSyncer_Sync_LogsAliasRemappedImmutableRejectionWithSourceProject(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	const rejectedEventID = "remapped-immutable-event"
	const remoteContent = "secret remote immutable content"
	store := &mockSyncStore{
		jwt: "valid-token",
		applyRemoteMutationErrors: map[string]error{
			rejectedEventID: &db.RemoteImmutableCreateRejectedError{
				EventID:          rejectedEventID,
				CanonicalProject: "test-project",
				SourceProject:    "retired-project",
				Topic:            "sdd/change/apply-progress/v2",
				Operation:        db.MutationOpCreate,
				RejectionCode:    db.ImmutableRemoteCreateRejectionProjectRemapped,
				PayloadDigest:    "0123456789abcdef",
			},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveProjectBlockInbox(t, w, r) {
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(syncResponse{
			CompatibilityMode:  compatibilityModeMutationV2,
			NextMutationCursor: &db.MutationCursor{Sequence: 21, EventID: rejectedEventID},
			PulledMutations: []db.MutationEnvelope{
				{EventID: rejectedEventID, EntityType: "memory", EntitySyncID: "remapped-sync", Project: "retired-project", Op: db.MutationOpCreate, Sequence: 21, OccurredAt: now, Memory: &db.MutationMemoryPayload{Content: remoteContent}},
			},
		}))
	}))
	defer server.Close()

	var logs bytes.Buffer
	logger.Log.SetOutput(&logs)
	t.Cleanup(func() { logger.Log.SetOutput(os.Stderr) })
	syncer := newTestSyncer(&Config{APIURL: server.URL, Email: "test@example.com", Password: "password123"}, store, syncDeps{
		now:    func() time.Time { return now },
		jitter: func(time.Duration) time.Duration { return 0 },
	})

	_, err := syncer.Sync(context.Background(), "test-project")

	require.NoError(t, err)
	logged := logs.String()
	require.Contains(t, logged, `rejection_code="project_remapped"`)
	require.Contains(t, logged, `canonical_project="test-project"`)
	require.Contains(t, logged, `source_project="retired-project"`)
	require.NotContains(t, logged, remoteContent)
}
