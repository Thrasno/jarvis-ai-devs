package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
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
		Conflicts:           model.OverviewConflicts{Open: 1},
		SyncHealthByProject: []model.ProjectSyncHealth{},
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
	assert.Equal(t, 1, resp.Conflicts.Open)
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
