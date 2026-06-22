package service

import (
	"context"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
)

const (
	overviewDaemonHealthyWindow = 24 * time.Hour
	overviewWindow30d           = 30 * 24 * time.Hour
	overviewLiveWindow          = time.Hour
	overviewGrowthMonths        = 5
	overviewTopProjects         = 5
)

// OverviewService provides aggregated dashboard metrics.
type OverviewService interface {
	GetStats(ctx context.Context) (*model.OverviewStatsResponse, error)
	GetGrowth(ctx context.Context) (*model.OverviewGrowthResponse, error)
}

type overviewService struct {
	memRepo   repository.MemoryRepository
	syncRepo  repository.SyncAttemptRepository
	auditRepo repository.AuditRepository
}

// NewOverviewService creates an OverviewService with the given repositories.
func NewOverviewService(
	mem repository.MemoryRepository,
	sync repository.SyncAttemptRepository,
	audit repository.AuditRepository,
) OverviewService {
	return &overviewService{
		memRepo:   mem,
		syncRepo:  sync,
		auditRepo: audit,
	}
}

// GetStats aggregates all overview stats in a single response.
func (s *overviewService) GetStats(ctx context.Context) (*model.OverviewStatsResponse, error) {
	resp := &model.OverviewStatsResponse{
		SyncHealthByProject: []model.ProjectSyncHealth{},
		MostActiveProjects:  []model.ProjectCount{},
	}

	// 1. Daemon health
	healthy, total, err := s.syncRepo.DaemonHealth(ctx, overviewDaemonHealthyWindow, overviewWindow30d)
	if err != nil {
		return nil, err
	}
	resp.DaemonHealth = model.OverviewDaemonHealth{Healthy: healthy, Total: total}

	// 2. Conflicts (open sync conflicts in 30d)
	conflicts, err := s.auditRepo.CountSyncConflicts(ctx, time.Now().UTC().Add(-overviewWindow30d))
	if err != nil {
		return nil, err
	}
	resp.Conflicts = model.OverviewConflicts{Open: conflicts}

	// 3. Sync health by project
	healthRows, err := s.syncRepo.SyncHealthByProject(ctx, overviewWindow30d)
	if err != nil {
		return nil, err
	}
	for _, row := range healthRows {
		status := "degraded"
		if row.LastOutcome == model.SyncAttemptOutcomeSuccess {
			status = "healthy"
		}
		resp.SyncHealthByProject = append(resp.SyncHealthByProject, model.ProjectSyncHealth{
			Project:          row.Project,
			Status:           status,
			Region:           "",
			ContributorCount: row.ContributorCount,
		})
	}

	// 4. Live activity (last 1h)
	count, newestSyncID, err := s.memRepo.CountLiveActivity(ctx, time.Now().UTC().Add(-overviewLiveWindow))
	if err != nil {
		return nil, err
	}
	resp.LiveActivity = model.OverviewLiveActivity{Count: count, NewestSyncID: newestSyncID}

	// 5. Most active projects (capped at top 5)
	byProject, err := s.memRepo.CountByProject(ctx, model.MemoryFilter{})
	if err != nil {
		return nil, err
	}
	top := byProject
	if len(top) > overviewTopProjects {
		top = top[:overviewTopProjects]
	}
	resp.MostActiveProjects = top

	return resp, nil
}

// GetGrowth returns cumulative knowledge growth by month.
func (s *overviewService) GetGrowth(ctx context.Context) (*model.OverviewGrowthResponse, error) {
	months, err := s.memRepo.CountGrowthByMonth(ctx, overviewGrowthMonths)
	if err != nil {
		return nil, err
	}

	points := []model.OverviewChartPoint{}
	for _, m := range months {
		points = append(points, model.OverviewChartPoint{Label: m.Label, Value: m.Value})
	}
	return &model.OverviewGrowthResponse{KnowledgeGrowth: points}, nil
}
