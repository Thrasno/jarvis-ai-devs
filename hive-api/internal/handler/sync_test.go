package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
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
	// Pull es llamado para obtener memorias del servidor
	syncSvc.On("Pull", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string")).
		Return([]*model.Memory{}, nil)

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
	syncSvc.On("Pull", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string")).
		Return([]*model.Memory{}, nil)

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
	syncSvc.On("Pull", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string")).
		Return([]*model.Memory{}, nil)

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
