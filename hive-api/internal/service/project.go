package service

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
)

type ProjectService interface {
	List(ctx context.Context) (model.ProjectListResponse, error)
}

type projectService struct {
	repo repository.ProjectRepository
}

func NewProjectService(repo repository.ProjectRepository) ProjectService {
	return &projectService{repo: repo}
}

func (s *projectService) List(ctx context.Context) (model.ProjectListResponse, error) {
	records, err := s.repo.ListAggregates(ctx)
	if err != nil {
		return model.ProjectListResponse{}, err
	}

	projects := make([]model.ProjectSummary, 0, len(records))
	for _, record := range records {
		projects = append(projects, model.ProjectSummary{
			Name:           record.Name,
			MemoryCount:    record.MemoryCount,
			SessionCount:   record.SessionCount,
			LastActivityAt: latestTime(record.LastMemoryAt, record.LastSessionAt, record.LastSyncAt),
			SyncHealth:     syncHealth(record.LatestSyncOutcome),
		})
	}

	return model.ProjectListResponse{Projects: projects, Total: len(projects)}, nil
}

func latestTime(values ...*time.Time) *time.Time {
	var latest *time.Time
	for _, value := range values {
		if value == nil {
			continue
		}
		if latest == nil || value.After(*latest) {
			copy := *value
			latest = &copy
		}
	}
	return latest
}

func syncHealth(outcome *model.SyncAttemptOutcome) string {
	if outcome == nil {
		return model.ProjectSyncHealthUnknown
	}
	switch *outcome {
	case model.SyncAttemptOutcomeSuccess:
		return model.ProjectSyncHealthHealthy
	case model.SyncAttemptOutcomeFailure:
		return model.ProjectSyncHealthDegraded
	default:
		return model.ProjectSyncHealthUnknown
	}
}
