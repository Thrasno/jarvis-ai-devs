package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-dev/hive-api/internal/service"
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
	adminSvc.On("SetLevel", context.Background(), "targetuser", model.LevelViewer).Return(nil)

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
	adminSvc.On("SetLevel", context.Background(), "nobody", mock.AnythingOfType("model.UserLevel")).
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
	adminSvc.On("SetLevel", context.Background(), "newadmin", model.LevelAdmin).
		Return(service.ErrMaxAdminsReached)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/newadmin/level",
		map[string]string{"level": "admin"}, "admin-token")

	assert.Equal(t, http.StatusConflict, w.Code)
}

// TestDeactivate_Success verifica que un admin pueda desactivar a un usuario
func TestDeactivate_Success(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	adminSvc := &mockAdminSvc{}
	adminSvc.On("Deactivate", context.Background(), "targetuser").Return(nil)

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
	adminSvc.On("Deactivate", context.Background(), "nobody").Return(repository.ErrNotFound)

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/nobody/deactivate",
		nil, "admin-token")

	assert.Equal(t, http.StatusNotFound, w.Code)
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
	adminSvc.On("SetLevel", context.Background(), "user1", model.LevelMember).
		Return(errors.New("unexpected db error"))

	w := doAuthRequest(t, adminDeps(authSvc, adminSvc), http.MethodPost, "/admin/users/user1/level",
		map[string]string{"level": "member"}, "admin-token")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
