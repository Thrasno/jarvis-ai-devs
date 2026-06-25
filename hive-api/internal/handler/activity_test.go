package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func activityDeps(authSvc *mockAuthSvc, activitySvc *mockActivitySvc) RouterDeps {
	return RouterDeps{
		AuthSvc:     authSvc,
		MemorySvc:   &mockMemorySvc{},
		SyncSvc:     &mockSyncSvc{},
		ProjectSvc:  &mockProjectSvc{},
		AdminSvc:    &mockAdminSvc{},
		OverviewSvc: &mockOverviewSvc{},
		ActivitySvc: activitySvc,
	}
}

func TestActivityFeed_AuthenticatedRequestReturnsLifecycleEntries(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	occurredAt := time.Date(2026, 6, 25, 10, 30, 0, 0, time.UTC)
	activitySvc := &mockActivitySvc{}
	activitySvc.On("List", context.Background(), model.ActivityFeedQuery{Limit: 3}).Return(&model.ActivityFeedResponse{
		Entries: []model.ActivityFeedEntry{
			{
				ID:           "evt-create",
				EventType:    model.ActivityEventCreate,
				OccurredAt:   occurredAt,
				Actor:        "ada",
				Project:      "jarvis-dev",
				Category:     "decision",
				Title:        "Created activity",
				Summary:      "Created memory",
				MemorySyncID: "mem-create",
			},
			{
				ID:           "evt-update",
				EventType:    model.ActivityEventUpdate,
				OccurredAt:   occurredAt.Add(-time.Minute),
				Actor:        "ada",
				Project:      "jarvis-dev",
				Category:     "pattern",
				Title:        "Updated activity",
				Summary:      "Updated memory",
				MemorySyncID: "mem-update",
			},
			{
				ID:           "evt-delete",
				EventType:    model.ActivityEventDelete,
				OccurredAt:   occurredAt.Add(-2 * time.Minute),
				Actor:        "ada",
				Project:      "jarvis-dev",
				Category:     "bugfix",
				Title:        "Deleted activity",
				Summary:      "Deleted memory",
				MemorySyncID: "mem-delete",
			},
		},
	}, nil)

	w := doAuthRequest(t, activityDeps(authSvc, activitySvc), http.MethodGet, "/activity?limit=3", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	var response model.ActivityFeedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Entries, 3)
	assert.Equal(t, model.ActivityEventCreate, response.Entries[0].EventType)
	assert.Equal(t, "evt-create", response.Entries[0].ID)
	assert.Equal(t, "Created memory", response.Entries[0].Summary)
	assert.Equal(t, "mem-create", response.Entries[0].MemorySyncID)
	assert.Equal(t, model.ActivityEventUpdate, response.Entries[1].EventType)
	assert.Equal(t, "Updated activity", response.Entries[1].Title)
	assert.Equal(t, "pattern", response.Entries[1].Category)
	assert.Equal(t, model.ActivityEventDelete, response.Entries[2].EventType)
	assert.Equal(t, "Deleted memory", response.Entries[2].Summary)
	assert.Equal(t, occurredAt.Add(-2*time.Minute), response.Entries[2].OccurredAt)
	authSvc.AssertExpectations(t)
	activitySvc.AssertExpectations(t)
}

func TestActivityFeed_UnauthenticatedRequestIsRejected(t *testing.T) {
	w := doRequest(t, activityDeps(&mockAuthSvc{}, &mockActivitySvc{}), http.MethodGet, "/activity", nil)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.JSONEq(t, `{"error":"authorization header requerido"}`, w.Body.String())
}

func TestActivityFeed_InvalidCursorReturnsBadRequest(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	activitySvc := &mockActivitySvc{}
	activitySvc.On("List", context.Background(), model.ActivityFeedQuery{Limit: 2, Cursor: "bad-cursor"}).
		Return((*model.ActivityFeedResponse)(nil), service.ErrInvalidActivityCursor)

	w := doAuthRequest(t, activityDeps(authSvc, activitySvc), http.MethodGet, "/activity?limit=2&cursor=bad-cursor", nil, "valid-token")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"invalid activity cursor"}`, w.Body.String())
	authSvc.AssertExpectations(t)
	activitySvc.AssertExpectations(t)
}

func TestActivityFeed_SemanticallyInvalidDecodedCursorReturnsBadRequest(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)
	repo := &handlerActivityRepoStub{}
	cursor, err := service.EncodeActivityCursor(model.ActivityFeedCursor{
		OccurredAt: time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC),
		Sequence:   10,
		EventID:    "not-a-uuid",
	})
	require.NoError(t, err)

	w := doAuthRequest(t, RouterDeps{
		AuthSvc:     authSvc,
		MemorySvc:   &mockMemorySvc{},
		SyncSvc:     &mockSyncSvc{},
		ProjectSvc:  &mockProjectSvc{},
		AdminSvc:    &mockAdminSvc{},
		OverviewSvc: &mockOverviewSvc{},
		ActivitySvc: service.NewActivityService(repo),
	}, http.MethodGet, "/activity?cursor="+cursor, nil, "valid-token")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"invalid activity cursor"}`, w.Body.String())
	assert.False(t, repo.called)
	authSvc.AssertExpectations(t)
}

func TestActivityFeed_EmptyFeedReturnsEmptyEntries(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	activitySvc := &mockActivitySvc{}
	activitySvc.On("List", context.Background(), model.ActivityFeedQuery{}).Return(&model.ActivityFeedResponse{}, nil)

	w := doAuthRequest(t, activityDeps(authSvc, activitySvc), http.MethodGet, "/activity", nil, "valid-token")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"entries":[]}`, w.Body.String())
	authSvc.AssertExpectations(t)
	activitySvc.AssertExpectations(t)
}

func TestActivityFeed_ServiceFailureReturnsInternalServerError(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	activitySvc := &mockActivitySvc{}
	activitySvc.On("List", context.Background(), model.ActivityFeedQuery{}).
		Return((*model.ActivityFeedResponse)(nil), errors.New("repository unavailable"))

	w := doAuthRequest(t, activityDeps(authSvc, activitySvc), http.MethodGet, "/activity", nil, "valid-token")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.JSONEq(t, `{"error":"error listing activity feed"}`, w.Body.String())
	authSvc.AssertExpectations(t)
	activitySvc.AssertExpectations(t)
}

func TestActivityFeed_BindsQueryValidation(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(testClaims(), nil)

	w := doAuthRequest(t, activityDeps(authSvc, &mockActivitySvc{}), http.MethodGet, "/activity?limit=101", nil, "valid-token")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "Limit")
	authSvc.AssertExpectations(t)
}

var _ ActivityService = (*mockActivitySvc)(nil)

type handlerActivityRepoStub struct {
	called bool
}

func (r *handlerActivityRepoStub) ListActivityFeed(ctx context.Context, query model.ActivityFeedRepositoryQuery) ([]model.ActivityJournalRow, error) {
	r.called = true
	return nil, nil
}
