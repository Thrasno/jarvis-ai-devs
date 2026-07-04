package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newTestOverviewService(t *testing.T) (service.OverviewService, *repository.MockMemoryRepository, *repository.MockSyncAttemptRepository, *repository.MockAuditRepository) {
	t.Helper()
	memRepo := &repository.MockMemoryRepository{}
	syncRepo := &repository.MockSyncAttemptRepository{}
	auditRepo := &repository.MockAuditRepository{}
	svc := service.NewOverviewService(memRepo, syncRepo, auditRepo)
	return svc, memRepo, syncRepo, auditRepo
}

// --- GetStats tests ---

func TestOverviewService_GetStats_FullyPopulated(t *testing.T) {
	svc, memRepo, syncRepo, auditRepo := newTestOverviewService(t)
	ctx := context.Background()

	syncRepo.On("DaemonHealth", ctx, mock.AnythingOfType("time.Duration"), mock.AnythingOfType("time.Duration")).
		Return(2, 5, nil)
	auditRepo.On("CountSyncConflicts", ctx, mock.AnythingOfType("time.Time")).
		Return(3, nil)
	lastActivityAt := time.Date(2026, 7, 4, 12, 30, 0, 0, time.UTC)
	syncRepo.On("SyncHealthByProject", ctx, mock.AnythingOfType("time.Duration")).
		Return([]model.ProjectSyncHealthRow{
			{Project: "proj-a", LastOutcome: model.SyncAttemptOutcomeSuccess, ContributorCount: 2, LastActivityAt: lastActivityAt},
		}, nil)
	memRepo.On("CountLiveActivity", ctx, mock.AnythingOfType("time.Time")).
		Return(10, "sync-id-abc", nil)
	memRepo.On("CountByProject", ctx, model.MemoryFilter{}).
		Return([]model.ProjectCount{
			{Project: "proj-a", Count: 100},
			{Project: "proj-b", Count: 50},
			{Project: "proj-c", Count: 30},
		}, nil)

	stats, err := svc.GetStats(ctx)
	require.NoError(t, err)

	assert.Equal(t, 2, stats.DaemonHealth.Healthy)
	assert.Equal(t, 5, stats.DaemonHealth.Total)
	assert.Equal(t, 3, stats.Conflicts.Open)
	assert.Len(t, stats.SyncHealthByProject, 1)
	assert.Equal(t, "proj-a", stats.SyncHealthByProject[0].Project)
	assert.Equal(t, "healthy", stats.SyncHealthByProject[0].Status)
	require.NotNil(t, stats.SyncHealthByProject[0].LastActivityAt)
	assert.Equal(t, lastActivityAt, *stats.SyncHealthByProject[0].LastActivityAt)
	assert.Equal(t, 10, stats.LiveActivity.Count)
	assert.Equal(t, "sync-id-abc", stats.LiveActivity.NewestSyncID)
	assert.Len(t, stats.MostActiveProjects, 3)
	assert.NotNil(t, stats.SyncHealthByProject)
	assert.NotNil(t, stats.MostActiveProjects)
}

func TestOverviewService_GetStats_DaemonHealthZero(t *testing.T) {
	svc, memRepo, syncRepo, auditRepo := newTestOverviewService(t)
	ctx := context.Background()

	syncRepo.On("DaemonHealth", ctx, mock.Anything, mock.Anything).Return(0, 0, nil)
	auditRepo.On("CountSyncConflicts", ctx, mock.AnythingOfType("time.Time")).Return(0, nil)
	syncRepo.On("SyncHealthByProject", ctx, mock.Anything).Return([]model.ProjectSyncHealthRow{}, nil)
	memRepo.On("CountLiveActivity", ctx, mock.AnythingOfType("time.Time")).Return(0, "", nil)
	memRepo.On("CountByProject", ctx, model.MemoryFilter{}).Return([]model.ProjectCount{}, nil)

	stats, err := svc.GetStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, stats.DaemonHealth.Healthy)
	assert.Equal(t, 0, stats.DaemonHealth.Total)
}

func TestOverviewService_GetStats_ConflictsZero(t *testing.T) {
	svc, memRepo, syncRepo, auditRepo := newTestOverviewService(t)
	ctx := context.Background()

	syncRepo.On("DaemonHealth", ctx, mock.Anything, mock.Anything).Return(1, 1, nil)
	auditRepo.On("CountSyncConflicts", ctx, mock.AnythingOfType("time.Time")).Return(0, nil)
	syncRepo.On("SyncHealthByProject", ctx, mock.Anything).Return([]model.ProjectSyncHealthRow{}, nil)
	memRepo.On("CountLiveActivity", ctx, mock.AnythingOfType("time.Time")).Return(0, "", nil)
	memRepo.On("CountByProject", ctx, model.MemoryFilter{}).Return([]model.ProjectCount{}, nil)

	stats, err := svc.GetStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Conflicts.Open)
}

func TestOverviewService_GetStats_SyncHealthByProjectEmpty(t *testing.T) {
	svc, memRepo, syncRepo, auditRepo := newTestOverviewService(t)
	ctx := context.Background()

	syncRepo.On("DaemonHealth", ctx, mock.Anything, mock.Anything).Return(0, 0, nil)
	auditRepo.On("CountSyncConflicts", ctx, mock.AnythingOfType("time.Time")).Return(0, nil)
	syncRepo.On("SyncHealthByProject", ctx, mock.Anything).Return([]model.ProjectSyncHealthRow{}, nil)
	memRepo.On("CountLiveActivity", ctx, mock.AnythingOfType("time.Time")).Return(0, "", nil)
	memRepo.On("CountByProject", ctx, model.MemoryFilter{}).Return([]model.ProjectCount{}, nil)

	stats, err := svc.GetStats(ctx)
	require.NoError(t, err)
	assert.NotNil(t, stats.SyncHealthByProject)
	assert.Empty(t, stats.SyncHealthByProject)
}

func TestOverviewService_GetStats_MostActiveProjectsCappedAtFive(t *testing.T) {
	svc, memRepo, syncRepo, auditRepo := newTestOverviewService(t)
	ctx := context.Background()

	syncRepo.On("DaemonHealth", ctx, mock.Anything, mock.Anything).Return(0, 0, nil)
	auditRepo.On("CountSyncConflicts", ctx, mock.AnythingOfType("time.Time")).Return(0, nil)
	syncRepo.On("SyncHealthByProject", ctx, mock.Anything).Return([]model.ProjectSyncHealthRow{}, nil)
	memRepo.On("CountLiveActivity", ctx, mock.AnythingOfType("time.Time")).Return(0, "", nil)
	// Return 7 items — must be capped at 5
	memRepo.On("CountByProject", ctx, model.MemoryFilter{}).Return([]model.ProjectCount{
		{Project: "a", Count: 7},
		{Project: "b", Count: 6},
		{Project: "c", Count: 5},
		{Project: "d", Count: 4},
		{Project: "e", Count: 3},
		{Project: "f", Count: 2},
		{Project: "g", Count: 1},
	}, nil)

	stats, err := svc.GetStats(ctx)
	require.NoError(t, err)
	assert.Len(t, stats.MostActiveProjects, 5)
}

func TestOverviewService_GetStats_MostActiveProjectsUnderFive(t *testing.T) {
	svc, memRepo, syncRepo, auditRepo := newTestOverviewService(t)
	ctx := context.Background()

	syncRepo.On("DaemonHealth", ctx, mock.Anything, mock.Anything).Return(0, 0, nil)
	auditRepo.On("CountSyncConflicts", ctx, mock.AnythingOfType("time.Time")).Return(0, nil)
	syncRepo.On("SyncHealthByProject", ctx, mock.Anything).Return([]model.ProjectSyncHealthRow{}, nil)
	memRepo.On("CountLiveActivity", ctx, mock.AnythingOfType("time.Time")).Return(0, "", nil)
	memRepo.On("CountByProject", ctx, model.MemoryFilter{}).Return([]model.ProjectCount{
		{Project: "a", Count: 3},
		{Project: "b", Count: 2},
		{Project: "c", Count: 1},
	}, nil)

	stats, err := svc.GetStats(ctx)
	require.NoError(t, err)
	assert.Len(t, stats.MostActiveProjects, 3)
}

func TestOverviewService_GetStats_DaemonHealthError(t *testing.T) {
	svc, _, syncRepo, _ := newTestOverviewService(t)
	ctx := context.Background()
	repoErr := errors.New("db error")

	syncRepo.On("DaemonHealth", ctx, mock.Anything, mock.Anything).Return(0, 0, repoErr)

	_, err := svc.GetStats(ctx)
	assert.ErrorIs(t, err, repoErr)
}

func TestOverviewService_GetStats_ConflictsError(t *testing.T) {
	svc, _, syncRepo, auditRepo := newTestOverviewService(t)
	ctx := context.Background()
	repoErr := errors.New("audit db error")

	syncRepo.On("DaemonHealth", ctx, mock.Anything, mock.Anything).Return(1, 1, nil)
	auditRepo.On("CountSyncConflicts", ctx, mock.AnythingOfType("time.Time")).Return(0, repoErr)

	_, err := svc.GetStats(ctx)
	assert.ErrorIs(t, err, repoErr)
}

func TestOverviewService_GetStats_SyncHealthByProjectError(t *testing.T) {
	svc, _, syncRepo, auditRepo := newTestOverviewService(t)
	ctx := context.Background()
	repoErr := errors.New("sync health error")

	syncRepo.On("DaemonHealth", ctx, mock.Anything, mock.Anything).Return(1, 1, nil)
	auditRepo.On("CountSyncConflicts", ctx, mock.AnythingOfType("time.Time")).Return(0, nil)
	syncRepo.On("SyncHealthByProject", ctx, mock.Anything).Return(nil, repoErr)

	_, err := svc.GetStats(ctx)
	assert.ErrorIs(t, err, repoErr)
}

func TestOverviewService_GetStats_LiveActivityError(t *testing.T) {
	svc, memRepo, syncRepo, auditRepo := newTestOverviewService(t)
	ctx := context.Background()
	repoErr := errors.New("live activity error")

	syncRepo.On("DaemonHealth", ctx, mock.Anything, mock.Anything).Return(1, 1, nil)
	auditRepo.On("CountSyncConflicts", ctx, mock.AnythingOfType("time.Time")).Return(0, nil)
	syncRepo.On("SyncHealthByProject", ctx, mock.Anything).Return([]model.ProjectSyncHealthRow{}, nil)
	memRepo.On("CountLiveActivity", ctx, mock.AnythingOfType("time.Time")).Return(0, "", repoErr)

	_, err := svc.GetStats(ctx)
	assert.ErrorIs(t, err, repoErr)
}

func TestOverviewService_GetStats_CountByProjectError(t *testing.T) {
	svc, memRepo, syncRepo, auditRepo := newTestOverviewService(t)
	ctx := context.Background()
	repoErr := errors.New("count by project error")

	syncRepo.On("DaemonHealth", ctx, mock.Anything, mock.Anything).Return(1, 1, nil)
	auditRepo.On("CountSyncConflicts", ctx, mock.AnythingOfType("time.Time")).Return(0, nil)
	syncRepo.On("SyncHealthByProject", ctx, mock.Anything).Return([]model.ProjectSyncHealthRow{}, nil)
	memRepo.On("CountLiveActivity", ctx, mock.AnythingOfType("time.Time")).Return(0, "", nil)
	memRepo.On("CountByProject", ctx, model.MemoryFilter{}).Return(nil, repoErr)

	_, err := svc.GetStats(ctx)
	assert.ErrorIs(t, err, repoErr)
}

// --- GetGrowth tests ---

func TestOverviewService_GetGrowth_ReturnsFiveChartPoints(t *testing.T) {
	svc, memRepo, _, _ := newTestOverviewService(t)
	ctx := context.Background()

	memRepo.On("CountGrowthByMonth", ctx, 5).Return([]model.MonthCount{
		{Label: "Jan 2026", Value: 10},
		{Label: "Feb 2026", Value: 15},
		{Label: "Mar 2026", Value: 20},
		{Label: "Apr 2026", Value: 25},
		{Label: "May 2026", Value: 30},
	}, nil)

	resp, err := svc.GetGrowth(ctx)
	require.NoError(t, err)
	assert.Len(t, resp.KnowledgeGrowth, 5)
	assert.Equal(t, "Jan 2026", resp.KnowledgeGrowth[0].Label)
	assert.Equal(t, 10, resp.KnowledgeGrowth[0].Value)
}

func TestOverviewService_GetGrowth_EmptyReturnsEmptySlice(t *testing.T) {
	svc, memRepo, _, _ := newTestOverviewService(t)
	ctx := context.Background()

	memRepo.On("CountGrowthByMonth", ctx, 5).Return([]model.MonthCount{}, nil)

	resp, err := svc.GetGrowth(ctx)
	require.NoError(t, err)
	assert.NotNil(t, resp.KnowledgeGrowth)
	assert.Empty(t, resp.KnowledgeGrowth)
}

func TestOverviewService_GetStats_SyncHealthStatusMapping(t *testing.T) {
	svc, memRepo, syncRepo, auditRepo := newTestOverviewService(t)
	ctx := context.Background()

	syncRepo.On("DaemonHealth", ctx, mock.Anything, mock.Anything).Return(1, 1, nil)
	auditRepo.On("CountSyncConflicts", ctx, mock.AnythingOfType("time.Time")).Return(0, nil)
	syncRepo.On("SyncHealthByProject", ctx, mock.Anything).Return([]model.ProjectSyncHealthRow{
		{Project: "proj-success", LastOutcome: model.SyncAttemptOutcomeSuccess, ContributorCount: 1},
		{Project: "proj-fail", LastOutcome: model.SyncAttemptOutcomeFailure, ContributorCount: 2},
	}, nil)
	memRepo.On("CountLiveActivity", ctx, mock.AnythingOfType("time.Time")).Return(0, "", nil)
	memRepo.On("CountByProject", ctx, model.MemoryFilter{}).Return([]model.ProjectCount{}, nil)

	stats, err := svc.GetStats(ctx)
	require.NoError(t, err)

	var successStatus, failStatus string
	for _, p := range stats.SyncHealthByProject {
		switch p.Project {
		case "proj-success":
			successStatus = p.Status
		case "proj-fail":
			failStatus = p.Status
		}
	}
	assert.Equal(t, "healthy", successStatus)
	assert.Equal(t, "degraded", failStatus)
}

// Ensure CountLiveActivity is called with a time.Time (not duration) argument.
// This test verifies the since argument is in the past.
func TestOverviewService_GetStats_LiveActivitySinceIsInPast(t *testing.T) {
	svc, memRepo, syncRepo, auditRepo := newTestOverviewService(t)
	ctx := context.Background()

	before := time.Now().UTC()

	syncRepo.On("DaemonHealth", ctx, mock.Anything, mock.Anything).Return(0, 0, nil)
	auditRepo.On("CountSyncConflicts", ctx, mock.MatchedBy(func(since time.Time) bool {
		return since.Before(before)
	})).Return(0, nil)
	syncRepo.On("SyncHealthByProject", ctx, mock.Anything).Return([]model.ProjectSyncHealthRow{}, nil)
	memRepo.On("CountLiveActivity", ctx, mock.MatchedBy(func(since time.Time) bool {
		return since.Before(before)
	})).Return(0, "", nil)
	memRepo.On("CountByProject", ctx, model.MemoryFilter{}).Return([]model.ProjectCount{}, nil)

	_, err := svc.GetStats(ctx)
	require.NoError(t, err)
	memRepo.AssertExpectations(t)
	auditRepo.AssertExpectations(t)
}
