package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// The capability STRING is compile-bound: both modules reference
// projectidentity.CapabilityReproject, so a rename breaks the build. The JSON
// FIELD NAMES carrying the same handshake are not bound to anything.
//
// Renaming `json:"from_project"` on model.SyncSessionPayload would leave the
// daemon suite green (it asserts what it SENDS) and the API suite green (nothing
// at this layer parses raw JSON), while every relocation silently no-ops forever
// with both ends reporting a healthy sync. Same for `sync_capabilities`, whose
// absence makes the server withhold every reproject, and for
// `project_identity_version`, whose absence reads as the "omitted is valid for
// API-first rollout" case and quietly disables the contract check.
//
// This test posts the bytes a daemon actually puts on the wire and asserts they
// arrive at the service. It is deliberately one test at one seam, not a
// wire-contract framework.
func TestSyncRequestWireFieldNamesReachTheService(t *testing.T) {
	const rawBody = `{
	  "project": "jarvis-dev",
	  "project_identity_version": "v1",
	  "sync_capabilities": ["mutation.reproject"],
	  "sessions": [{
	    "id": "session-1",
	    "sync_id": "22222222-2222-2222-2222-222222222222",
	    "project": "jarvis-dev",
	    "from_project": "Jarvis.Dev",
	    "dev_id": "dev-1",
	    "client": "claude-code"
	  }],
	  "prompts": [{
	    "sync_id": "33333333-3333-3333-3333-333333333333",
	    "project": "jarvis-dev",
	    "from_project": "Jarvis.Dev",
	    "content": "a prompt"
	  }],
	  "memories": []
	}`

	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	var pushed model.SyncRequest
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.MatchedBy(func(req model.SyncRequest) bool {
		pushed = req
		return true
	}), "user-uuid-123").Return(&model.SyncResponse{Pulled: []*model.Memory{}}, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev",
		mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), mock.AnythingOfType("int"),
		mock.AnythingOfType("model.PullCursor"), mock.AnythingOfType("model.PullCursor")).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		json.RawMessage(rawBody), "valid-token")
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	assert.Equal(t, []string{model.SyncCapabilityReproject}, pushed.SyncCapabilities,
		`"sync_capabilities" must bind — without it the server withholds every reproject and both ends still report a healthy sync`)
	assert.Equal(t, "v1", pushed.ProjectIdentityVersion,
		`"project_identity_version" must bind — an unbound field reads as "omitted", which is valid, so the contract check silently stops running`)
	require.Len(t, pushed.Sessions, 1)
	assert.Equal(t, "Jarvis.Dev", pushed.Sessions[0].FromProject,
		`"sessions[].from_project" must bind — it is the precondition for the ONE column a re-push may move`)
	require.Len(t, pushed.Prompts, 1)
	assert.Equal(t, "Jarvis.Dev", pushed.Prompts[0].FromProject,
		`"prompts[].from_project" carries the same precondition on the prompt path`)
}
