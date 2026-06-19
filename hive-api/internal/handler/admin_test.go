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
)

// adminClaims devuelve claims con nivel admin para tests de admin
func adminClaims() *model.Claims {
	return &model.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "admin-uuid-123",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Username: "adminuser",
		Level:    model.LevelAdmin,
	}
}

func adminDeps(authSvc *mockAuthSvc, adminSvc *mockAdminSvc) RouterDeps {
	return RouterDeps{
		AuthSvc:   authSvc,
		MemorySvc: &mockMemorySvc{},
		SyncSvc:   &mockSyncSvc{},
		AdminSvc:  adminSvc,
	}
}

// TestListUsers_Success verifica que un admin obtenga la lista de usuarios
func TestListUsers_Success(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	users := []*model.User{{ID: "1", Username: "user1"}}
	adminSvc := &mockAdminSvc{}
	adminSvc.On("ListUsers", context.Background()).Return(users, nil)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodGet, "/admin/users", nil, "admin-token")

	assert.Equal(t, http.StatusOK, w.Code)
	adminSvc.AssertExpectations(t)
}

// TestListUsers_Forbidden verifica que un no-admin reciba 403
func TestListUsers_Forbidden(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "member-token").Return(testClaims(), nil) // LevelMember

	w := doAuthRequest(t, adminDeps(authSvc, &mockAdminSvc{}), http.MethodGet, "/admin/users", nil, "member-token")

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestSetLevel_Success verifica que un admin pueda cambiar el nivel de un usuario
func TestSetLevel_Success(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	adminSvc.On("SetLevel", context.Background(), model.AdminActor{UserID: "admin-uuid-123", Username: "adminuser"}, "targetuser", model.LevelViewer).Return(nil)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/targetuser/level",
		map[string]string{"level": "viewer"}, "admin-token")

	assert.Equal(t, http.StatusOK, w.Code)
	adminSvc.AssertExpectations(t)
}

// TestSetLevel_NotFound verifica que 404 cuando el usuario no existe
func TestSetLevel_NotFound(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	adminSvc.On("SetLevel", context.Background(), model.AdminActor{UserID: "admin-uuid-123", Username: "adminuser"}, "nobody", mock.AnythingOfType("model.UserLevel")).
		Return(repository.ErrNotFound)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/nobody/level",
		map[string]string{"level": "viewer"}, "admin-token")

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestSetLevel_MaxAdmins verifica que 409 cuando se supera el límite de admins
func TestSetLevel_MaxAdmins(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	adminSvc.On("SetLevel", context.Background(), model.AdminActor{UserID: "admin-uuid-123", Username: "adminuser"}, "newadmin", model.LevelAdmin).
		Return(service.ErrMaxAdminsReached)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/newadmin/level",
		map[string]string{"level": "admin"}, "admin-token")

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestSetLevel_InsufficientAdminsReturnsConflict(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	adminSvc.On("SetLevel", context.Background(), model.AdminActor{UserID: "admin-uuid-123", Username: "adminuser"}, "lastadmin", model.LevelMember).
		Return(service.ErrInsufficientAdmins)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/lastadmin/level",
		map[string]string{"level": "member"}, "admin-token")

	assert.Equal(t, http.StatusConflict, w.Code)
	adminSvc.AssertExpectations(t)
}

// TestDeactivate_Success verifica que un admin pueda desactivar a un usuario
func TestDeactivate_Success(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	adminSvc.On("Deactivate", context.Background(), model.AdminActor{UserID: "admin-uuid-123", Username: "adminuser"}, "targetuser").Return(nil)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/targetuser/deactivate",
		nil, "admin-token")

	assert.Equal(t, http.StatusOK, w.Code)
	adminSvc.AssertExpectations(t)
}

// TestDeactivate_NotFound verifica que 404 cuando el usuario no existe
func TestDeactivate_NotFound(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	adminSvc.On("Deactivate", context.Background(), model.AdminActor{UserID: "admin-uuid-123", Username: "adminuser"}, "nobody").Return(repository.ErrNotFound)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/nobody/deactivate",
		nil, "admin-token")

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeactivate_SelfDeactivationForbidden(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/adminuser/deactivate",
		nil, "admin-token")

	assert.Equal(t, http.StatusForbidden, w.Code)
	adminSvc.AssertNotCalled(t, "Deactivate", mock.Anything, mock.Anything, mock.Anything)
}

func TestDeactivate_InsufficientAdminsReturnsConflict(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	adminSvc.On("Deactivate", context.Background(), model.AdminActor{UserID: "admin-uuid-123", Username: "adminuser"}, "lastadmin").
		Return(service.ErrInsufficientAdmins)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/lastadmin/deactivate",
		nil, "admin-token")

	assert.Equal(t, http.StatusConflict, w.Code)
	adminSvc.AssertExpectations(t)
}

// TestSetLevel_InvalidBody verifica que 400 cuando el body es inválido
func TestSetLevel_InvalidBody(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	w := doAuthRequest(t, adminDeps(authSvc, &mockAdminSvc{}), http.MethodPost, "/admin/users/someone/level",
		map[string]string{}, "admin-token") // falta "level"

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestSetLevel_ServiceError verifica que 500 en errores de servidor desconocidos
func TestSetLevel_ServiceError(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	adminSvc.On("SetLevel", context.Background(), model.AdminActor{UserID: "admin-uuid-123", Username: "adminuser"}, "user1", model.LevelMember).
		Return(errors.New("unexpected db error"))

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/user1/level",
		map[string]string{"level": "member"}, "admin-token")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestSetLevel_SelfChange verifica que un admin no puede cambiar su propio nivel.
func TestSetLevel_SelfChange(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	// "adminuser" es el username de adminClaims() — misma persona intentando cambiar su nivel
	w := doAuthRequest(t, adminDeps(authSvc, &mockAdminSvc{}), http.MethodPost, "/admin/users/adminuser/level",
		map[string]string{"level": "member"}, "admin-token")

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- GrantAdmin handler tests ---

// TestGrantAdmin_Success verifica que un admin pueda ascender a otro usuario.
func TestGrantAdmin_Success(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	adminSvc.On("GrantAdmin", context.Background(), model.AdminActor{UserID: "admin-uuid-123", Username: "adminuser"}, "newguy").Return(nil)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/newguy/grant-admin",
		nil, "admin-token")

	assert.Equal(t, http.StatusOK, w.Code)
	adminSvc.AssertExpectations(t)
}

// TestGrantAdmin_MaxAdmins verifica que 409 cuando se supera el límite de admins.
func TestGrantAdmin_MaxAdmins(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	adminSvc.On("GrantAdmin", context.Background(), model.AdminActor{UserID: "admin-uuid-123", Username: "adminuser"}, "blocked").Return(service.ErrMaxAdminsReached)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/blocked/grant-admin",
		nil, "admin-token")

	assert.Equal(t, http.StatusConflict, w.Code)
}

// TestGrantAdmin_SelfChange verifica que un admin no puede ascenderse a sí mismo.
func TestGrantAdmin_SelfChange(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	w := doAuthRequest(t, adminDeps(authSvc, &mockAdminSvc{}), http.MethodPost, "/admin/users/adminuser/grant-admin",
		nil, "admin-token")

	assert.Equal(t, http.StatusForbidden, w.Code)
}

// --- GetStats handler tests ---

// TestGetStats_Success verifica que un admin obtenga estadísticas del sistema.
func TestGetStats_Success(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	stats := &model.AdminStatsResponse{}
	stats.Users.Total = 5
	stats.Users.Active = 4
	stats.Users.ByLevel = map[string]int{"admin": 1, "member": 4}
	stats.Memories.Total = 42
	stats.Memories.ByProject = []model.ProjectCount{}
	stats.Memories.ByCategory = []model.CategoryCount{}

	adminSvc := &mockAdminSvc{}
	adminSvc.On("GetStats", context.Background()).Return(stats, nil)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodGet, "/admin/stats", nil, "admin-token")

	assert.Equal(t, http.StatusOK, w.Code)
	adminSvc.AssertExpectations(t)
}

// TestGetStats_Forbidden verifica que un no-admin no puede ver estadísticas.
func TestGetStats_Forbidden(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "member-token").Return(testClaims(), nil) // LevelMember

	w := doAuthRequest(t, adminDeps(authSvc, &mockAdminSvc{}), http.MethodGet, "/admin/stats", nil, "member-token")

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestListAuditLogs_ForbiddenForNonAdmin(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "member-token").Return(testClaims(), nil)

	w := doAuthRequest(t, adminDeps(authSvc, &mockAdminSvc{}), http.MethodGet, "/admin/audit-logs", nil, "member-token")

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestListAuditLogs_ForwardsFiltersAndPagination(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	since := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	until := time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)
	project := "jarvis-dev"
	actor := "018f9dd4-7a6c-7ccf-b8e6-4f8a5f3c7b91"
	action := model.AuditActionUserLevelChange
	outcome := model.AuditOutcomeSuccess
	filter := model.AuditFilter{
		Project:     &project,
		ActorUserID: &actor,
		Action:      &action,
		Outcome:     &outcome,
		Since:       &since,
		Until:       &until,
		Limit:       10,
		Offset:      20,
	}

	reason := "level_changed"
	entry := &model.AuditEntry{
		ID:          "audit-1",
		OccurredAt:  since,
		ActorUserID: &actor,
		Project:     &project,
		Action:      action,
		Outcome:     outcome,
		EntryCount:  1,
		ReasonCode:  &reason,
		Metadata:    model.AuditMetadata{"target_username": "targetuser"},
	}
	adminSvc := &mockAdminSvc{}
	adminSvc.On("ListAuditLogs", context.Background(), filter).
		Return(model.NewAuditListResponse([]*model.AuditEntry{entry}, 42, filter), nil)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodGet,
		"/admin/audit-logs?project=jarvis-dev&actor_user_id=018f9dd4-7a6c-7ccf-b8e6-4f8a5f3c7b91&action=user_level_change&outcome=success&since=2026-05-09T10:00:00Z&until=2026-05-10T10:00:00Z&limit=10&offset=20",
		nil, "admin-token")

	assert.Equal(t, http.StatusOK, w.Code)
	adminSvc.AssertExpectations(t)
}

func TestListAuditLogs_DefaultsBoundsAndStableEmptyResponse(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	expectedFilter := model.AuditFilter{Limit: model.DefaultAuditLimit, Offset: 0}
	adminSvc := &mockAdminSvc{}
	adminSvc.On("ListAuditLogs", context.Background(), expectedFilter).
		Return(model.NewAuditListResponse(nil, 0, expectedFilter), nil)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodGet, "/admin/audit-logs?limit=0&offset=-5", nil, "admin-token")

	assert.Equal(t, http.StatusOK, w.Code)
	var body struct {
		AuditLogs []model.AuditEntry `json:"audit_logs"`
		Total     int64              `json:"total"`
		Limit     int                `json:"limit"`
		Offset    int                `json:"offset"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotNil(t, body.AuditLogs)
	assert.Len(t, body.AuditLogs, 0)
	assert.Equal(t, int64(0), body.Total)
	assert.Equal(t, model.DefaultAuditLimit, body.Limit)
	assert.Equal(t, 0, body.Offset)
	adminSvc.AssertExpectations(t)
}

func TestListAuditLogs_RejectsInvalidQueryParams(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{name: "invalid actor user id", path: "/admin/audit-logs?actor_user_id=not-a-uuid"},
		{name: "invalid since", path: "/admin/audit-logs?since=not-a-date"},
		{name: "invalid until", path: "/admin/audit-logs?until=2026-99-99T00:00:00Z"},
		{name: "invalid limit", path: "/admin/audit-logs?limit=abc"},
		{name: "invalid offset", path: "/admin/audit-logs?offset=abc"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			authSvc := &mockAuthSvc{}
			authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

			w := doAuthRequest(t, adminDeps(authSvc, &mockAdminSvc{}), http.MethodGet, tc.path, nil, "admin-token")

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestListAuditLogs_InvalidActorUserIDReturnsStableBadRequest(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodGet, "/admin/audit-logs?actor_user_id=not-a-uuid", nil, "admin-token")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body model.ErrorResponse
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "actor_user_id debe ser un UUID válido", body.Error)
	adminSvc.AssertNotCalled(t, "ListAuditLogs", mock.Anything, mock.Anything)
}
