package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/middleware"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func overviewDeps(authSvc *mockAuthSvc, overviewSvc *mockOverviewSvc) RouterDeps {
	return RouterDeps{
		AuthSvc:     authSvc,
		MemorySvc:   &mockMemorySvc{},
		SyncSvc:     &mockSyncSvc{},
		AdminSvc:    &mockAdminSvc{},
		OverviewSvc: overviewSvc,
	}
}

// --- GET /admin/overview/stats tests ---

func TestOverviewHandler_GetStats_NoAuth_Returns401(t *testing.T) {
	w := doRequest(t, overviewDeps(&mockAuthSvc{}, &mockOverviewSvc{}),
		http.MethodGet, "/admin/overview/stats", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOverviewHandler_GetStats_NonAdmin_Returns403(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "member-token").Return(testClaims(), nil) // LevelMember

	w := doAuthRequest(t, overviewDeps(authSvc, &mockOverviewSvc{}),
		http.MethodGet, "/admin/overview/stats", nil, "member-token")
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestOverviewHandler_GetStats_AdminJWT_Returns200(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	overviewSvc := &mockOverviewSvc{}
	overviewSvc.On("GetStats", context.Background()).Return(&model.OverviewStatsResponse{
		DaemonHealth:        model.OverviewDaemonHealth{Healthy: 2, Total: 5},
		DegradedProjects:    model.OverviewDegradedProjects{Degraded: 1, Total: 2},
		SyncHealthByProject: []model.ProjectSyncHealth{{Project: "project", Status: "degraded", ContributorCount: 2}},
		LiveActivity:        model.OverviewLiveActivity{Count: 3, NewestSyncID: "abc"},
		MostActiveProjects:  []model.ProjectCount{},
	}, nil)

	w := doAuthRequest(t, overviewDeps(authSvc, overviewSvc),
		http.MethodGet, "/admin/overview/stats", nil, "admin-token")

	assert.Equal(t, http.StatusOK, w.Code)
	var resp model.OverviewStatsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.DaemonHealth.Healthy)
	assert.Equal(t, 5, resp.DaemonHealth.Total)
	assert.Equal(t, model.OverviewDegradedProjects{Degraded: 1, Total: 2}, resp.DegradedProjects)
	assert.Contains(t, w.Body.String(), `"status":"degraded"`)
	assert.NotContains(t, w.Body.String(), `"dev_id"`)
	assert.NotContains(t, w.Body.String(), `"daemon_id"`)
	assert.NotContains(t, w.Body.String(), `"device_classification"`)
	overviewSvc.AssertExpectations(t)
}

func TestOverviewHandler_GetStats_ServiceError_Returns500(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	overviewSvc := &mockOverviewSvc{}
	overviewSvc.On("GetStats", context.Background()).Return(nil, errors.New("db error"))

	w := doAuthRequest(t, overviewDeps(authSvc, overviewSvc),
		http.MethodGet, "/admin/overview/stats", nil, "admin-token")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	overviewSvc.AssertExpectations(t)
}

// --- GET /admin/overview/growth tests ---

func TestOverviewHandler_GetGrowth_NoAuth_Returns401(t *testing.T) {
	w := doRequest(t, overviewDeps(&mockAuthSvc{}, &mockOverviewSvc{}),
		http.MethodGet, "/admin/overview/growth", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOverviewHandler_GetGrowth_NonAdmin_Returns403(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "member-token").Return(testClaims(), nil)

	w := doAuthRequest(t, overviewDeps(authSvc, &mockOverviewSvc{}),
		http.MethodGet, "/admin/overview/growth", nil, "member-token")
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestOverviewHandler_GetGrowth_AdminJWT_Returns200(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	overviewSvc := &mockOverviewSvc{}
	overviewSvc.On("GetGrowth", context.Background()).Return(&model.OverviewGrowthResponse{
		KnowledgeGrowth: []model.OverviewChartPoint{
			{Label: "Jan 2026", Value: 10},
		},
	}, nil)

	w := doAuthRequest(t, overviewDeps(authSvc, overviewSvc),
		http.MethodGet, "/admin/overview/growth", nil, "admin-token")

	assert.Equal(t, http.StatusOK, w.Code)
	var resp model.OverviewGrowthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, w.Body.String(), "knowledge_growth")
	overviewSvc.AssertExpectations(t)
}

func TestOverviewHandler_GetGrowth_ServiceError_Returns500(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)

	overviewSvc := &mockOverviewSvc{}
	overviewSvc.On("GetGrowth", context.Background()).Return(nil, errors.New("db error"))

	w := doAuthRequest(t, overviewDeps(authSvc, overviewSvc),
		http.MethodGet, "/admin/overview/growth", nil, "admin-token")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	overviewSvc.AssertExpectations(t)
}

func TestOverviewHandler_Get_NoAuthReturns401(t *testing.T) {
	w := doRequest(t, overviewDeps(&mockAuthSvc{}, &mockOverviewSvc{}), http.MethodGet, "/overview", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOverviewHandler_Get_AuthenticatedCapabilityProjection(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		claims      *model.Claims
		user        *model.User
		userErr     error
		response    *model.CapabilityOverviewResponse
		overviewErr error
		expectCode  int
	}{
		{
			name:   "persisted member receives safe overview",
			token:  "member-token",
			claims: testClaims(),
			user:   &model.User{ID: "user-uuid-123", Level: model.LevelMember, IsActive: true},
			response: &model.CapabilityOverviewResponse{
				Capability: model.OverviewCapabilityMember,
				Summary: model.OverviewSummary{
					MostActiveProjects: []model.ProjectCount{},
				},
			},
			expectCode: http.StatusOK,
		},
		{
			name:   "stale member jwt uses persisted admin level",
			token:  "stale-token",
			claims: testClaims(),
			user:   &model.User{ID: "user-uuid-123", Level: model.LevelAdmin, IsActive: true},
			response: &model.CapabilityOverviewResponse{
				Capability: model.OverviewCapabilityAdmin,
				Summary:    model.OverviewSummary{MostActiveProjects: []model.ProjectCount{}},
				Operations: &model.AdminOverviewOperations{},
			},
			expectCode: http.StatusOK,
		},
		{
			name:       "viewer is forbidden",
			token:      "viewer-token",
			claims:     testClaims(),
			user:       &model.User{ID: "user-uuid-123", Level: model.LevelViewer, IsActive: true},
			expectCode: http.StatusForbidden,
		},
		{
			name:       "inactive user is forbidden",
			token:      "inactive-token",
			claims:     testClaims(),
			user:       &model.User{ID: "user-uuid-123", Level: model.LevelMember, IsActive: false},
			expectCode: http.StatusForbidden,
		},
		{
			name:       "inactive lookup is forbidden",
			token:      "inactive-lookup-token",
			claims:     testClaims(),
			userErr:    service.ErrUserInactive,
			expectCode: http.StatusForbidden,
		},
		{
			name:       "current user failure is generic",
			token:      "failure-token",
			claims:     testClaims(),
			userErr:    errors.New("repository unavailable"),
			expectCode: http.StatusInternalServerError,
		},
		{
			name:        "overview repository failure is generic",
			token:       "overview-failure-token",
			claims:      testClaims(),
			user:        &model.User{ID: "user-uuid-123", Level: model.LevelMember, IsActive: true},
			overviewErr: errors.New("aggregate repository unavailable"),
			expectCode:  http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authSvc := &mockAuthSvc{}
			authSvc.On("ValidateToken", tt.token).Return(tt.claims, nil)
			authSvc.On("GetCurrentUser", context.Background(), tt.claims.Subject).Return(tt.user, tt.userErr)
			overviewSvc := &mockOverviewSvc{}
			if tt.response != nil || tt.overviewErr != nil {
				overviewSvc.On("GetForLevel", context.Background(), tt.user.Level).Return(tt.response, tt.overviewErr)
			}

			w := doAuthRequest(t, overviewDeps(authSvc, overviewSvc), http.MethodGet, "/overview", nil, tt.token)

			assert.Equal(t, tt.expectCode, w.Code)
			if tt.expectCode == http.StatusInternalServerError {
				assert.JSONEq(t, `{"error":"internal server error"}`, w.Body.String())
			}
			authSvc.AssertExpectations(t)
			overviewSvc.AssertExpectations(t)
		})
	}
}

func TestOverviewHandler_Get_ProtectsWireAndLegacyAdminRoutes(t *testing.T) {
	memberAuth := &mockAuthSvc{}
	memberAuth.On("ValidateToken", "member-token").Return(testClaims(), nil)
	memberAuth.On("GetCurrentUser", context.Background(), "user-uuid-123").Return(&model.User{ID: "user-uuid-123", Level: model.LevelMember, IsActive: true}, nil)
	memberOverview := &mockOverviewSvc{}
	memberOverview.On("GetForLevel", context.Background(), model.LevelMember).Return(&model.CapabilityOverviewResponse{
		Capability: model.OverviewCapabilityMember,
		Summary: model.OverviewSummary{
			TotalMemories:      12,
			ActiveProjects:     0,
			LiveActivity:       model.MemberOverviewLiveActivity{Count: 0},
			MostActiveProjects: []model.ProjectCount{},
		},
	}, nil)

	w := doAuthRequest(t, overviewDeps(memberAuth, memberOverview), http.MethodGet, "/overview", nil, "member-token")
	require.Equal(t, http.StatusOK, w.Code)
	var payload any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assertNoOverviewForbiddenKeys(t, payload)
	assert.Contains(t, w.Body.String(), "total_memories")
	assert.Contains(t, w.Body.String(), "most_active_projects")
	memberAuth.AssertExpectations(t)
	memberOverview.AssertExpectations(t)

	legacyAuth := &mockAuthSvc{}
	legacyAuth.On("ValidateToken", "member-token").Return(testClaims(), nil)
	legacy := doAuthRequest(t, overviewDeps(legacyAuth, &mockOverviewSvc{}), http.MethodGet, "/admin/overview/stats", nil, "member-token")
	assert.Equal(t, http.StatusForbidden, legacy.Code)
	legacyAuth.AssertExpectations(t)
}

func TestOverviewHandler_Get_MissingOrInvalidClaimsReturns500(t *testing.T) {
	for _, rawClaims := range []any{nil, "not claims", &model.Claims{}} {
		t.Run("configuration error", func(t *testing.T) {
			handler := NewOverviewHandler(&mockAuthSvc{}, &mockOverviewSvc{})
			r := gin.New()
			r.GET("/overview", func(c *gin.Context) {
				if rawClaims != nil {
					c.Set(middleware.ClaimsKey, rawClaims)
				}
				handler.Get(c)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/overview", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.JSONEq(t, `{"error":"internal server error"}`, w.Body.String())
		})
	}
}

func assertNoOverviewForbiddenKeys(t *testing.T, value any) {
	t.Helper()
	forbidden := map[string]bool{
		"operations": true, "newest_sync_id": true, "daemon_health": true, "conflicts": true,
		"knowledge_growth": true, "growth": true, "sync_health_by_project": true,
		"contributor_count": true, "correlation_id": true, "raw_activity": true,
		"raw_memory": true, "raw_memories": true, "activity": true, "activity_feed": true,
		"entries": true, "actor": true, "timestamp": true, "last_activity_at": true,
	}
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			assert.Falsef(t, forbidden[key], "forbidden overview key %q was serialized", key)
			assertNoOverviewForbiddenKeys(t, child)
		}
	case []any:
		for _, child := range node {
			assertNoOverviewForbiddenKeys(t, child)
		}
	}
}
