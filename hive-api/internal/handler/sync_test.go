package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
