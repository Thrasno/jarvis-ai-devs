package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/google/uuid"
)

const (
	ActivityDefaultLimit = 20
	ActivityMaxLimit     = 100
)

var ErrInvalidActivityCursor = errors.New("invalid activity cursor")

type ActivityFeedRepository interface {
	ListActivityFeed(ctx context.Context, query model.ActivityFeedRepositoryQuery) ([]model.ActivityJournalRow, error)
}

type ActivityService interface {
	List(ctx context.Context, query model.ActivityFeedQuery) (*model.ActivityFeedResponse, error)
}

type activityService struct {
	repo ActivityFeedRepository
}

func NewActivityService(repo ActivityFeedRepository) ActivityService {
	return &activityService{repo: repo}
}

func (s *activityService) List(ctx context.Context, query model.ActivityFeedQuery) (*model.ActivityFeedResponse, error) {
	limit := normalizeActivityLimit(query.Limit)

	var cursor *model.ActivityFeedCursor
	if query.Cursor != "" {
		decoded, err := DecodeActivityCursor(query.Cursor)
		if err != nil {
			return nil, err
		}
		cursor = &decoded
	}

	rows, err := s.repo.ListActivityFeed(ctx, model.ActivityFeedRepositoryQuery{
		Limit:  limit + 1,
		Cursor: cursor,
	})
	if err != nil {
		return nil, err
	}

	entries := make([]model.ActivityFeedEntry, 0, limit)
	var nextCursor *string
	var lastCursor model.ActivityFeedCursor
	for _, row := range rows {
		entry, ok := mapActivityEntry(row)
		if !ok {
			continue
		}
		if len(entries) == limit {
			cursorValue, err := EncodeActivityCursor(lastCursor)
			if err != nil {
				return nil, err
			}
			nextCursor = &cursorValue
			break
		}
		lastCursor = model.ActivityFeedCursor{OccurredAt: row.OccurredAt, Sequence: row.Sequence, EventID: row.EventID}
		entries = append(entries, entry)
	}

	return &model.ActivityFeedResponse{Entries: entries, NextCursor: nextCursor}, nil
}

func EncodeActivityCursor(cursor model.ActivityFeedCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidActivityCursor, err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func DecodeActivityCursor(value string) (model.ActivityFeedCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return model.ActivityFeedCursor{}, ErrInvalidActivityCursor
	}

	var cursor model.ActivityFeedCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return model.ActivityFeedCursor{}, ErrInvalidActivityCursor
	}
	if cursor.OccurredAt.IsZero() || cursor.Sequence == 0 || cursor.EventID == "" {
		return model.ActivityFeedCursor{}, ErrInvalidActivityCursor
	}
	if _, err := uuid.Parse(cursor.EventID); err != nil {
		return model.ActivityFeedCursor{}, ErrInvalidActivityCursor
	}
	return cursor, nil
}

func normalizeActivityLimit(limit int) int {
	if limit <= 0 {
		return ActivityDefaultLimit
	}
	if limit > ActivityMaxLimit {
		return ActivityMaxLimit
	}
	return limit
}

func mapActivityEntry(row model.ActivityJournalRow) (model.ActivityFeedEntry, bool) {
	if row.EntityType != model.MutationEntityMemory {
		return model.ActivityFeedEntry{}, false
	}

	entry := model.ActivityFeedEntry{
		ID:           row.EventID,
		OccurredAt:   row.OccurredAt,
		Actor:        row.ActorID,
		Project:      row.Project,
		MemorySyncID: row.EntitySyncID,
	}
	if row.Memory != nil {
		entry.Project = firstNonEmpty(entry.Project, row.Memory.Project)
		entry.Category = string(row.Memory.Category)
		entry.Title = row.Memory.Title
		entry.MemorySyncID = firstNonEmpty(entry.MemorySyncID, row.Memory.SyncID)
	}

	switch row.Op {
	case model.MutationOpCreate:
		entry.EventType = model.ActivityEventCreate
		entry.Summary = "Created memory"
	case model.MutationOpUpdate:
		entry.EventType = model.ActivityEventUpdate
		entry.Summary = "Updated memory"
	case model.MutationOpDelete:
		entry.EventType = model.ActivityEventDelete
		entry.Summary = "Deleted memory"
		if row.Tombstone != nil {
			entry.Actor = firstNonEmpty(row.Tombstone.DeletedBy, entry.Actor)
		}
	default:
		return model.ActivityFeedEntry{}, false
	}

	return entry, true
}

func firstNonEmpty(preferred string, fallback string) string {
	if preferred != "" {
		return preferred
	}
	return fallback
}
