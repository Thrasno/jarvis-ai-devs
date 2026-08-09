package repository

import (
	"context"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
)

// ProjectBlockRepository stores and finds quarantines by project literal. Every
// projectKey here is a raw project spelling: callers pass what a request or a
// row carries, and it is matched with plain equality. The underlying
// `canonical_project_key` column keeps its historical name (migrations 012 and
// 018 hang foreign keys and a trigger off it) but holds no derived value.
type ProjectBlockRepository interface {
	BlockProject(ctx context.Context, create model.ProjectBlockCreate) (*model.ProjectBlock, error)
	// GetByProjectKey returns the active block stored under this exact project
	// literal, or ErrNotFound. It never folds spellings together.
	GetByProjectKey(ctx context.Context, projectKey string) (*model.ProjectBlock, error)
	ListInboxCommands(ctx context.Context, subject model.ProjectBlockAckSubject) ([]model.ProjectBlockCommand, error)
	EnsureAckDelivery(ctx context.Context, block *model.ProjectBlock, subject model.ProjectBlockAckSubject) (model.ProjectBlockCommand, error)
	GetAckDelivery(ctx context.Context, projectKey, commandID string, subject model.ProjectBlockAckSubject) (*model.ProjectBlockAckDelivery, error)
	RecordAck(ctx context.Context, ack model.ProjectBlockAck) (model.ProjectBlockAck, error)
	LatestAckForCommand(ctx context.Context, projectKey, commandID string) (*model.ProjectBlockAck, error)
	ListQuarantines(ctx context.Context) ([]model.QuarantineSummary, error)
	QuarantineProgress(ctx context.Context, projectKey string, generation int64, after string, limit int) (model.QuarantineProgressResponse, error)
}
