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

	tests := []struct {
		name      string
		record    model.ProjectAggregate
		want      model.ProjectSummary
		wantTotal int
	}{
		{
			name: "latest activity uses max timestamp and successful sync is healthy",
			record: model.ProjectAggregate{
				Name:              "alpha",
				MemoryCount:       2,
				SessionCount:      1,
				LastMemoryAt:      timePtr(base.Add(-3 * time.Hour)),
				LastSessionAt:     timePtr(base.Add(-2 * time.Hour)),
				LastSyncAt:        timePtr(base.Add(-1 * time.Hour)),
				LatestSyncOutcome: &success,
			},
			want: model.ProjectSummary{
				Name:           "alpha",
				MemoryCount:    2,
				SessionCount:   1,
				LastActivityAt: timePtr(base.Add(-1 * time.Hour)),
				SyncHealth:     model.ProjectSyncHealthHealthy,
			},
			wantTotal: 1,
		},
		{
			name: "failed latest sync is degraded",
			record: model.ProjectAggregate{
				Name:              "beta",
				LastMemoryAt:      timePtr(base.Add(-30 * time.Minute)),
				LastSyncAt:        timePtr(base.Add(-90 * time.Minute)),
				LatestSyncOutcome: &failure,
			},
			want: model.ProjectSummary{
				Name:           "beta",
				LastActivityAt: timePtr(base.Add(-30 * time.Minute)),
				SyncHealth:     model.ProjectSyncHealthDegraded,
			},
			wantTotal: 1,
		},
		{
			name: "missing sync outcome is unknown and activity can be nil",
			record: model.ProjectAggregate{
				Name: "gamma",
			},
			want: model.ProjectSummary{
				Name:       "gamma",
				SyncHealth: model.ProjectSyncHealthUnknown,
			},
			wantTotal: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeProjectRepository{records: []model.ProjectAggregate{tt.record}}
			svc := service.NewProjectService(repo)

			got, err := svc.List(context.Background())

			require.NoError(t, err)
			require.Len(t, got.Projects, 1)
			assert.Equal(t, tt.wantTotal, got.Total)
			assert.Equal(t, tt.want, got.Projects[0])
		})
	}
}

func TestProjectService_ListReturnsNonNilEmptyResponse(t *testing.T) {
	svc := service.NewProjectService(&fakeProjectRepository{})

	got, err := svc.List(context.Background())

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
