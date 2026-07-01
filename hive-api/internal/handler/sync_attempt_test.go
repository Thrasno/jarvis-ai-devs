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

func syncAttemptDeps(authSvc *mockAuthSvc, attemptsSvc *mockSyncAttemptSvc) RouterDeps {
	return RouterDeps{
		AuthSvc:        authSvc,
		MemorySvc:      &mockMemorySvc{},
		SyncSvc:        &mockSyncSvc{},
		SyncAttemptSvc: attemptsSvc,
		AdminSvc:       &mockAdminSvc{},
	}
}

func syncAttemptMemberUser(email string) *model.User {
	return &model.User{ID: "user-uuid-123", Username: "testuser", Email: email, Level: model.LevelMember, IsActive: true}
}

func TestSyncAttempts_RejectsMissingDevID(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	authSvc.On("GetCurrentUser", mock.Anything, "user-uuid-123").Return(syncAttemptMemberUser("dev@example.com"), nil)
	svc := &mockSyncAttemptSvc{}
	svc.On("Ingest", context.Background(), mock.AnythingOfType("model.SyncAttemptIngestRequest")).Return(model.SyncAttemptIngestResponse{Rejected: []model.SyncAttemptRejected{{AttemptID: "attempt-1", Error: "dev_id is required"}}}, nil)

	w := doAuthRequest(t, syncAttemptDeps(authSvc, svc), http.MethodPost, "/sync-attempts", map[string]interface{}{
		"attempts": []interface{}{map[string]interface{}{"attempt_id": "attempt-1", "project": "jarvis-dev", "started_at": time.Now().UTC().Format(time.RFC3339), "outcome": "success"}},
	}, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	var resp model.SyncAttemptIngestResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Rejected, 1)
	assert.Equal(t, "attempt-1", resp.Rejected[0].AttemptID)
	svc.AssertExpectations(t)
}

func TestSyncAttempts_RejectsOversizedBatch(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	authSvc.On("GetCurrentUser", mock.Anything, "user-uuid-123").Return(syncAttemptMemberUser("dev@example.com"), nil)
	attempts := make([]interface{}, 101)
	for i := range attempts {
		attempts[i] = map[string]interface{}{"attempt_id": "attempt", "dev_id": "dev@example.com", "project": "jarvis-dev", "started_at": time.Now().UTC().Format(time.RFC3339), "outcome": "success"}
	}

	w := doAuthRequest(t, syncAttemptDeps(authSvc, &mockSyncAttemptSvc{}), http.MethodPost, "/sync-attempts", map[string]interface{}{"attempts": attempts}, "valid-token")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSyncAttempts_ReturnsAcceptedDuplicateAndRejectedIDs(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	authSvc.On("GetCurrentUser", mock.Anything, "user-uuid-123").Return(syncAttemptMemberUser("dev@example.com"), nil)
	expected := model.SyncAttemptIngestResponse{
		AcceptedIDs:  []string{"new"},
		DuplicateIDs: []string{"existing"},
		Rejected:     []model.SyncAttemptRejected{{AttemptID: "invalid", Error: "dev_id is required"}},
	}
	svc := &mockSyncAttemptSvc{}
	svc.On("Ingest", context.Background(), mock.AnythingOfType("model.SyncAttemptIngestRequest")).Return(expected, nil)

	w := doAuthRequest(t, syncAttemptDeps(authSvc, svc), http.MethodPost, "/sync-attempts", map[string]interface{}{
		"attempts": []interface{}{map[string]interface{}{"attempt_id": "new", "dev_id": "dev@example.com", "project": "jarvis-dev", "started_at": time.Now().UTC().Format(time.RFC3339), "outcome": "success"}},
	}, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	var resp model.SyncAttemptIngestResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, expected.AcceptedIDs, resp.AcceptedIDs)
	assert.Equal(t, expected.DuplicateIDs, resp.DuplicateIDs)
	assert.Equal(t, expected.Rejected, resp.Rejected)
	svc.AssertExpectations(t)
}

func TestSyncAttempts_RejectsCrossDeveloperSpoofingForNonAdmin(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	authSvc.On("GetCurrentUser", mock.Anything, "user-uuid-123").Return(syncAttemptMemberUser("dev@example.com"), nil)
	svc := &mockSyncAttemptSvc{}

	w := doAuthRequest(t, syncAttemptDeps(authSvc, svc), http.MethodPost, "/sync-attempts", map[string]interface{}{
		"attempts": []interface{}{map[string]interface{}{"attempt_id": "spoofed", "dev_id": "other@example.com", "project": "jarvis-dev", "started_at": time.Now().UTC().Format(time.RFC3339), "outcome": "success"}},
	}, "valid-token")

	assert.Equal(t, http.StatusForbidden, w.Code)
	svc.AssertNotCalled(t, "Ingest", mock.Anything, mock.Anything)
	authSvc.AssertExpectations(t)
}

func TestSyncAttempts_AdminCanIngestForAnotherDeveloper(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "admin-token").Return(adminClaims(), nil)
	expected := model.SyncAttemptIngestResponse{AcceptedIDs: []string{"admin-attempt"}}
	svc := &mockSyncAttemptSvc{}
	svc.On("Ingest", context.Background(), mock.AnythingOfType("model.SyncAttemptIngestRequest")).Return(expected, nil)

	w := doAuthRequest(t, syncAttemptDeps(authSvc, svc), http.MethodPost, "/sync-attempts", map[string]interface{}{
		"attempts": []interface{}{map[string]interface{}{"attempt_id": "admin-attempt", "dev_id": "other@example.com", "project": "jarvis-dev", "started_at": time.Now().UTC().Format(time.RFC3339), "outcome": "success"}},
	}, "admin-token")

	assert.Equal(t, http.StatusOK, w.Code)
	svc.AssertExpectations(t)
	authSvc.AssertExpectations(t)
}

func TestSyncAttempts_DoesNotChangePostSyncRoute(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	syncSvc := &mockSyncSvc{}
	syncSvc.On("Push", context.Background(), mock.AnythingOfType("model.SyncRequest"), "user-uuid-123").Return(&model.SyncResponse{Pushed: 0, Pulled: []*model.Memory{}}, nil)
	syncSvc.On("PullAll", context.Background(), "jarvis-dev", mock.AnythingOfType("time.Time"), mock.AnythingOfType("[]string"), mock.AnythingOfType("int"), mock.AnythingOfType("model.PullCursor"), mock.AnythingOfType("model.PullCursor")).Return(&model.PullResult{Sessions: []*model.Session{}, Memories: []*model.Memory{}}, nil)

	w := doAuthRequest(t, RouterDeps{AuthSvc: authSvc, MemorySvc: &mockMemorySvc{}, SyncSvc: syncSvc, SyncAttemptSvc: &mockSyncAttemptSvc{}, AdminSvc: &mockAdminSvc{}}, http.MethodPost, "/sync", map[string]interface{}{"project": "jarvis-dev", "memories": []interface{}{}}, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	syncSvc.AssertExpectations(t)
}

func TestAdminSyncAttemptSummary_RequiresAuthAndAdmin(t *testing.T) {
	t.Run("unauthenticated request is denied", func(t *testing.T) {
		w := doRequest(t, syncAttemptDeps(&mockAuthSvc{}, &mockSyncAttemptSvc{}), http.MethodGet, "/admin/sync-attempts/summary?window=24h", nil)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("non-admin request is denied", func(t *testing.T) {
		authSvc := &mockAuthSvc{}
		authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

		w := doAuthRequest(t, syncAttemptDeps(authSvc, &mockSyncAttemptSvc{}), http.MethodGet, "/admin/sync-attempts/summary?window=24h", nil, "valid-token")

		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestAdminSyncAttemptSummary_AdminCanReadSummary(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(adminClaims(), nil)
	lastSuccess := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	expected := model.SyncAttemptSummaryResponse{Windows: []model.SyncAttemptWindowSummary{{
		Window:        "24h",
		Total:         2,
		Successes:     1,
		Failures:      1,
		FailureRate:   0.5,
		LastSuccessAt: &lastSuccess,
		ByDeveloper:   []model.SyncAttemptDimensionCount{{Key: "ada@example.com", Count: 2}},
		TopErrors:     []model.SyncAttemptDimensionCount{{Key: "network_error", Count: 1}},
	}}}
	svc := &mockSyncAttemptSvc{}
	svc.On("Summary", context.Background(), model.SyncAttemptSummaryQuery{Window: "24h", Project: "jarvis-dev", DevID: "ada@example.com", Client: "hive-daemon", DaemonID: "daemon-a", Outcome: "failure", ErrorCode: "network_error"}).Return(expected, nil)

	w := doAuthRequest(t, syncAttemptDeps(authSvc, svc), http.MethodGet, "/admin/sync-attempts/summary?window=24h&project=jarvis-dev&dev_id=ada@example.com&client=hive-daemon&daemon_id=daemon-a&outcome=failure&error_code=network_error", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	var resp model.SyncAttemptSummaryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, expected, resp)
	svc.AssertExpectations(t)
}

func TestAdminSyncAttemptSummary_RejectsInvalidWindow(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(adminClaims(), nil)

	w := doAuthRequest(t, syncAttemptDeps(authSvc, &mockSyncAttemptSvc{}), http.MethodGet, "/admin/sync-attempts/summary?window=1h", nil, "valid-token")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminSyncAttemptSummary_ServiceFailure(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(adminClaims(), nil)
	svc := &mockSyncAttemptSvc{}
	svc.On("Summary", context.Background(), model.SyncAttemptSummaryQuery{Window: "7d"}).Return(model.SyncAttemptSummaryResponse{}, errors.New("database unavailable"))

	w := doAuthRequest(t, syncAttemptDeps(authSvc, svc), http.MethodGet, "/admin/sync-attempts/summary?window=7d", nil, "valid-token")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
