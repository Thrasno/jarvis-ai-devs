package service

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
)

type ProjectService interface {
	List(ctx context.Context, health string) (model.ProjectListResponse, error)
}

type projectService struct {
	repo       repository.ProjectRepository
	healthRepo projectSyncHealthRepository
}

type projectSyncHealthRepository interface {
	ProjectSyncHealth(ctx context.Context) (model.ProjectSyncHealthProjection, error)
}

func NewProjectService(repo repository.ProjectRepository, healthRepo projectSyncHealthRepository) ProjectService {
	return &projectService{repo: repo, healthRepo: healthRepo}
}

func (s *projectService) List(ctx context.Context, health string) (model.ProjectListResponse, error) {
	records, err := s.repo.ListAggregates(ctx)
	if err != nil {
		return model.ProjectListResponse{}, err
	}
	projection, err := s.healthRepo.ProjectSyncHealth(ctx)
	if err != nil {
		return model.ProjectListResponse{}, err
	}
	// Both sides carry the literal project spelling stored on the row, so they
	// join on exact equality. Folding here would attach one project's sync
	// health to a different project that merely spells its name similarly.
	//
	// This is also why ListAggregates must return that literal as the name and
	// not a display spelling: the lookup below is keyed on it. A record named
	// anything else silently finds no health row, which drops the project out of
	// ?health=degraded entirely rather than showing it as degraded.
	healthByProject := make(map[string]model.ProjectSyncHealthRow, len(projection.Rows))
	for _, row := range projection.Rows {
		healthByProject[row.Project] = row
	}

	projects := make([]model.ProjectSummary, 0, len(records))
	for _, record := range records {
		row, participates := healthByProject[record.Name]
		summary := model.ProjectSummary{
			Name:           record.Name,
			MemoryCount:    record.MemoryCount,
			SessionCount:   record.SessionCount,
			LastActivityAt: latestTime(record.LastMemoryAt, record.LastSessionAt, record.LastSyncAt),
		}
		if participates {
			status := projectSyncHealth(row.LastOutcome)
			summary.SyncHealth = &status
			summary.LastActivityAt = latestTime(summary.LastActivityAt, timePtr(row.LastActivityAt))
		}
		if health == model.ProjectSyncHealthDegraded && (!participates || *summary.SyncHealth != health) {
			continue
		}
		projects = append(projects, summary)
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

func projectSyncHealth(outcome model.SyncAttemptOutcome) string {
	switch outcome {
	case model.SyncAttemptOutcomeSuccess:
		return model.ProjectSyncHealthHealthy
	case model.SyncAttemptOutcomeFailure:
		return model.ProjectSyncHealthDegraded
	default:
		return ""
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
