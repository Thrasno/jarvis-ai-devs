package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresAuditRepository_CountSyncConflicts_ReturnsCountInWindow(t *testing.T) {
	pool, cleanup := startPostgresWithAuditLogs(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresAuditRepository(pool)
	now := time.Now().UTC()
	since := now.Add(-30 * 24 * time.Hour)

	// 5 sync_conflict entries in window
	for i := 0; i < 5; i++ {
		err := repo.Insert(ctx, &model.AuditEntry{
			OccurredAt: now.Add(-time.Duration(i) * time.Hour),
			Action:     model.AuditActionSyncConflict,
			Outcome:    model.AuditOutcomeConflict,
			EntryCount: 1,
			Metadata:   model.AuditMetadata{},
		})
		require.NoError(t, err)
	}
	// 2 older entries outside the window
	for i := 0; i < 2; i++ {
		err := repo.Insert(ctx, &model.AuditEntry{
			OccurredAt: now.Add(-31*24*time.Hour - time.Duration(i)*time.Hour),
			Action:     model.AuditActionSyncConflict,
			Outcome:    model.AuditOutcomeConflict,
			EntryCount: 1,
			Metadata:   model.AuditMetadata{},
		})
		require.NoError(t, err)
	}

	count, err := repo.CountSyncConflicts(ctx, since)
	require.NoError(t, err)
	assert.Equal(t, 5, count)
}

func TestPostgresAuditRepository_CountSyncConflicts_NoEntriesInWindow(t *testing.T) {
	pool, cleanup := startPostgresWithAuditLogs(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresAuditRepository(pool)
	since := time.Now().UTC().Add(-30 * 24 * time.Hour)

	// Insert only non-sync_conflict entries
	err := repo.Insert(ctx, &model.AuditEntry{
		OccurredAt: time.Now().UTC(),
		Action:     model.AuditActionSyncPush,
		Outcome:    model.AuditOutcomeSuccess,
		EntryCount: 1,
		Metadata:   model.AuditMetadata{},
	})
	require.NoError(t, err)

	count, err := repo.CountSyncConflicts(ctx, since)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestPostgresAuditRepository_CountSyncConflicts_EmptyTable(t *testing.T) {
	pool, cleanup := startPostgresWithAuditLogs(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresAuditRepository(pool)

	count, err := repo.CountSyncConflicts(ctx, time.Now().UTC().Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
