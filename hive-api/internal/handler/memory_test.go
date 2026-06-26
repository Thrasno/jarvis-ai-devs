package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

// TestCreateMemory_DuplicateSyncID verifica que sync_id duplicado devuelve 200 (idempotente).
// El daemon puede reenviar la misma memoria sin recibir un error — simplemente obtendrá
// el registro existente con HTTP 200 en lugar de 201.
func TestCreateMemory_DuplicateSyncID(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	existing := &model.Memory{
		ID:      "existing-uuid",
		SyncID:  "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Project: "jarvis-dev",
		Title:   "Existing memory",
	}
	memSvc := &mockMemorySvc{}
	memSvc.On("Create", context.Background(), mock.AnythingOfType("*model.Memory")).
		Return(existing, service.ErrSyncIDExists)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodPost, "/memories",
		map[string]interface{}{
			"sync_id":  "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"project":  "jarvis-dev",
			"category": "decision",
			"title":    "Existing memory",
			"content":  "Some content",
		}, "valid-token")

	// 200 OK (no 201) — idempotente, devuelve el existente
	assert.Equal(t, http.StatusOK, w.Code)
	memSvc.AssertExpectations(t)
}

// R2-CRIT-2 — POST /memories must propagate session_id end-to-end. Without this
// (and the lazy-fallback in the service), every direct REST POST after the
// memories.session_id NOT NULL flip fails with an FK violation.

func TestHandlerMemory_Create_WithExplicitSessionID_UsesIt(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	created := &model.Memory{ID: "mem-uuid-explicit"}
	memSvc := &mockMemorySvc{}
	memSvc.On("Create", context.Background(), mock.MatchedBy(func(m *model.Memory) bool {
		return m.SessionID != nil && *m.SessionID == "sess-explicit-1"
	})).Return(created, nil)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodPost, "/memories",
		map[string]interface{}{
			"sync_id":    "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"project":    "jarvis-dev",
			"category":   "decision",
			"title":      "Explicit session",
			"content":    "Some content",
			"session_id": "sess-explicit-1",
		}, "valid-token")

	assert.Equal(t, http.StatusCreated, w.Code)
	memSvc.AssertExpectations(t)
}

func TestHandlerMemory_Create_WithoutSessionID_PropagatesNil(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	created := &model.Memory{ID: "mem-uuid-no-sess"}
	memSvc := &mockMemorySvc{}
	// When session_id is omitted, the handler must NOT fabricate one — the service
	// is responsible for the lazy-fallback. Handler passes through what arrived.
	memSvc.On("Create", context.Background(), mock.MatchedBy(func(m *model.Memory) bool {
		return m.SessionID == nil
	})).Return(created, nil)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodPost, "/memories",
		map[string]interface{}{
			"sync_id":  "b1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"project":  "jarvis-dev",
			"category": "decision",
			"title":    "No session",
			"content":  "Some content",
		}, "valid-token")

	assert.Equal(t, http.StatusCreated, w.Code)
	memSvc.AssertExpectations(t)
}

// R3-FIX-2 — handler must classify ErrSessionProjectMismatch as 400, not 500.
func TestHandlerMemory_Create_SessionMismatch_Returns400(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	memSvc := &mockMemorySvc{}
	memSvc.On("Create", context.Background(), mock.AnythingOfType("*model.Memory")).
		Return(nil, service.ErrSessionProjectMismatch)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodPost, "/memories",
		map[string]interface{}{
			"sync_id":    "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			"project":    "this",
			"category":   "decision",
			"title":      "cross-project",
			"content":    "Some content",
			"session_id": "manual-save-other",
		}, "valid-token")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	memSvc.AssertExpectations(t)
}

// R3-FIX-2 — handler must classify ErrSessionNotFound as 400, not 500.
func TestHandlerMemory_Create_SessionNotFound_Returns400(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	memSvc := &mockMemorySvc{}
	memSvc.On("Create", context.Background(), mock.AnythingOfType("*model.Memory")).
		Return(nil, service.ErrSessionNotFound)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodPost, "/memories",
		map[string]interface{}{
			"sync_id":    "a1b2c3d4-e5f6-7890-abcd-ef1234567891",
			"project":    "this",
			"category":   "decision",
			"title":      "ghost session",
			"content":    "Some content",
			"session_id": "sess-uuid-ghost",
		}, "valid-token")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	memSvc.AssertExpectations(t)
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

	memSvc := &mockMemorySvc{}
	memSvc.On("List", context.Background(), mock.AnythingOfType("model.MemoryFilter")).
		Return([]*model.Memory{}, int64(0), nil)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodGet, "/memories?project=no-match", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	var body model.ListMemoriesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Empty(t, body.Memories)
	assert.Equal(t, int64(0), body.Total)
	memSvc.AssertExpectations(t)
}

func TestListMemories_PassesStructuredDiscoveryFilters(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	memSvc := &mockMemorySvc{}
	wantFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	wantUntil := time.Date(2026, 1, 31, 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)
	memSvc.On("List", context.Background(), mock.MatchedBy(func(filter model.MemoryFilter) bool {
		return filter.Project == "jarvis-dev" && filter.Category != nil && *filter.Category == model.CatDecision &&
			filter.Limit == 10 && filter.Offset == 5 && filter.CreatedFrom.Equal(wantFrom) && filter.CreatedUntil.Equal(wantUntil)
	})).Return([]*model.Memory{{ID: "mem-filtered", Title: "filtered"}}, int64(3), nil)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodGet,
		"/memories?project=jarvis-dev&category=decision&from=2026-01-01&until=2026-01-31&limit=10&offset=5", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	memSvc.AssertExpectations(t)
}

func TestListMemories_InvalidDateRangeReturnsErrorResponse(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	memSvc := &mockMemorySvc{}

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodGet,
		"/memories?from=2026-02-01&until=2026-01-01", nil, "valid-token")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body model.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body.Error, "from must be before until")
	memSvc.AssertNotCalled(t, "List", mock.Anything, mock.Anything)
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
		Return(mems, int64(1), nil)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodGet, "/memories/search?query=test+query", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	memSvc.AssertExpectations(t)
}

func TestSearchMemories_ReturnsPaginationOffsetAndTotal(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	mems := []*model.Memory{{ID: "search-2", Title: "second result"}}
	memSvc := &mockMemorySvc{}
	memSvc.On("Search", context.Background(), "postgres", mock.MatchedBy(func(filter model.MemoryFilter) bool {
		return filter.Project == "jarvis-dev" && filter.Limit == 1 && filter.Offset == 1
	})).Return(mems, int64(2), nil)

	w := doAuthRequest(t, authDeps(authSvc, memSvc), http.MethodGet,
		"/memories/search?query=postgres&project=jarvis-dev&limit=1&offset=1", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	var body model.SearchResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Memories, 1)
	assert.Equal(t, int64(2), body.Total)
	assert.Equal(t, 1, body.Limit)
	assert.Equal(t, 1, body.Offset)
	memSvc.AssertExpectations(t)
}

func TestSearchMemories_MissingQuery(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	w := doAuthRequest(t, authDeps(authSvc, &mockMemorySvc{}), http.MethodGet, "/memories/search", nil, "valid-token")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
