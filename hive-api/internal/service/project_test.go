package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectService_ListMapsAggregatesToSummaries(t *testing.T) {
	base := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	success := model.SyncAttemptOutcomeSuccess
	failure := model.SyncAttemptOutcomeFailure

	repo := &fakeProjectRepository{records: []model.ProjectAggregate{
		{
			Name:              "alpha",
			MemoryCount:       2,
			SessionCount:      1,
			LastMemoryAt:      timePtr(base.Add(-3 * time.Hour)),
			LastSessionAt:     timePtr(base.Add(-2 * time.Hour)),
			LastSyncAt:        timePtr(base.Add(-1 * time.Hour)),
			LatestSyncOutcome: &failure,
		},
		{
			Name:              "beta",
			LastMemoryAt:      timePtr(base.Add(-30 * time.Minute)),
			LastSyncAt:        timePtr(base.Add(-90 * time.Minute)),
			LatestSyncOutcome: &success,
		},
		{
			Name:              "gamma",
			LatestSyncOutcome: &failure,
		},
	}}
	healthRepo := &fakeProjectHealthRepository{projection: model.ProjectSyncHealthProjection{Rows: []model.ProjectSyncHealthRow{
		{Project: "alpha", LastOutcome: model.SyncAttemptOutcomeSuccess, LastActivityAt: base.Add(-1 * time.Hour)},
		{Project: "beta", LastOutcome: model.SyncAttemptOutcomeFailure, LastActivityAt: base.Add(-90 * time.Minute)},
	}}}
	svc := service.NewProjectService(repo, healthRepo)

	got, err := svc.List(context.Background(), "")

	require.NoError(t, err)
	assert.Equal(t, []model.ProjectSummary{
		{Name: "alpha", MemoryCount: 2, SessionCount: 1, LastActivityAt: timePtr(base.Add(-1 * time.Hour)), SyncHealth: stringPtr(model.ProjectSyncHealthHealthy)},
		{Name: "beta", LastActivityAt: timePtr(base.Add(-30 * time.Minute)), SyncHealth: stringPtr(model.ProjectSyncHealthDegraded)},
		{Name: "gamma", SyncHealth: nil},
	}, got.Projects)
	assert.Equal(t, 3, got.Total)

	degraded, err := svc.List(context.Background(), model.ProjectSyncHealthDegraded)
	require.NoError(t, err)
	assert.Equal(t, []model.ProjectSummary{{Name: "beta", LastActivityAt: timePtr(base.Add(-30 * time.Minute)), SyncHealth: stringPtr(model.ProjectSyncHealthDegraded)}}, degraded.Projects)
	assert.Equal(t, 1, degraded.Total)
}

func TestProjectService_ListKeepsSyncActivityForNonParticipatingProjects(t *testing.T) {
	base := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	failure := model.SyncAttemptOutcomeFailure

	repo := &fakeProjectRepository{records: []model.ProjectAggregate{{
		Name:              "orphaned",
		LastMemoryAt:      timePtr(base.Add(-5 * time.Hour)),
		LastSyncAt:        timePtr(base.Add(-1 * time.Hour)),
		LatestSyncOutcome: &failure,
	}}}
	healthRepo := &fakeProjectHealthRepository{projection: model.ProjectSyncHealthProjection{}}
	svc := service.NewProjectService(repo, healthRepo)

	got, err := svc.List(context.Background(), "")

	require.NoError(t, err)
	assert.Equal(t, []model.ProjectSummary{{
		Name:           "orphaned",
		LastActivityAt: timePtr(base.Add(-1 * time.Hour)),
		SyncHealth:     nil,
	}}, got.Projects)
}

func TestProjectService_ListReturnsNonNilEmptyResponse(t *testing.T) {
	svc := service.NewProjectService(&fakeProjectRepository{}, &fakeProjectHealthRepository{})

	got, err := svc.List(context.Background(), "")

	require.NoError(t, err)
	assert.NotNil(t, got.Projects)
	assert.Len(t, got.Projects, 0)
	assert.Equal(t, 0, got.Total)
}

type fakeProjectRepository struct {
	records []model.ProjectAggregate
	err     error
}

func (f *fakeProjectRepository) ListAggregates(context.Context) ([]model.ProjectAggregate, error) {
	return f.records, f.err
}

func timePtr(t time.Time) *time.Time {
	return &t
}

type fakeProjectHealthRepository struct {
	projection model.ProjectSyncHealthProjection
	err        error
}

func (f *fakeProjectHealthRepository) ProjectSyncHealth(context.Context) (model.ProjectSyncHealthProjection, error) {
	return f.projection, f.err
}

func stringPtr(value string) *string {
	return &value
}
