package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// claims válidos para tests autenticados
func testClaims() *model.Claims {
	return &model.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-uuid-123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Username: "testuser",
		Level:    model.LevelMember,
	}
}

func authDeps(authSvc *mockAuthSvc, memSvc *mockMemorySvc) RouterDeps {
	return RouterDeps{
		AuthSvc:   authSvc,
		MemorySvc: memSvc,
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  &mockAdminSvc{},
	}
}

// --- Create tests ---

func TestCreateMemory_Success(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	created := &model.Memory{
		ID:      "mem-uuid-1",
		SyncID:  "sync-uuid-1",
		Project: "jarvis-dev",
		Title:   "Test memory",
		Content: "Some content",
	}
	memSvc := &mockMemorySvc{}
	memSvc.On("Create", context.Background(), mock.AnythingOfType("*model.Memory")).
		Return(created, nil)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodPost, "/memories",
		map[string]interface{}{
			"sync_id":  "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"project":  "jarvis-dev",
			"category": "decision",
			"title":    "Test memory",
			"content":  "Some content",
		}, "valid-token")

	assert.Equal(t, http.StatusCreated, w.Code)
	memSvc.AssertExpectations(t)
}

func TestCreateMemory_InvalidBody(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	w := doAuthRequest(t, authDeps(authSvc, &mockMemorySvc{}), http.MethodPost, "/memories",
		map[string]string{}, "valid-token") // body vacío — faltan campos requeridos

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateMemory_ServiceError(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	memSvc := &mockMemorySvc{}
	memSvc.On("Create", context.Background(), mock.AnythingOfType("*model.Memory")).
		Return(nil, errors.New("db error"))

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodPost, "/memories",
		map[string]interface{}{
			"sync_id":  "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"project":  "jarvis-dev",
			"category": "decision",
			"title":    "Test memory",
			"content":  "Some content",
		}, "valid-token")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// --- List tests ---

func TestListMemories_Success(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	mems := []*model.Memory{{ID: "1", Title: "mem1"}}
	memSvc := &mockMemorySvc{}
	memSvc.On("List", context.Background(), mock.AnythingOfType("model.MemoryFilter")).
		Return(mems, int64(1), nil)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodGet, "/memories", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	memSvc.AssertExpectations(t)
}

// --- GetByID tests ---

func TestGetMemoryByID_Success(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	mem := &model.Memory{ID: "mem-uuid-1", Title: "found"}
	memSvc := &mockMemorySvc{}
	memSvc.On("GetByID", context.Background(), "mem-uuid-1").Return(mem, nil)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodGet, "/memories/mem-uuid-1", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	memSvc.AssertExpectations(t)
}

func TestGetMemoryByID_NotFound(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	memSvc := &mockMemorySvc{}
	memSvc.On("GetByID", context.Background(), "nonexistent").Return(nil, repository.ErrNotFound)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodGet, "/memories/nonexistent", nil, "valid-token")

	assert.Equal(t, http.StatusNotFound, w.Code)
	memSvc.AssertExpectations(t)
}

// --- Search tests ---

func TestSearchMemories_Success(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	mems := []*model.Memory{{ID: "1", Title: "found"}}
	memSvc := &mockMemorySvc{}
	memSvc.On("Search", context.Background(), "test query", mock.AnythingOfType("model.MemoryFilter")).
		Return(mems, nil)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodGet, "/memories/search?query=test+query", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	memSvc.AssertExpectations(t)
}

func TestSearchMemories_MissingQuery(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	w := doAuthRequest(t, authDeps(authSvc, &mockMemorySvc{}), http.MethodGet, "/memories/search", nil, "valid-token")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
