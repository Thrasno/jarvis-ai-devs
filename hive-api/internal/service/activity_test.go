package service_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type activityFeedRepoStub struct {
	rows   []model.ActivityJournalRow
	query  model.ActivityFeedRepositoryQuery
	called bool
}

func (r *activityFeedRepoStub) ListActivityFeed(ctx context.Context, query model.ActivityFeedRepositoryQuery) ([]model.ActivityJournalRow, error) {
	r.called = true
	r.query = query
	return r.rows, nil
}

func TestActivityServiceListAppliesDefaultLimitAndReturnsEmptyFeed(t *testing.T) {
	repo := &activityFeedRepoStub{}
	svc := service.NewActivityService(repo)

	response, err := svc.List(context.Background(), model.ActivityFeedQuery{})

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.True(t, repo.called)
	assert.Equal(t, service.ActivityDefaultLimit+1, repo.query.Limit)
	assert.Nil(t, repo.query.Cursor)
	assert.NotNil(t, response.Entries)
	assert.Empty(t, response.Entries)
	assert.Nil(t, response.NextCursor)
}

func TestActivityServiceListCapsLimitAndBuildsNextCursor(t *testing.T) {
	second := activityRow("22222222-2222-4222-8222-222222222222", model.MutationOpUpdate, time.Date(2026, 6, 24, 12, 1, 0, 0, time.UTC), 20)
	repo := &activityFeedRepoStub{rows: []model.ActivityJournalRow{
		activityRow("evt-1", model.MutationOpCreate, time.Date(2026, 6, 24, 12, 2, 0, 0, time.UTC), 30),
		second,
		activityRow("evt-3", model.MutationOpDelete, time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC), 10),
	}}
	svc := service.NewActivityService(repo)

	response, err := svc.List(context.Background(), model.ActivityFeedQuery{Limit: 999})

	require.NoError(t, err)
	assert.Equal(t, service.ActivityMaxLimit+1, repo.query.Limit)
	assert.Len(t, response.Entries, 3)
	assert.Nil(t, response.NextCursor)

	repo = &activityFeedRepoStub{rows: []model.ActivityJournalRow{
		activityRow("evt-1", model.MutationOpCreate, time.Date(2026, 6, 24, 12, 2, 0, 0, time.UTC), 30),
		second,
		activityRow("evt-3", model.MutationOpDelete, time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC), 10),
	}}
	svc = service.NewActivityService(repo)

	response, err = svc.List(context.Background(), model.ActivityFeedQuery{Limit: 2})

	require.NoError(t, err)
	assert.Equal(t, 3, repo.query.Limit)
	require.Len(t, response.Entries, 2)
	require.NotNil(t, response.NextCursor)
	assert.NotContains(t, *response.NextCursor, "+")
	assert.NotContains(t, *response.NextCursor, "/")
	assert.NotContains(t, *response.NextCursor, "=")
	decoded, err := service.DecodeActivityCursor(*response.NextCursor)
	require.NoError(t, err)
	assert.Equal(t, second.OccurredAt, decoded.OccurredAt)
	assert.Equal(t, second.Sequence, decoded.Sequence)
	assert.Equal(t, second.EventID, decoded.EventID)
}

func TestActivityServiceListRejectsBadCursor(t *testing.T) {
	tests := []struct {
		name   string
		cursor string
	}{
		{
			name:   "base64 invalid cursor",
			cursor: "not-a-valid-cursor",
		},
		{
			name: "semantically invalid event id",
			cursor: encodedActivityCursorForTest(t, model.ActivityFeedCursor{
				OccurredAt: time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC),
				Sequence:   10,
				EventID:    "not-a-uuid",
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &activityFeedRepoStub{}
			svc := service.NewActivityService(repo)

			response, err := svc.List(context.Background(), model.ActivityFeedQuery{Cursor: tt.cursor})

			require.Error(t, err)
			assert.ErrorIs(t, err, service.ErrInvalidActivityCursor)
			assert.Nil(t, response)
			assert.False(t, repo.called)
		})
	}
}

func TestActivityServiceMapsCreateUpdateAndDeleteRows(t *testing.T) {
	createdAt := time.Date(2026, 6, 24, 12, 2, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 6, 24, 12, 1, 0, 0, time.UTC)
	deletedAt := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	repo := &activityFeedRepoStub{rows: []model.ActivityJournalRow{
		activityRowWithMemory("evt-create", model.MutationOpCreate, createdAt, 30, "creator", "sync-create", "jarvis-dev", model.CatDecision, "Created title", "Created content"),
		activityRowWithMemory("evt-update", model.MutationOpUpdate, updatedAt, 20, "editor", "sync-update", "jarvis-dev", model.CatBugfix, "Updated title", "Updated content"),
		activityDeleteRow("evt-delete", deletedAt, 10, "actor-from-event", "deleter", "no longer useful"),
	}}
	svc := service.NewActivityService(repo)

	response, err := svc.List(context.Background(), model.ActivityFeedQuery{Limit: 10})

	require.NoError(t, err)
	require.Len(t, response.Entries, 3)
	assert.Equal(t, model.ActivityEventCreate, response.Entries[0].EventType)
	assert.Equal(t, "creator", response.Entries[0].Actor)
	assert.Equal(t, "jarvis-dev", response.Entries[0].Project)
	assert.Equal(t, "decision", response.Entries[0].Category)
	assert.Equal(t, "Created title", response.Entries[0].Title)
	assert.Equal(t, "Created memory", response.Entries[0].Summary)
	assert.Equal(t, "sync-create", response.Entries[0].MemorySyncID)

	assert.Equal(t, model.ActivityEventUpdate, response.Entries[1].EventType)
	assert.Equal(t, "editor", response.Entries[1].Actor)
	assert.Equal(t, "bugfix", response.Entries[1].Category)
	assert.Equal(t, "Updated memory", response.Entries[1].Summary)

	assert.Equal(t, model.ActivityEventDelete, response.Entries[2].EventType)
	assert.Equal(t, "deleter", response.Entries[2].Actor)
	assert.Equal(t, "Deleted memory", response.Entries[2].Summary)
	assert.NotContains(t, response.Entries[2].Summary, "no longer useful")
}

func TestActivityServiceExcludesNonMemoryAndUnsupportedOperations(t *testing.T) {
	repo := &activityFeedRepoStub{rows: []model.ActivityJournalRow{
		activityRow("evt-restore", model.MutationOpRestore, time.Date(2026, 6, 24, 12, 3, 0, 0, time.UTC), 40),
		{
			EventID:      "evt-sync",
			EntityType:   "sync",
			EntitySyncID: "sync-operation",
			Project:      "jarvis-dev",
			Op:           model.MutationOpCreate,
			Sequence:     30,
			OccurredAt:   time.Date(2026, 6, 24, 12, 2, 0, 0, time.UTC),
			ActorID:      "sync-agent",
			Memory:       memoryPayload("sync-operation", "jarvis-dev", model.CatConfig, "Sync", "Ignored"),
		},
		activityRow("evt-create", model.MutationOpCreate, time.Date(2026, 6, 24, 12, 1, 0, 0, time.UTC), 20),
	}}
	svc := service.NewActivityService(repo)

	response, err := svc.List(context.Background(), model.ActivityFeedQuery{Limit: 10})

	require.NoError(t, err)
	require.Len(t, response.Entries, 1)
	assert.Equal(t, "evt-create", response.Entries[0].ID)
}

func activityRow(eventID string, op model.MutationOp, occurredAt time.Time, sequence int64) model.ActivityJournalRow {
	return activityRowWithMemory(eventID, op, occurredAt, sequence, "actor", "sync-"+eventID, "jarvis-dev", model.CatDecision, "Title "+eventID, "Content "+eventID)
}

func activityRowWithMemory(eventID string, op model.MutationOp, occurredAt time.Time, sequence int64, actor string, syncID string, project string, category model.MemoryCategory, title string, content string) model.ActivityJournalRow {
	return model.ActivityJournalRow{
		EventID:      eventID,
		EntityType:   model.MutationEntityMemory,
		EntitySyncID: syncID,
		Project:      project,
		Op:           op,
		Sequence:     sequence,
		OccurredAt:   occurredAt,
		ActorID:      actor,
		Memory:       memoryPayload(syncID, project, category, title, content),
	}
}

func activityDeleteRow(eventID string, occurredAt time.Time, sequence int64, actor string, deletedBy string, reason string) model.ActivityJournalRow {
	row := activityRowWithMemory(eventID, model.MutationOpDelete, occurredAt, sequence, actor, "sync-delete", "jarvis-dev", model.CatPattern, "Deleted title", "Deleted content")
	row.Tombstone = &model.TombstonePayload{DeletedAt: occurredAt, DeletedBy: deletedBy, Reason: reason}
	return row
}

func memoryPayload(syncID string, project string, category model.MemoryCategory, title string, content string) *model.MemoryPayload {
	return &model.MemoryPayload{
		SyncID:    syncID,
		Project:   project,
		Category:  category,
		Title:     title,
		Content:   content,
		CreatedBy: "creator",
	}
}

func encodedActivityCursorForTest(t *testing.T, cursor model.ActivityFeedCursor) string {
	t.Helper()

	data, err := json.Marshal(cursor)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(data)
}
