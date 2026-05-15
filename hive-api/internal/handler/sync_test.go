package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
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
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string")).
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
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string")).
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
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string")).
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
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string")).
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
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string")).
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
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string")).
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
