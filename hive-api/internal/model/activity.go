package model

import "time"

type ActivityEventType string

const (
	ActivityEventCreate ActivityEventType = "create"
	ActivityEventUpdate ActivityEventType = "update"
	ActivityEventDelete ActivityEventType = "delete"
)

type ActivityFeedQuery struct {
	Limit  int    `form:"limit" binding:"omitempty,min=1,max=100"`
	Cursor string `form:"cursor"`
}

type ActivityFeedCursor struct {
	OccurredAt time.Time `json:"occurred_at"`
	Sequence   int64     `json:"sequence"`
	EventID    string    `json:"event_id"`
}

type ActivityFeedRepositoryQuery struct {
	Limit  int
	Cursor *ActivityFeedCursor
}

type ActivityJournalRow struct {
	EventID      string
	EntityType   string
	EntitySyncID string
	Project      string
	Op           MutationOp
	Sequence     int64
	OccurredAt   time.Time
	ActorID      string
	Memory       *MemoryPayload
	Tombstone    *TombstonePayload
}

type ActivityFeedEntry struct {
	ID           string            `json:"id"`
	EventType    ActivityEventType `json:"event_type"`
	OccurredAt   time.Time         `json:"occurred_at"`
	Actor        string            `json:"actor"`
	Project      string            `json:"project"`
	Category     string            `json:"category"`
	Title        string            `json:"title"`
	Summary      string            `json:"summary"`
	MemorySyncID string            `json:"memory_sync_id"`
}

type ActivityFeedResponse struct {
	Entries    []ActivityFeedEntry `json:"entries"`
	NextCursor *string             `json:"next_cursor,omitempty"`
}
