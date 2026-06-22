package repository

import (
	"context"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
)

import "time"

type AuditRepository interface {
	Insert(ctx context.Context, entry *model.AuditEntry) error
	List(ctx context.Context, filter model.AuditFilter) ([]*model.AuditEntry, error)
	Count(ctx context.Context, filter model.AuditFilter) (int64, error)

	// CountSyncConflicts counts sync_conflict audit entries occurred on or after since.
	CountSyncConflicts(ctx context.Context, since time.Time) (int, error)
}
