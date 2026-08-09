package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	req := model.ProjectBlockRequest{Project: "Org/Repo", Action: model.ProjectBlockActionBlock, Reason: "duplicate", Confirmation: "org-repo", ExportMarker: "export-1"}
	blockedAt := time.Date(2026, 7, 5, 20, 0, 0, 0, time.UTC)
	govSvc.On("BlockProject", context.Background(), model.AdminActor{UserID: "admin-uuid-123", Username: "adminuser"}, "Org/Repo", req).
		Return(model.ProjectBlockResponse{CommandID: "cmd-1", Project: "Org/Repo", ProjectKey: "org-repo", BlockedAt: blockedAt}, nil)

	w := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodPost, "/admin/project-blocks/block", req, "admin-token")
	require.Equal(t, http.StatusCreated, w.Code)

	var resp model.ProjectBlockResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "org-repo", resp.ProjectKey)
	govSvc.AssertExpectations(t)
}

func TestProjectGovernance_AckProjectBlockAllowsAuthenticatedDaemonUser(t *testing.T) {
	authSvc := &mockAuthSvc{}
	claims := testClaims()
	claims.DaemonID = "daemon-1"
	claims.Client = "hive-daemon"
	authSvc.On("ValidateToken", "valid-token").Return(claims, nil)
	govSvc := &mockProjectGovernanceSvc{}
	req := model.ProjectBlockAck{CommandID: "cmd-1", ProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: model.ProjectBlockAckApplied, AppliedAt: time.Now().UTC()}
	govSvc.On("Acknowledge", context.Background(), mock.MatchedBy(func(got model.ProjectBlockAck) bool {
		return got.CommandID == req.CommandID && got.AckToken == req.AckToken &&
			got.AckSubject == (model.ProjectBlockAckSubject{AuthSubject: "user-uuid-123", DaemonID: "daemon-1", Client: "hive-daemon"})
	})).Return(req, nil)

	w := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodPost, "/admin/project-blocks/ack", req, "valid-token")
	require.Equal(t, http.StatusOK, w.Code)
	govSvc.AssertExpectations(t)
}

func TestProjectGovernance_InboxDeliversOnlyAuthenticatedAccountCommands(t *testing.T) {
	authSvc := &mockAuthSvc{}
	claims := testClaims()
	claims.DaemonID = "daemon-1"
	claims.Client = "hive-daemon"
	authSvc.On("ValidateToken", "valid-token").Return(claims, nil)
	govSvc := &mockProjectGovernanceSvc{}
	govSvc.On("Inbox", context.Background(), model.ProjectBlockAckSubject{AuthSubject: "user-uuid-123", DaemonID: "daemon-1", Client: "hive-daemon"}).
		Return([]model.ProjectBlockCommand{{CommandID: "cmd-2", AckToken: "account-token", Project: "alpha", ProjectKey: "alpha", Action: model.ProjectBlockActionUnblock, Generation: 2}}, nil)

	w := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodGet, "/project-blocks/inbox", nil, "valid-token")
	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Commands []model.ProjectBlockCommand `json:"commands"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Commands, 1)
	require.Equal(t, "account-token", body.Commands[0].AckToken)
	govSvc.AssertExpectations(t)
}

func TestProjectGovernance_InboxRejectsUnauthenticatedAndAccountlessCallersWithoutDisclosure(t *testing.T) {
	t.Run("missing authentication", func(t *testing.T) {
		router := NewRouter(governanceDeps(&mockAuthSvc{}, &mockProjectGovernanceSvc{}, &mockSyncSvc{}))
		request := httptest.NewRequest(http.MethodGet, "/project-blocks/inbox", nil)
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		require.Equal(t, http.StatusUnauthorized, response.Code)
		require.NotContains(t, response.Body.String(), "commands")
		require.NotContains(t, response.Body.String(), "project")
	})

	t.Run("authenticated caller without account subject", func(t *testing.T) {
		authSvc := &mockAuthSvc{}
		claims := testClaims()
		claims.Subject = ""
		authSvc.On("ValidateToken", "accountless-token").Return(claims, nil)
		govSvc := &mockProjectGovernanceSvc{}

		response := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodGet, "/project-blocks/inbox", nil, "accountless-token")

		require.Equal(t, http.StatusForbidden, response.Code)
		require.NotContains(t, response.Body.String(), "commands")
		require.NotContains(t, response.Body.String(), "project")
		govSvc.AssertNotCalled(t, "Inbox", mock.Anything, mock.Anything)
	})
}

func TestProjectGovernance_ListQuarantinesUsesAdminProjection(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)
	govSvc := &mockProjectGovernanceSvc{}
	transitionedAt := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	govSvc.On("ListQuarantines", context.Background()).Return([]model.QuarantineSummary{{
		Project: "Org/Repo", ProjectKey: "org-repo", Generation: 7,
		Action: model.ProjectBlockActionBlock, State: model.ProjectBlockAckApplied, TransitionedAt: transitionedAt,
	}}, nil)

	response := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodGet, "/admin/quarantines", nil, "admin-token")

	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"quarantines":[{"project":"Org/Repo","canonical_project_key":"org-repo","generation":7,"action":"block","state":"applied","transitioned_at":"2026-08-02T12:00:00Z"}]}`, response.Body.String())
	require.NotContains(t, response.Body.String(), "actor_user_id")
	require.NotContains(t, response.Body.String(), "ack_token")
	govSvc.AssertExpectations(t)
}

func TestProjectGovernance_AckProjectBlockAllowsUserTokenWithoutDaemonClaims(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	govSvc := &mockProjectGovernanceSvc{}
	req := model.ProjectBlockAck{CommandID: "cmd-1", ProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: model.ProjectBlockAckApplied}
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
	req := model.ProjectBlockAck{CommandID: "cmd-1", ProjectKey: "jarvis-dev", AckToken: "ack-token-1", Status: model.ProjectBlockAckApplied, AppliedAt: time.Now().UTC()}
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
	req := model.ProjectBlockAck{CommandID: "forged-cmd", ProjectKey: "jarvis-dev", AckToken: "forged-token", Status: model.ProjectBlockAckApplied, AppliedAt: time.Now().UTC()}
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
	cmd := model.ProjectBlockCommand{CommandID: "cmd-1", AckToken: "ack-token-1", Project: "Org/Repo", ProjectKey: "org-repo", Reason: "duplicate", BlockedAt: time.Now().UTC()}
	govSvc.On("Status", context.Background(), "Org/Repo").Return(model.ProjectBlockStatusResponse{Project: "Org/Repo", ProjectKey: "org-repo", Blocked: true, Reason: "duplicate", Command: &cmd}, nil)

	w := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodGet, "/admin/project-blocks/status?project=Org%2FRepo", nil, "admin-token")
	require.Equal(t, http.StatusOK, w.Code)

	var resp model.ProjectBlockStatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "duplicate", resp.Reason)
	require.NotNil(t, resp.Command)
}

func TestProjectGovernance_QuarantineDetailIsAdminOnlyAndRedacted(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)
	govSvc := &mockProjectGovernanceSvc{}
	acknowledgedAt := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	govSvc.On("QuarantineProgress", context.Background(), "org-repo", int64(7), "", 20).Return(model.QuarantineProgressResponse{
		Project: "Org/Repo", ProjectKey: "org-repo", Generation: 7, Action: model.ProjectBlockActionBlock,
		Totals:   model.QuarantineProgressTotals{Active: 2, Acknowledged: 1, Pending: 1},
		Progress: []model.QuarantineProgressRow{{Username: "ada", State: model.ProjectBlockAckApplied, AcknowledgedAt: &acknowledgedAt}, {Username: "zoe", State: "pending"}},
	}, nil)

	w := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodGet, "/admin/quarantines/org-repo?generation=7&limit=20", nil, "admin-token")
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"project":"Org/Repo","canonical_project_key":"org-repo","generation":7,"action":"block","totals":{"active":2,"acknowledged":1,"pending":1},"progress":[{"username":"ada","state":"applied","acknowledged_at":"2026-08-02T12:00:00Z"},{"username":"zoe","state":"pending"}]}`, w.Body.String())
	require.NotContains(t, w.Body.String(), "email")
	require.NotContains(t, w.Body.String(), "auth_subject")
	govSvc.AssertExpectations(t)

	memberAuth := &mockAuthSvc{}
	memberAuth.On("ValidateToken", "member-token").Return(testClaims(), nil)
	member := doAuthRequest(t, governanceDeps(memberAuth, &mockProjectGovernanceSvc{}, &mockSyncSvc{}), http.MethodGet, "/admin/quarantines/org-repo", nil, "member-token")
	require.Equal(t, http.StatusForbidden, member.Code)
	require.NotContains(t, member.Body.String(), "org-repo")
}

func TestProjectGovernance_QuarantineDetailRejectsInvalidCursorWithoutProgress(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)
	govSvc := &mockProjectGovernanceSvc{}
	govSvc.On("QuarantineProgress", context.Background(), "org-repo", int64(7), "tampered", 20).
		Return(model.QuarantineProgressResponse{}, model.ErrInvalidQuarantineCursor)

	w := doAuthRequest(t, governanceDeps(authSvc, govSvc, &mockSyncSvc{}), http.MethodGet, "/admin/quarantines/org-repo?generation=7&limit=20&after=tampered", nil, "admin-token")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.NotContains(t, w.Body.String(), "org-repo")
	govSvc.AssertExpectations(t)
}

func TestProjectGovernance_LegacySlashUnsafeBlockRouteIsNotRegistered(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)
	req := model.ProjectBlockRequest{Action: model.ProjectBlockActionBlock, Reason: "duplicate", Confirmation: "org-repo", ExportMarker: "export-1"}

	w := doAuthRequest(t, governanceDeps(authSvc, &mockProjectGovernanceSvc{}, &mockSyncSvc{}), http.MethodPost, "/admin/projects/Org%2FRepo/block", req, "admin-token")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestSync_BlockedProjectMapsToHTTP423(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	syncSvc := &mockSyncSvc{}
	cmd := model.ProjectBlockCommand{CommandID: "cmd-1", AckToken: "ack-token-1", Project: "jarvis-dev", ProjectKey: "jarvis-dev", Reason: "duplicate", BlockedAt: time.Now().UTC()}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").Return(nil, &service.ProjectBlockedError{Command: cmd})

	w := doAuthRequest(t, governanceDeps(authSvc, &mockProjectGovernanceSvc{}, syncSvc), http.MethodPost, "/sync", map[string]any{"project": "jarvis-dev", "memories": []any{}}, "valid-token")
	require.Equal(t, http.StatusLocked, w.Code)

	var resp model.ProjectBlockedErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "cmd-1", resp.Command.CommandID)
}
