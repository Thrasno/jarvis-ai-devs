package repository

import (
	"context"

	"github.com/Thrasno/jarvis-dev/hive-api/internal/model"
)

type AuditRepository interface {
	Insert(ctx context.Context, entry *model.AuditEntry) error
	List(ctx context.Context, filter model.AuditFilter) ([]*model.AuditEntry, error)
	Count(ctx context.Context, filter model.AuditFilter) (int64, error)
}
