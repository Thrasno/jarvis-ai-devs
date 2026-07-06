package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func governanceDeps(authSvc *mockAuthSvc, govSvc *mockProjectGovernanceSvc, syncSvc *mockSyncSvc) RouterDeps {
	return RouterDeps{
		AuthSvc:                  authSvc,
		MemorySvc:                &mockMemorySvc{},
		SyncSvc:                  syncSvc,
		AdminSvc:                 &mockAdminSvc{},
		ProjectGovernanceSvc:     govSvc,
		ProjectBlockAdminEnabled: true,
	}
}

func TestProjectGovernance_BlockProject(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)
	govSvc := &mockProjectGovernanceSvc{}
	req := model.ProjectBlockRequest{Project: "Org/Repo", Action: model.ProjectBlockActionQuarantine, Reason: "duplicate", Confirmation: "org-repo", ExportMarker: "export-1"}
	blockedAt := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)
	govSvc.On("BlockProject", context.Background(), model.AdminActor{UserID: "admin-uuid-123", Username: "adminuser"}, "Org/Repo", req).
		Return(model.ProjectBlockResponse{CommandID: "cmd-1", Project: "Org/Repo", CanonicalProjectKey: "org-repo", BlockedAt: blockedAt}, nil)

	w := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodPost, "/admin/project-blocks/block", req, "admin-token")
	require.Equal(t, http.StatusCreated, w.Code)

	var resp model.ProjectBlockResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "org-repo", resp.CanonicalProjectKey)
	govSvc.AssertExpectations(t)
}

func TestProjectGovernance_AckProjectBlockAllowsAuthenticatedDaemonUser(t *testing.T) {
	authSvc := &mockAuthSvc{}
	claims := testClaims()
	claims.DaemonID = "daemon-1"
	claims.Client = "hive-daemon"
	authSvc.On("ValidateToken", "valid-token").Return(claims, nil)
	govSvc := &mockProjectGovernanceSvc{}
	req := model.ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: model.ProjectBlockAckApplied, AppliedAt: time.Now().UTC()}
	govSvc.On("Acknowledge", context.Background(), mock.MatchedBy(func(got model.ProjectBlockAck) bool {
		return got.CommandID == req.CommandID && got.AckToken == req.AckToken &&
			got.AckSubject == (model.ProjectBlockAckSubject{AuthSubject: "user-uuid-123", DaemonID: "daemon-1", Client: "hive-daemon"})
	})).Return(req, nil)

	w := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodPost, "/admin/project-blocks/ack", req, "valid-token")
	require.Equal(t, http.StatusOK, w.Code)
	govSvc.AssertExpectations(t)
}

func TestProjectGovernance_AckProjectBlockAllowsUserTokenWithoutDaemonClaims(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	govSvc := &mockProjectGovernanceSvc{}
	req := model.ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: model.ProjectBlockAckApplied}
	govSvc.On("Acknowledge", context.Background(), mock.MatchedBy(func(got model.ProjectBlockAck) bool {
		return got.CommandID == req.CommandID && got.AckSubject == (model.ProjectBlockAckSubject{AuthSubject: "user-uuid-123"})
	})).Return(req, nil)

	w := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodPost, "/admin/project-blocks/ack", req, "valid-token")
	require.Equal(t, http.StatusOK, w.Code)
	govSvc.AssertExpectations(t)
}

func TestProjectGovernance_AckProjectBlockAllowsAdmin(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)
	govSvc := &mockProjectGovernanceSvc{}
	req := model.ProjectBlockAck{CommandID: "cmd-1", CanonicalProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: model.ProjectBlockAckApplied, AppliedAt: time.Now().UTC()}
	govSvc.On("Acknowledge", context.Background(), mock.MatchedBy(func(got model.ProjectBlockAck) bool {
		return got.CommandID == req.CommandID && got.AckSubject == (model.ProjectBlockAckSubject{AuthSubject: "admin-uuid-123"})
	})).Return(req, nil)

	w := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodPost, "/admin/project-blocks/ack", req, "admin-token")
	require.Equal(t, http.StatusOK, w.Code)
	govSvc.AssertExpectations(t)
}

func TestProjectGovernance_AckProjectBlockInvalidRequestReturns400(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)
	govSvc := &mockProjectGovernanceSvc{}
	req := model.ProjectBlockAck{CommandID: "forged-cmd", CanonicalProjectKey: "jarvis-dev", AckToken: "forged-token", Status: model.ProjectBlockAckApplied, AppliedAt: time.Now().UTC()}
	admin := adminClaims()
	admin.DaemonID = "daemon-admin"
	admin.Client = "hive-daemon"
	authSvc.ExpectedCalls = nil
	authSvc.On("ValidateToken", "admin-token").Return(admin, nil)
	govSvc.On("Acknowledge", context.Background(), mock.AnythingOfType("model.ProjectBlockAck")).Return(model.ProjectBlockAck{}, service.ErrProjectBlockInvalidRequest)

	w := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodPost, "/admin/project-blocks/ack", req, "admin-token")
	require.Equal(t, http.StatusBadRequest, w.Code)
	govSvc.AssertExpectations(t)
}

func TestProjectGovernance_StatusRequiresAdmin(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	w := doAuthRequest(t, governanceDeps(authSvc, &mockProjectGovernanceSvc{}, &mockSyncSvc{}), http.MethodGet, "/admin/project-blocks/status?project=Org%2FRepo", nil, "valid-token")
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestProjectGovernance_StatusShowsCommandToAdmin(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)
	govSvc := &mockProjectGovernanceSvc{}
	cmd := model.ProjectBlockCommand{CommandID: "cmd-1", AckToken: "ack-token-1", Project: "Org/Repo", CanonicalProjectKey: "org-repo", Reason: "duplicate", BlockedAt: time.Now().UTC()}
	govSvc.On("Status", context.Background(), "Org/Repo").Return(model.ProjectBlockStatusResponse{Project: "Org/Repo", CanonicalProjectKey: "org-repo", Blocked: true, Reason: "duplicate", Command: &cmd}, nil)

	w := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodGet, "/admin/project-blocks/status?project=Org%2FRepo", nil, "admin-token")
	require.Equal(t, http.StatusOK, w.Code)

	var resp model.ProjectBlockStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "duplicate", resp.Reason)
	require.NotNil(t, resp.Command)
}

func TestProjectGovernance_LegacySlashUnsafeBlockRouteIsNotRegistered(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)
	req := model.ProjectBlockRequest{Action: model.ProjectBlockActionQuarantine, Reason: "duplicate", Confirmation: "org-repo", ExportMarker: "export-1"}

	w := doAuthRequest(t, governanceDeps(authSvc, &mockProjectGovernanceSvc{}, &mockSyncSvc{}), http.MethodPost, "/admin/projects/Org%2FRepo/block", req, "admin-token")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestSync_BlockedProjectMapsToHTTP423(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	syncSvc := &mockSyncSvc{}
	cmd := model.ProjectBlockCommand{CommandID: "cmd-1", AckToken: "ack-token-1", Project: "jarvis-dev", CanonicalProjectKey: "jarvis-dev", Reason: "duplicate", BlockedAt: time.Now().UTC()}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").Return(nil, &service.ProjectBlockedError{Command: cmd})

	w := doAuthRequest(t, governanceDeps(authSvc, &mockProjectGovernanceSvc{}, syncSvc), http.MethodPost, "/sync", map[string]any{"project": "jarvis-dev", "memories": []any{}}, "valid-token")
	require.Equal(t, http.StatusLocked, w.Code)

	var resp model.ProjectBlockedErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "cmd-1", resp.Command.CommandID)
}
