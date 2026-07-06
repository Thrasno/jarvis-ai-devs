package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func syncDeps(authSvc *mockAuthSvc, syncSvc *mockSyncSvc) RouterDeps {
	return RouterDeps{
		AuthSvc:   authSvc,
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   syncSvc,
		AdminSvc:  &mockAdminSvc{},
	}
}

func TestSync_Success(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncResp := &model.SyncResponse{
		Pushed:    2,
		Pulled:    []*model.Memory{{ID: "pulled-1"}},
		Conflicts: 0,
	}
	syncSvc := &mockSyncSvc{}
	// Push es llamado con el request y el userID del token
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(syncResp, nil)
	// PullAll es llamado para obtener sesiones + memorias del servidor
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), mock.AnythingOfType("int"), mock.AnythingOfType("model.PullCursor"), mock.AnythingOfType("model.PullCursor")).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":   "jarvis-dev",
			"memories":  []interface{}{},
			"last_sync": time.Now().Add(-time.Hour).Format(time.RFC3339),
		}, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	syncSvc.AssertExpectations(t)
}

func TestSync_InvalidBody(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	w := doAuthRequest(t, syncDeps(authSvc, &mockSyncSvc{}), http.MethodPost, "/sync",
		map[string]string{}, "valid-token") // falta "project" requerido

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSync_LegacyMemoryMetadataFieldsDoNotBreakBinding(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncResp := &model.SyncResponse{Pushed: 1, Pulled: []*model.Memory{}, Conflicts: 0}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.MatchedBy(func(req model.SyncRequest) bool {
		return len(req.Memories) == 1 && req.Memories[0].SyncID == "11111111-1111-1111-1111-111111111111"
	}), "user-uuid-123").Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), mock.AnythingOfType("int"), mock.AnythingOfType("model.PullCursor"), mock.AnythingOfType("model.PullCursor")).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project": "jarvis-dev",
			"memories": []interface{}{
				map[string]interface{}{
					"sync_id":        "11111111-1111-1111-1111-111111111111",
					"project":        "jarvis-dev",
					"category":       "decision",
					"title":          "Legacy metadata",
					"content":        "Compatibility payload",
					"created_by":     "daemon-user",
					"confidence":     "high",
					"impact_score":   7,
					"created_at":     time.Now().UTC().Format(time.RFC3339),
					"updated_at":     time.Now().UTC().Format(time.RFC3339),
					"tags":           []interface{}{},
					"files_affected": []interface{}{},
				},
			},
		}, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	syncSvc.AssertExpectations(t)
}

func TestSync_LegacyMutationMetadataFieldsDoNotBreakBinding(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncResp := &model.SyncResponse{Pushed: 1, Pulled: []*model.Memory{}, Conflicts: 0}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.MatchedBy(func(req model.SyncRequest) bool {
		return len(req.Mutations) == 1 && req.Mutations[0].EntitySyncID == "22222222-2222-2222-2222-222222222222"
	}), "user-uuid-123").Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), mock.AnythingOfType("int"), mock.AnythingOfType("model.PullCursor"), mock.AnythingOfType("model.PullCursor")).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":          "jarvis-dev",
			"protocol_version": model.MutationProtocolVersion,
			"memories":         []interface{}{},
			"mutations": []interface{}{
				map[string]interface{}{
					"event_id":       "evt-legacy-metadata",
					"entity_type":    model.MutationEntityMemory,
					"entity_sync_id": "22222222-2222-2222-2222-222222222222",
					"project":        "jarvis-dev",
					"op":             string(model.MutationOpCreate),
					"sequence":       1,
					"occurred_at":    time.Now().UTC().Format(time.RFC3339),
					"memory": map[string]interface{}{
						"sync_id":        "22222222-2222-2222-2222-222222222222",
						"project":        "jarvis-dev",
						"category":       "decision",
						"title":          "Legacy mutation metadata",
						"content":        "Compatibility payload",
						"created_by":     "daemon-user",
						"confidence":     "high",
						"impact_score":   7,
						"created_at":     time.Now().UTC().Format(time.RFC3339),
						"updated_at":     time.Now().UTC().Format(time.RFC3339),
						"tags":           []interface{}{},
						"files_affected": []interface{}{},
					},
				},
			},
		}, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	syncSvc.AssertExpectations(t)
}

func TestSync_ServiceError(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(nil, errors.New("db error"))

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":  "jarvis-dev",
			"memories": []interface{}{},
		}, "valid-token")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSync_ProjectBlockedResponseRedactsReason(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncSvc := &mockSyncSvc{}
	syncSvc.On("Sync", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").Return(nil, &service.ProjectBlockedError{Command: model.ProjectBlockCommand{
		CommandID: "cmd-1", AckToken: "ack-token-1", Project: "Jarvis Dev", CanonicalProjectKey: "jarvis-dev", Reason: "sensitive admin reason", BlockedAt: time.Now().UTC(),
	}})

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{"project": "jarvis-dev", "memories": []interface{}{}}, "valid-token")

	require.Equal(t, http.StatusLocked, w.Code)
	var body model.ProjectBlockedErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "cmd-1", body.Command.CommandID)
	require.Equal(t, "ack-token-1", body.Command.AckToken)
	require.Equal(t, "jarvis-dev", body.Command.CanonicalProjectKey)
	require.Empty(t, body.Command.Reason)
}

func TestSync_ProjectBlockedResponseFromServicePrecheckPreservesAckToken(t *testing.T) {
	authSvc := &mockAuthSvc{}
	claims := testClaims()
	claims.DaemonID = "daemon-1"
	claims.Client = "hive-daemon"
	authSvc.On("ValidateToken", "valid-token").Return(claims, nil)

	ctx := context.Background()
	txMemRepo := &repository.MockMemoryRepository{}
	txPromptRepo := &repository.MockPromptRepository{}
	txSessionRepo := &repository.MockSessionRepository{}
	txBlockRepo := &repository.MockProjectBlockRepository{}
	txAuditRepo := &repository.MockAuditRepository{}
	txLocks := &repository.MockProjectKeyLockRepository{}
	tx := repository.NewMockTxManager(nil, txAuditRepo)
	tx.Memory = txMemRepo
	tx.Prompt = txPromptRepo
	tx.Session = txSessionRepo
	tx.ProjectBlocks = txBlockRepo
	tx.ProjectKeyLocks = txLocks
	syncSvc := service.NewSyncService(&repository.MockMemoryRepository{}, &repository.MockPromptRepository{}, &repository.MockSessionRepository{}, nil, &repository.MockProjectBlockRepository{}, tx)
	blockedAt := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	subject := model.ProjectBlockAckSubject{AuthSubject: "user-uuid-123", DaemonID: "daemon-1", Client: "hive-daemon"}
	block := &model.ProjectBlock{CommandID: "cmd-real-precheck", AckToken: "legacy-global-token", Project: "Jarvis Dev", CanonicalProjectKey: "jarvis-dev", Reason: "sensitive admin reason", BlockedAt: blockedAt}

	txLocks.On("LockCanonicalProjectKeys", ctx, []string{"jarvis-dev"}).Return(nil).Once()
	txBlockRepo.On("GetByCanonicalKey", ctx, "jarvis-dev").Return(block, nil).Once()
	txBlockRepo.On("EnsureAckDelivery", ctx, block, subject).Return(model.ProjectBlockCommand{CommandID: "cmd-real-precheck", AckToken: "ack-delivery-subject", Project: "Jarvis Dev", CanonicalProjectKey: "jarvis-dev", Reason: "sensitive admin reason", BlockedAt: blockedAt}, nil).Once()

	w := doAuthRequest(t, RouterDeps{AuthSvc: authSvc, MemorySvc: &mockMemorySvc{}, SyncSvc: syncSvc, AdminSvc: &mockAdminSvc{}}, http.MethodPost, "/sync",
		map[string]interface{}{"project": "Jarvis Dev", "memories": []interface{}{}}, "valid-token")

	require.Equal(t, http.StatusLocked, w.Code)
	var body model.ProjectBlockedErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "cmd-real-precheck", body.Command.CommandID)
	require.Equal(t, "ack-delivery-subject", body.Command.AckToken)
	require.Equal(t, "jarvis-dev", body.Command.CanonicalProjectKey)
	require.Empty(t, body.Command.Reason)
	require.True(t, tx.RolledBack)
	txMemRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
	txSessionRepo.AssertNotCalled(t, "UpsertSession", mock.Anything, mock.Anything)
	txPromptRepo.AssertNotCalled(t, "Upsert", mock.Anything, mock.Anything)
}

func TestSync_ProjectKeyLockContentionReturns503(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncSvc := &mockSyncSvc{}
	syncSvc.On("Sync", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").Return(nil, service.ErrProjectKeyLockBusy)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{"project": "jarvis-dev", "memories": []interface{}{}}, "valid-token")

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// TestSync_WithPrompts verifica que prompts_pushed aparece en la respuesta
// cuando el cliente envía prompts (S11).
func TestSync_WithPrompts(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncResp := &model.SyncResponse{
		Pushed:        0,
		Pulled:        []*model.Memory{},
		Conflicts:     0,
		PromptsPushed: 2,
	}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), mock.AnythingOfType("int"), mock.AnythingOfType("model.PullCursor"), mock.AnythingOfType("model.PullCursor")).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":  "jarvis-dev",
			"memories": []interface{}{},
			"prompts": []interface{}{
				map[string]interface{}{
					"sync_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
					"project": "jarvis-dev",
					"content": "Be concise",
				},
				map[string]interface{}{
					"sync_id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
					"project": "jarvis-dev",
					"content": "Use examples",
				},
			},
			"last_sync": time.Now().Add(-time.Hour).Format(time.RFC3339),
		}, "valid-token")

	require.Equal(t, http.StatusOK, w.Code)

	var resp model.SyncResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.PromptsPushed)
	syncSvc.AssertExpectations(t)
}

func TestSync_MutationProtocolV2ResponseIncludesCursorAndMutationFields(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	occurredAt := time.Date(2026, 5, 11, 14, 30, 0, 0, time.UTC)
	syncResp := &model.SyncResponse{
		Pushed:        1,
		Pulled:        []*model.Memory{},
		Conflicts:     0,
		PromptsPushed: 0,
		NextMutationCursor: &model.MutationCursor{
			Sequence: 42,
			EventID:  "evt-next-42",
		},
		PulledMutations: []model.MutationEnvelope{
			{
				EventID:      "evt-41",
				EntityType:   model.MutationEntityMemory,
				EntitySyncID: "mem-sync-41",
				Project:      "jarvis-dev",
				Op:           model.MutationOpDelete,
				Sequence:     41,
				OccurredAt:   occurredAt,
			},
		},
		CompatibilityMode: model.CompatibilityModeMutationV2,
	}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.MatchedBy(func(req model.SyncRequest) bool {
		return req.ProtocolVersion == model.MutationProtocolVersion &&
			req.MutationCursor != nil &&
			req.MutationCursor.Sequence == 40 &&
			req.MutationCursor.EventID == "evt-40"
	}), "user-uuid-123").Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), mock.AnythingOfType("int"), mock.AnythingOfType("model.PullCursor"), mock.AnythingOfType("model.PullCursor")).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":          "jarvis-dev",
			"protocol_version": model.MutationProtocolVersion,
			"mutation_cursor": map[string]interface{}{
				"sequence": float64(40),
				"event_id": "evt-40",
			},
			"memories": []interface{}{},
		}, "valid-token")

	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, model.CompatibilityModeMutationV2, body["compatibility_mode"])

	nextCursor, ok := body["next_mutation_cursor"].(map[string]interface{})
	require.True(t, ok, "next_mutation_cursor must be a JSON object")
	assert.Equal(t, float64(42), nextCursor["sequence"])
	assert.Equal(t, "evt-next-42", nextCursor["event_id"])

	mutations, ok := body["pulled_mutations"].([]interface{})
	require.True(t, ok, "pulled_mutations must use the stable JSON field name")
	require.Len(t, mutations, 1)

	mutation, ok := mutations[0].(map[string]interface{})
	require.True(t, ok, "pulled_mutations[0] must be a JSON object")
	assert.Equal(t, "evt-41", mutation["event_id"])
	assert.Equal(t, "mem-sync-41", mutation["entity_sync_id"])
	assert.Equal(t, model.MutationEntityMemory, mutation["entity_type"])
	assert.Equal(t, string(model.MutationOpDelete), mutation["op"])
	assert.Equal(t, occurredAt.Format(time.RFC3339Nano), mutation["occurred_at"])
	syncSvc.AssertExpectations(t)
}

func TestSync_LegacyResponseOmitsAbsentMutationProtocolV2Fields(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncResp := &model.SyncResponse{
		Pushed:    1,
		Pulled:    []*model.Memory{},
		Conflicts: 0,
	}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.MatchedBy(func(req model.SyncRequest) bool {
		return req.ProtocolVersion == 0 && req.MutationCursor == nil && len(req.Mutations) == 0
	}), "user-uuid-123").Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), mock.AnythingOfType("int"), mock.AnythingOfType("model.PullCursor"), mock.AnythingOfType("model.PullCursor")).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":  "jarvis-dev",
			"memories": []interface{}{},
		}, "valid-token")

	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotContains(t, body, "next_mutation_cursor")
	assert.NotContains(t, body, "pulled_mutations")
	assert.NotContains(t, body, "compatibility_mode")
	syncSvc.AssertExpectations(t)
}

// ─── T4.10 SC-15: pulled_sessions present in sync response ───────────────────

// TestSyncHandler_Pull_IncludesSessions verifies SC-15: when the server has sessions
// newer than last_sync, the response body contains pulled_sessions[] non-empty.
// This is the spec contract assertion for FR-S-5.
func TestSyncHandler_Pull_IncludesSessions(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	pullResult := &model.PullResult{
		Sessions: []*model.Session{
			{ID: "sess-server-1", Project: "jarvis-dev", DevID: "dev", Client: "claude-code"},
		},
		Memories: []*model.Memory{
			{ID: "mem-srv-1", SyncID: "aaaa0000-0000-0000-0000-000000000001"},
		},
	}
	syncResp := &model.SyncResponse{Pushed: 0, Pulled: []*model.Memory{}}

	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), mock.AnythingOfType("int"), mock.AnythingOfType("model.PullCursor"), mock.AnythingOfType("model.PullCursor")).
		Return(pullResult, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":   "jarvis-dev",
			"memories":  []interface{}{},
			"last_sync": time.Now().Add(-time.Hour).Format(time.RFC3339),
		}, "valid-token")

	require.Equal(t, http.StatusOK, w.Code)

	var resp model.SyncResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.PulledSessions, 1, "pulled_sessions must be populated from server sessions")
	assert.Equal(t, "sess-server-1", resp.PulledSessions[0].ID)
	require.Len(t, resp.Pulled, 1)
	assert.Equal(t, "aaaa0000-0000-0000-0000-000000000001", resp.Pulled[0].SyncID)
	syncSvc.AssertExpectations(t)
}

// R2-CRIT-6 — Push validation errors must map to 4xx, not 500. Project mismatch
// and unknown-session are user input errors (the daemon needs to fix its payload),
// not server faults.

func TestHandlerSync_Push_ProjectMismatch_Returns400(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(nil, service.ErrSessionProjectMismatch)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":  "this",
			"memories": []interface{}{},
		}, "valid-token")

	assert.Equal(t, http.StatusBadRequest, w.Code, "project mismatch must be 400, not 500")
}

func TestHandlerSync_Push_UnknownSession_Returns400(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(nil, service.ErrSessionNotFound)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":  "jarvis-dev",
			"memories": []interface{}{},
		}, "valid-token")

	assert.Equal(t, http.StatusBadRequest, w.Code, "unknown session_id must be 400, not 500")
}

// ─── PR 2a: bounded legacy pull pagination — handler-level pull_limit clamp ──

// TestSync_PullLimitAbsentMeansUnboundedPull verifies that when pull_limit is
// absent from the request, the handler forwards limit=model.UnboundedPullLimit
// (0) to SyncService.PullAll — meaning "no LIMIT clause, single unbounded
// page". This is the CRITICAL backward-compat contract for PR 2a: the current
// hive-daemon has no pulled_has_more/next_pull_cursor handling and hardcodes
// PullHasMore=false, so if the server silently capped the page at 100 for
// clients that never opted in, any channel with >100 pending rows would strand
// the remainder until PR 2b ships. Old daemons that never send pull_limit MUST
// keep getting exactly today's behavior: unbounded pull, has_more=false.
func TestSync_PullLimitAbsentMeansUnboundedPull(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncResp := &model.SyncResponse{Pushed: 0, Pulled: []*model.Memory{}}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), model.UnboundedPullLimit, model.PullCursor{}, model.PullCursor{}).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":  "jarvis-dev",
			"memories": []interface{}{},
		}, "valid-token")

	require.Equal(t, http.StatusOK, w.Code)
	syncSvc.AssertExpectations(t)
}

// TestSync_PullLimitZeroMeansUnboundedPull verifies an explicit pull_limit: 0
// is treated the same as absent — unbounded pull, not a bounded page of 0 or
// the old 100 default. Zero is indistinguishable from "omitted" on the wire
// (Go zero value for int), so both must mean "did not opt in".
func TestSync_PullLimitZeroMeansUnboundedPull(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncResp := &model.SyncResponse{Pushed: 0, Pulled: []*model.Memory{}}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), model.UnboundedPullLimit, model.PullCursor{}, model.PullCursor{}).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":    "jarvis-dev",
			"memories":   []interface{}{},
			"pull_limit": 0,
		}, "valid-token")

	require.Equal(t, http.StatusOK, w.Code)
	syncSvc.AssertExpectations(t)
}

// TestSync_PullLimitClampedToMax verifies a pull_limit above MaxPullLimit is
// clamped server-side, never trusted raw from the client.
func TestSync_PullLimitClampedToMax(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncResp := &model.SyncResponse{Pushed: 0, Pulled: []*model.Memory{}}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), 100, model.PullCursor{}, model.PullCursor{}).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":    "jarvis-dev",
			"memories":   []interface{}{},
			"pull_limit": 5000,
		}, "valid-token")

	require.Equal(t, http.StatusOK, w.Code)
	syncSvc.AssertExpectations(t)
}

// TestSync_PullLimitWithinRangeForwardedAsIs verifies a valid pull_limit passes
// through unchanged.
func TestSync_PullLimitWithinRangeForwardedAsIs(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncResp := &model.SyncResponse{Pushed: 0, Pulled: []*model.Memory{}}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), 17, model.PullCursor{}, model.PullCursor{}).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":    "jarvis-dev",
			"memories":   []interface{}{},
			"pull_limit": 17,
		}, "valid-token")

	require.Equal(t, http.StatusOK, w.Code)
	syncSvc.AssertExpectations(t)
}

// TestSync_PullCursorsForwardedToPullAll verifies pull_cursor and
// pull_session_cursor from the request body are forwarded as the
// memories/sessions cursor arguments respectively (independent channels).
func TestSync_PullCursorsForwardedToPullAll(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncResp := &model.SyncResponse{Pushed: 0, Pulled: []*model.Memory{}}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(syncResp, nil)

	expectedMemCursor := model.PullCursor{SyncedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), SyncID: "mem-cursor-x"}
	expectedSessCursor := model.PullCursor{SyncedAt: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), SyncID: "sess-cursor-x"}

	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), model.UnboundedPullLimit, expectedMemCursor, expectedSessCursor).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":  "jarvis-dev",
			"memories": []interface{}{},
			"pull_cursor": map[string]interface{}{
				"synced_at": expectedMemCursor.SyncedAt.Format(time.RFC3339),
				"sync_id":   expectedMemCursor.SyncID,
			},
			"pull_session_cursor": map[string]interface{}{
				"synced_at": expectedSessCursor.SyncedAt.Format(time.RFC3339),
				"sync_id":   expectedSessCursor.SyncID,
			},
		}, "valid-token")

	require.Equal(t, http.StatusOK, w.Code)
	syncSvc.AssertExpectations(t)
}

// TestSync_ResponseIncludesPullPaginationFields verifies the response wire
// fields (pulled_has_more, next_pull_cursor, pulled_sessions_has_more,
// next_session_cursor) round-trip from PullResult into the JSON body.
func TestSync_ResponseIncludesPullPaginationFields(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	nextMemCursor := &model.PullCursor{SyncedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), SyncID: "next-mem"}
	nextSessCursor := &model.PullCursor{SyncedAt: time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), SyncID: "next-sess"}

	syncResp := &model.SyncResponse{Pushed: 0, Pulled: []*model.Memory{}}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), model.UnboundedPullLimit, model.PullCursor{}, model.PullCursor{}).
		Return(&model.PullResult{
			Sessions:          []*model.Session{},
			Memories:          []*model.Memory{},
			MemoriesHasMore:   true,
			NextPullCursor:    nextMemCursor,
			SessionsHasMore:   true,
			NextSessionCursor: nextSessCursor,
		}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":  "jarvis-dev",
			"memories": []interface{}{},
		}, "valid-token")

	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["pulled_has_more"])
	assert.Equal(t, true, body["pulled_sessions_has_more"])

	nextPull, ok := body["next_pull_cursor"].(map[string]interface{})
	require.True(t, ok, "next_pull_cursor must be a JSON object")
	assert.Equal(t, "next-mem", nextPull["sync_id"])

	nextSess, ok := body["next_session_cursor"].(map[string]interface{})
	require.True(t, ok, "next_session_cursor must be a JSON object")
	assert.Equal(t, "next-sess", nextSess["sync_id"])
}

// TestSync_ResponseOmitsPullPaginationFieldsWhenFullyDrained verifies backward
// compat: when the pull fits in one page (the common/legacy case), the new
// fields are entirely absent from the JSON body — an old daemon parsing this
// response sees exactly the same shape as before PR 2a.
func TestSync_ResponseOmitsPullPaginationFieldsWhenFullyDrained(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	syncResp := &model.SyncResponse{Pushed: 0, Pulled: []*model.Memory{}}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), model.UnboundedPullLimit, model.PullCursor{}, model.PullCursor{}).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":  "jarvis-dev",
			"memories": []interface{}{},
		}, "valid-token")

	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotContains(t, body, "pulled_has_more")
	assert.NotContains(t, body, "next_pull_cursor")
	assert.NotContains(t, body, "pulled_sessions_has_more")
	assert.NotContains(t, body, "next_session_cursor")
}

// ─── PR 2a: field-identifying 400 for oversized arrays ───────────────────────

// TestSync_OversizedMemoriesArray_Returns400WithFieldName verifies that when
// memories[] exceeds binding:"max=100", the 400 error body names the offending
// field ("Memories" / "memories") instead of a generic validator message —
// gin's validator.FieldError carries this, we just need to surface it.
func TestSync_OversizedMemoriesArray_Returns400WithFieldName(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	oversizedMemories := make([]interface{}, 101)
	for i := range oversizedMemories {
		oversizedMemories[i] = map[string]interface{}{
			"sync_id":    "11111111-1111-1111-1111-111111111111",
			"project":    "jarvis-dev",
			"category":   "decision",
			"title":      "t",
			"content":    "c",
			"created_by": "daemon-user",
		}
	}

	w := doAuthRequest(t, syncDeps(authSvc, &mockSyncSvc{}), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":  "jarvis-dev",
			"memories": oversizedMemories,
		}, "valid-token")

	require.Equal(t, http.StatusBadRequest, w.Code)

	var body model.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, strings.ToLower(body.Error), "memories", "400 body must identify the offending field")
}

// TestSync_OversizedSessionsArray_Returns400WithFieldName mirrors the memories
// case for the sessions[] field.
func TestSync_OversizedSessionsArray_Returns400WithFieldName(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	oversizedSessions := make([]interface{}, 101)
	for i := range oversizedSessions {
		oversizedSessions[i] = map[string]interface{}{
			"id":      fmt.Sprintf("sess-%d", i),
			"sync_id": fmt.Sprintf("22222222-2222-2222-2222-%012d", i),
			"project": "jarvis-dev",
			"dev_id":  "dev",
			"client":  "claude-code",
		}
	}

	w := doAuthRequest(t, syncDeps(authSvc, &mockSyncSvc{}), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":  "jarvis-dev",
			"memories": []interface{}{},
			"sessions": oversizedSessions,
		}, "valid-token")

	require.Equal(t, http.StatusBadRequest, w.Code)

	var body model.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, strings.ToLower(body.Error), "sessions", "400 body must identify the offending field")
}

// TestSync_NoPrompts verifica S9 (backward-compat): un cliente antiguo que no envía
// el campo prompts recibe prompts_pushed=0 en la respuesta.
func TestSync_NoPrompts(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	// Mock devuelve PromptsPushed=0 — daemon antiguo, sin prompts
	syncResp := &model.SyncResponse{
		Pushed:        1,
		Pulled:        []*model.Memory{},
		Conflicts:     0,
		PromptsPushed: 0,
	}
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").
		Return(syncResp, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), mock.AnythingOfType("int"), mock.AnythingOfType("model.PullCursor"), mock.AnythingOfType("model.PullCursor")).
		Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	// Daemon antiguo: no incluye el campo "prompts" en el body
	w := doAuthRequest(t, syncDeps(authSvc, syncSvc), http.MethodPost, "/sync",
		map[string]interface{}{
			"project":   "jarvis-dev",
			"memories":  []interface{}{},
			"last_sync": time.Now().Add(-time.Hour).Format(time.RFC3339),
		}, "valid-token")

	require.Equal(t, http.StatusOK, w.Code)

	var resp model.SyncResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.PromptsPushed)
	syncSvc.AssertExpectations(t)
}
