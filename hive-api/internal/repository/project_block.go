package repository

import (
	"context"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
)

type ProjectBlockRepository interface {
	BlockProject(ctx context.Context, create model.ProjectBlockCreate) (*model.ProjectBlock, error)
	GetByCanonicalKey(ctx context.Context, canonicalProjectKey string) (*model.ProjectBlock, error)
	ListInboxCommands(ctx context.Context, subject model.ProjectBlockAckSubject) ([]model.ProjectBlockCommand, error)
	EnsureAckDelivery(ctx context.Context, block *model.ProjectBlock, subject model.ProjectBlockAckSubject) (model.ProjectBlockCommand, error)
	GetAckDelivery(ctx context.Context, canonicalProjectKey, commandID string, subject model.ProjectBlockAckSubject) (*model.ProjectBlockAckDelivery, error)
	RecordAck(ctx context.Context, ack model.ProjectBlockAck) (model.ProjectBlockAck, error)
	LatestAckForCommand(ctx context.Context, canonicalProjectKey, commandID string) (*model.ProjectBlockAck, error)
	ListQuarantines(ctx context.Context) ([]model.QuarantineSummary, error)
	QuarantineProgress(ctx context.Context, canonicalProjectKey string, generation int64, after string, limit int) (model.QuarantineProgressResponse, error)
}
