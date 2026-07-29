package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func projectDeps(authSvc *mockAuthSvc, projectSvc *mockProjectSvc) RouterDeps {
	return RouterDeps{
		AuthSvc:        authSvc,
		MemorySvc:      &mockMemorySvc{},
		SyncSvc:        &mockSyncSvc{},
		SyncAttemptSvc: &mockSyncAttemptSvc{},
		ProjectSvc:     projectSvc,
		AdminSvc:       &mockAdminSvc{},
	}
}

func TestProjects_RequiresAuthentication(t *testing.T) {
	w := doRequest(t, projectDeps(&mockAuthSvc{}, &mockProjectSvc{}), http.MethodGet, "/projects", nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestProjects_AllowsAuthenticatedNonAdminUser(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	lastActivity := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	expected := model.ProjectListResponse{
		Projects: []model.ProjectSummary{{
			Name:           "jarvis-dev",
			MemoryCount:    7,
			SessionCount:   2,
			LastActivityAt: &lastActivity,
			SyncHealth:     projectHealthPtr(model.ProjectSyncHealthHealthy),
		}},
		Total: 1,
	}
	projectSvc := &mockProjectSvc{}
	projectSvc.On("List", context.Background(), "").Return(expected, nil)

	w := doAuthRequest(t, projectDeps(authSvc, projectSvc), http.MethodGet, "/projects", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	var got model.ProjectListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, expected, got)
	projectSvc.AssertExpectations(t)
}

func TestProjects_ServiceErrorReturns500(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	projectSvc := &mockProjectSvc{}
	projectSvc.On("List", context.Background(), "").Return(model.ProjectListResponse{}, errors.New("database unavailable"))

	w := doAuthRequest(t, projectDeps(authSvc, projectSvc), http.MethodGet, "/projects", nil, "valid-token")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.JSONEq(t, `{"error":"error al listar proyectos"}`, w.Body.String())
	projectSvc.AssertExpectations(t)
}

func TestProjects_DoesNotRequireAdmin(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	projectSvc := &mockProjectSvc{}
	projectSvc.On("List", mock.Anything, "").Return(model.ProjectListResponse{Projects: []model.ProjectSummary{}, Total: 0}, nil)

	w := doAuthRequest(t, projectDeps(authSvc, projectSvc), http.MethodGet, "/projects", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	projectSvc.AssertExpectations(t)
}

func TestProjects_HealthFilterAcceptsOnlyDegraded(t *testing.T) {
	t.Run("passes degraded filter to the service", func(t *testing.T) {
		authSvc := &mockAuthSvc{}
		authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
		projectSvc := &mockProjectSvc{}
		projectSvc.On("List", context.Background(), model.ProjectSyncHealthDegraded).
			Return(model.ProjectListResponse{Projects: []model.ProjectSummary{{Name: "degraded", SyncHealth: projectHealthPtr(model.ProjectSyncHealthDegraded)}}, Total: 1}, nil)

		w := doAuthRequest(t, projectDeps(authSvc, projectSvc), http.MethodGet, "/projects?health=degraded", nil, "valid-token")

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"name":"degraded"`)
		projectSvc.AssertExpectations(t)
	})

	t.Run("rejects unsupported filter without calling the service", func(t *testing.T) {
		authSvc := &mockAuthSvc{}
		authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
		projectSvc := &mockProjectSvc{}

		w := doAuthRequest(t, projectDeps(authSvc, projectSvc), http.MethodGet, "/projects?health=healthy", nil, "valid-token")

		assert.Equal(t, http.StatusBadRequest, w.Code)
		projectSvc.AssertNotCalled(t, "List", mock.Anything, mock.Anything)
	})
}

func projectHealthPtr(value string) *string {
	return &value
}
