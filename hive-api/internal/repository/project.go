package repository

import (
	"context"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
)

type ProjectRepository interface {
	ListAggregates(ctx context.Context) ([]model.ProjectAggregate, error)
}
