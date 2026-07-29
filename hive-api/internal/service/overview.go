package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
)

const (
	overviewLiveWindow   = time.Hour
	overviewGrowthMonths = 5
	overviewTopProjects  = 5
)

// OverviewService provides aggregated dashboard metrics.
type OverviewService interface {
	GetForLevel(ctx context.Context, level model.UserLevel) (*model.CapabilityOverviewResponse, error)
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

// GetForLevel constructs an allowlisted overview projection for the given level.
func (s *overviewService) GetForLevel(ctx context.Context, level model.UserLevel) (*model.CapabilityOverviewResponse, error) {
	summary, err := s.getCommonSummary(ctx)
	if err != nil {
		return nil, err
	}

	if level == model.LevelMember {
		return &model.CapabilityOverviewResponse{
			Capability: model.OverviewCapabilityMember,
			Summary:    summary,
		}, nil
	}
	if level != model.LevelAdmin {
		return nil, fmt.Errorf("unsupported overview level: %s", level)
	}

	stats, err := s.GetStats(ctx)
	if err != nil {
		return nil, err
	}
	growth, err := s.GetGrowth(ctx)
	if err != nil {
		return nil, err
	}

	return &model.CapabilityOverviewResponse{
		Capability: model.OverviewCapabilityAdmin,
		Summary:    summary,
		Operations: &model.AdminOverviewOperations{
			SyncingUsers:        stats.SyncingUsers,
			DegradedProjects:    stats.DegradedProjects,
			KnowledgeGrowth:     growth.KnowledgeGrowth,
			SyncHealthByProject: stats.SyncHealthByProject,
			NewestSyncID:        stats.LiveActivity.NewestSyncID,
		},
	}, nil
}

func (s *overviewService) getCommonSummary(ctx context.Context) (model.OverviewSummary, error) {
	totalMemories, err := s.memRepo.Count(ctx, model.MemoryFilter{})
	if err != nil {
		return model.OverviewSummary{}, err
	}
	byProject, err := s.memRepo.CountByProject(ctx, model.MemoryFilter{})
	if err != nil {
		return model.OverviewSummary{}, err
	}
	liveCount, _, err := s.memRepo.CountLiveActivity(ctx, time.Now().UTC().Add(-overviewLiveWindow))
	if err != nil {
		return model.OverviewSummary{}, err
	}

	activeProjects := 0
	for _, project := range byProject {
		if project.Count > 0 {
			activeProjects++
		}
	}
	topProjects := append([]model.ProjectCount{}, byProject...)
	if len(topProjects) > overviewTopProjects {
		topProjects = topProjects[:overviewTopProjects]
	}

	return model.OverviewSummary{
		TotalMemories:      totalMemories,
		ActiveProjects:     activeProjects,
		LiveActivity:       model.MemberOverviewLiveActivity{Count: liveCount},
		MostActiveProjects: topProjects,
	}, nil
}

// GetStats aggregates all overview stats in a single response.
func (s *overviewService) GetStats(ctx context.Context) (*model.OverviewStatsResponse, error) {
	resp := &model.OverviewStatsResponse{
		SyncHealthByProject: []model.ProjectSyncHealth{},
		MostActiveProjects:  []model.ProjectCount{},
	}

	// 1. User synchronization KPI.
	now := time.Now().UTC()
	projection, err := s.syncRepo.UserSyncProjection(ctx, now)
	if err != nil {
		return nil, err
	}
	for _, row := range projection.Rows {
		if !row.IsActive {
			continue
		}
		resp.SyncingUsers.Total++
		if userSyncStatus(row, now) == model.UserSyncStatusLast24h {
			resp.SyncingUsers.Syncing++
		}
	}

	// 2. Canonical project health drives the project rows.
	projectProjection, err := s.syncRepo.ProjectSyncHealth(ctx)
	if err != nil {
		return nil, err
	}
	resp.DegradedProjects = model.OverviewDegradedProjects{Degraded: projectProjection.Degraded, Total: projectProjection.Total}

	// 3. Sync health by project
	for _, row := range projectProjection.Rows {
		status := "degraded"
		if row.LastOutcome == model.SyncAttemptOutcomeSuccess {
			status = "healthy"
		}
		var lastActivityAt *time.Time
		if !row.LastActivityAt.IsZero() {
			value := row.LastActivityAt
			lastActivityAt = &value
		}
		resp.SyncHealthByProject = append(resp.SyncHealthByProject, model.ProjectSyncHealth{
			Project:          row.Project,
			Status:           status,
			Region:           "",
			ContributorCount: row.ContributorCount,
			LastActivityAt:   lastActivityAt,
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
