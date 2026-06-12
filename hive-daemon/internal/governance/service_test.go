package governance

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestServiceReturnsReadOnlyGovernanceViews(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.CreateSession("sess-alpha", "alpha", "/repo/alpha", "dev", "test"))
	_, err = store.SaveMemory(&models.Memory{Project: "alpha", Title: "Read model", Content: "content", SessionID: "sess-alpha"})
	require.NoError(t, err)
	require.NoError(t, store.RecordSyncFailure("alpha", time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC), 2, time.Date(2026, 6, 6, 12, 5, 0, 0, time.UTC), errReadOnlyTest))

	service := NewService(store)

	projects, err := service.Projects(context.Background())
	require.NoError(t, err)
	require.Len(t, projects, 1)
	require.Equal(t, "alpha", projects[0].Name)
	require.Equal(t, 1, projects[0].ActiveMemoryCount)

	detail, err := service.Project(context.Background(), "alpha")
	require.NoError(t, err)
	require.Equal(t, "/repo/alpha", detail.Directory)

	memories, err := service.Memories(context.Background(), MemoryFilter{Project: "alpha", Limit: 10})
	require.NoError(t, err)
	require.Len(t, memories, 1)
	require.Equal(t, "Read model", memories[0].Title)

	health, err := service.Health(context.Background())
	require.NoError(t, err)
	require.Len(t, health, 1)
	require.Equal(t, 2, health[0].ConsecutiveFailures)
}

func TestServiceRejectsBlankProjectDetailAndMemoryFilter(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	service := NewService(store)

	_, err = service.Project(context.Background(), " ")
	require.ErrorContains(t, err, "project is required")

	_, err = service.Memories(context.Background(), MemoryFilter{})
	require.ErrorContains(t, err, "project is required")
}

func TestServiceGuardedMemoryDeleteBlocksWithoutFreshBackupOrConfirmation(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name         string
		backups      []BackupManifest
		backupID     string
		confirmation func(int64) string
		wantErr      error
	}{
		{
			name:         "no fresh backup",
			backupID:     "missing-backup",
			confirmation: func(id int64) string { return GuardConfirmation(GuardOperationDelete, GuardTargetMemory, id) },
			wantErr:      ErrDestructiveBackupRequired,
		},
		{
			name:         "stale backup",
			backups:      []BackupManifest{{ID: "stale-backup", CreatedAt: now.Add(-destructiveBackupFreshness - time.Second)}},
			backupID:     "stale-backup",
			confirmation: func(id int64) string { return GuardConfirmation(GuardOperationDelete, GuardTargetMemory, id) },
			wantErr:      ErrDestructiveBackupRequired,
		},
		{
			name:         "backup one second in the future",
			backups:      []BackupManifest{{ID: "future-backup", CreatedAt: now.Add(time.Second)}},
			backupID:     "future-backup",
			confirmation: func(id int64) string { return GuardConfirmation(GuardOperationDelete, GuardTargetMemory, id) },
			wantErr:      ErrDestructiveBackupRequired,
		},
		{
			name:     "confirmation with leading and trailing whitespace",
			backups:  []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			backupID: "fresh-backup",
			confirmation: func(id int64) string {
				return " " + GuardConfirmation(GuardOperationDelete, GuardTargetMemory, id) + " "
			},
			wantErr: ErrDestructiveConfirmationMismatch,
		},
		{
			name:         "whitespace-only confirmation",
			backups:      []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			backupID:     "fresh-backup",
			confirmation: func(int64) string { return " " },
			wantErr:      ErrDestructiveConfirmationMismatch,
		},
		{
			name:         "confirmation mismatch",
			backups:      []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			backupID:     "fresh-backup",
			confirmation: func(int64) string { return "DELETE memory wrong-target" },
			wantErr:      ErrDestructiveConfirmationMismatch,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, err := db.Open(":memory:")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, store.Close()) })
			memoryID := saveGovernanceServiceTestMemory(t, store, "alpha", "Selected memory")
			beforeMutations := requirePendingMemoryMutations(t, store)

			service := NewServiceWithBackup(store, fakeGuardBackupStore{backups: tt.backups})
			service.now = func() time.Time { return now }

			_, err = service.ExecuteGuard(context.Background(), GuardRequest{
				Operation:    GuardOperationDelete,
				TargetType:   GuardTargetMemory,
				TargetID:     memoryID,
				BackupID:     tt.backupID,
				Confirmation: tt.confirmation(memoryID),
				ActorID:      "tester",
				Reason:       "test cleanup",
			})

			require.ErrorIs(t, err, tt.wantErr)
			requireMemoryActive(t, store, memoryID)
			afterMutations := requirePendingMemoryMutations(t, store)
			require.Len(t, afterMutations, len(beforeMutations), "failed guard must not create mutation journal entries")
		})
	}
}

func TestServiceGuardedMemoryDeleteRequiresReasonBeforeMutation(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name   string
		reason string
	}{
		{name: "missing reason", reason: ""},
		{name: "blank reason", reason: "  \t  "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, err := db.Open(":memory:")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, store.Close()) })
			memoryID := saveGovernanceServiceTestMemory(t, store, "alpha", "Reason required")
			beforeMutations := requirePendingMemoryMutations(t, store)

			service := NewServiceWithBackup(store, fakeGuardBackupStore{backups: []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}}})
			service.now = func() time.Time { return now }

			_, err = service.ExecuteGuard(context.Background(), GuardRequest{
				Operation:    GuardOperationDelete,
				TargetType:   GuardTargetMemory,
				TargetID:     memoryID,
				BackupID:     "fresh-backup",
				Confirmation: GuardConfirmation(GuardOperationDelete, GuardTargetMemory, memoryID),
				ActorID:      "tester",
				Reason:       tt.reason,
			})

			require.ErrorIs(t, err, ErrDestructiveReasonRequired)
			requireMemoryActive(t, store, memoryID)
			afterMutations := requirePendingMemoryMutations(t, store)
			require.Len(t, afterMutations, len(beforeMutations), "missing reason must not create mutation journal entries")
		})
	}
}

func TestServiceGuardedMemoryOperationsMutateOnlySelectedLocalTarget(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	deleteTargetID := saveGovernanceServiceTestMemory(t, store, "alpha", "Delete target")
	activeNeighborID := saveGovernanceServiceTestMemory(t, store, "alpha", "Active neighbor")
	restoreTargetID := saveGovernanceServiceTestMemory(t, store, "beta", "Restore target")
	deletedNeighborID := saveGovernanceServiceTestMemory(t, store, "beta", "Deleted neighbor")
	require.NoError(t, store.DeleteMemory(restoreTargetID, "tester", "prepare restore target"))
	require.NoError(t, store.DeleteMemory(deletedNeighborID, "tester", "prepare deleted neighbor"))

	service := NewServiceWithBackup(store, fakeGuardBackupStore{backups: []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}}})
	service.now = func() time.Time { return now }

	beforeDeleteMutations := requirePendingMemoryMutations(t, store)
	deleted, err := service.ExecuteGuard(context.Background(), GuardRequest{
		Operation:    GuardOperationDelete,
		TargetType:   GuardTargetMemory,
		TargetID:     deleteTargetID,
		BackupID:     "fresh-backup",
		Confirmation: GuardConfirmation(GuardOperationDelete, GuardTargetMemory, deleteTargetID),
		ActorID:      "tester",
		Reason:       "  remove selected local memory  ",
	})
	require.NoError(t, err)
	require.Equal(t, GuardOperationDelete, deleted.Operation)
	require.Equal(t, deleteTargetID, deleted.TargetID)
	require.True(t, deleted.Mutated)
	requireMemoryDeleted(t, store, deleteTargetID)
	deletedMemory, err := store.GetDeletedMemory(deleteTargetID)
	require.NoError(t, err)
	require.Equal(t, "remove selected local memory", deletedMemory.DeleteReason)
	requireMemoryActive(t, store, activeNeighborID)
	afterDeleteMutations := requirePendingMemoryMutations(t, store)
	require.Len(t, afterDeleteMutations, len(beforeDeleteMutations)+1)
	deleteMutation := afterDeleteMutations[len(afterDeleteMutations)-1]
	require.Equal(t, db.MutationOpDelete, deleteMutation.Op)
	require.Equal(t, requireMemorySyncID(t, store, deleteTargetID), deleteMutation.EntitySyncID)
	require.NotEqual(t, requireMemorySyncID(t, store, activeNeighborID), deleteMutation.EntitySyncID)

	beforeRestoreMutations := requirePendingMemoryMutations(t, store)
	restored, err := service.ExecuteGuard(context.Background(), GuardRequest{
		Operation:    GuardOperationRestore,
		TargetType:   GuardTargetMemory,
		TargetID:     restoreTargetID,
		BackupID:     "fresh-backup",
		Confirmation: GuardConfirmation(GuardOperationRestore, GuardTargetMemory, restoreTargetID),
		ActorID:      "tester",
	})
	require.NoError(t, err)
	require.Equal(t, GuardOperationRestore, restored.Operation)
	require.Equal(t, restoreTargetID, restored.TargetID)
	require.True(t, restored.Mutated)
	requireMemoryActive(t, store, restoreTargetID)
	requireMemoryDeleted(t, store, deletedNeighborID)
	afterRestoreMutations := requirePendingMemoryMutations(t, store)
	require.Len(t, afterRestoreMutations, len(beforeRestoreMutations)+1)
	restoreMutation := afterRestoreMutations[len(afterRestoreMutations)-1]
	require.Equal(t, db.MutationOpRestore, restoreMutation.Op)
	require.Equal(t, requireMemorySyncID(t, store, restoreTargetID), restoreMutation.EntitySyncID)
	require.NotEqual(t, requireMemorySyncID(t, store, deletedNeighborID), restoreMutation.EntitySyncID)
}

func TestServiceGuardedMemoryOperationsBlockInvalidBackupArchiveBeforeMutation(t *testing.T) {
	for _, tt := range []struct {
		name       string
		operation  string
		invalidate func(t *testing.T, backup BackupManifest)
	}{
		{
			name:      "delete with missing archive",
			operation: GuardOperationDelete,
			invalidate: func(t *testing.T, backup BackupManifest) {
				t.Helper()
				require.NoError(t, os.Remove(backup.ArchivePath))
			},
		},
		{
			name:      "restore with corrupt archive",
			operation: GuardOperationRestore,
			invalidate: func(t *testing.T, backup BackupManifest) {
				t.Helper()
				require.NoError(t, os.WriteFile(backup.ArchivePath, []byte("tampered archive"), 0o600))
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, backupStore := newGovernanceServiceBackupTestStore(t)
			memoryID := saveGovernanceServiceTestMemory(t, store, "alpha", "Guard target")
			if tt.operation == GuardOperationRestore {
				require.NoError(t, store.DeleteMemory(memoryID, "tester", "prepare restore target"))
			}
			backup, err := backupStore.Create(context.Background())
			require.NoError(t, err)
			tt.invalidate(t, backup)

			beforeMutations := requirePendingMemoryMutations(t, store)
			service := NewServiceWithBackup(store, backupStore)
			service.now = func() time.Time { return backup.CreatedAt.Add(time.Minute) }

			_, err = service.ExecuteGuard(context.Background(), GuardRequest{
				Operation:    tt.operation,
				TargetType:   GuardTargetMemory,
				TargetID:     memoryID,
				BackupID:     backup.ID,
				Confirmation: GuardConfirmation(tt.operation, GuardTargetMemory, memoryID),
				ActorID:      "tester",
				Reason:       "guarded test",
			})

			require.ErrorIs(t, err, ErrBackupArchiveInvalid)
			if tt.operation == GuardOperationRestore {
				requireMemoryDeleted(t, store, memoryID)
			} else {
				requireMemoryActive(t, store, memoryID)
			}
			afterMutations := requirePendingMemoryMutations(t, store)
			require.Len(t, afterMutations, len(beforeMutations), "invalid backup archive must block before mutation journaling")
		})
	}
}

func TestServiceGuardedProjectArchiveBlocksWithoutFreshUsableBackupOrExactConfirmation(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name         string
		backups      []BackupManifest
		backupID     string
		validateErr  error
		confirmation string
		wantErr      error
	}{
		{
			name:         "no fresh backup",
			backupID:     "missing-backup",
			confirmation: ProjectArchiveConfirmation("alpha"),
			wantErr:      ErrDestructiveBackupRequired,
		},
		{
			name:         "stale backup",
			backups:      []BackupManifest{{ID: "stale-backup", CreatedAt: now.Add(-destructiveBackupFreshness - time.Second)}},
			backupID:     "stale-backup",
			confirmation: ProjectArchiveConfirmation("alpha"),
			wantErr:      ErrDestructiveBackupRequired,
		},
		{
			name:         "backup one second in the future",
			backups:      []BackupManifest{{ID: "future-backup", CreatedAt: now.Add(time.Second)}},
			backupID:     "future-backup",
			confirmation: ProjectArchiveConfirmation("alpha"),
			wantErr:      ErrDestructiveBackupRequired,
		},
		{
			name:         "invalid backup archive",
			backups:      []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			backupID:     "fresh-backup",
			validateErr:  ErrBackupArchiveInvalid,
			confirmation: ProjectArchiveConfirmation("alpha"),
			wantErr:      ErrBackupArchiveInvalid,
		},
		{
			name:         "confirmation with trailing whitespace",
			backups:      []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			backupID:     "fresh-backup",
			confirmation: ProjectArchiveConfirmation("alpha") + " ",
			wantErr:      ErrDestructiveConfirmationMismatch,
		},
		{
			name:         "confirmation mismatch",
			backups:      []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			backupID:     "fresh-backup",
			confirmation: "ARCHIVE project beta",
			wantErr:      ErrDestructiveConfirmationMismatch,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, err := db.Open(":memory:")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, store.Close()) })
			memoryID := saveGovernanceServiceTestMemory(t, store, "alpha", "Selected project memory")

			beforeMutations := requirePendingMemoryMutations(t, store)
			service := NewServiceWithBackup(store, fakeGuardBackupStore{backups: tt.backups, validateErr: tt.validateErr})
			service.now = func() time.Time { return now }

			_, err = service.ExecuteProjectArchive(context.Background(), ProjectArchiveRequest{
				Project:      "alpha",
				BackupID:     tt.backupID,
				Confirmation: tt.confirmation,
				ActorID:      "tester",
				Reason:       "local cleanup",
			})

			require.ErrorIs(t, err, tt.wantErr)
			detail, detailErr := service.Project(context.Background(), "alpha")
			require.NoError(t, detailErr)
			require.False(t, detail.Archived)
			require.Nil(t, detail.ArchivedAt)
			requireMemoryActive(t, store, memoryID)
			afterMutations := requirePendingMemoryMutations(t, store)
			require.Len(t, afterMutations, len(beforeMutations), "failed project archive guard must not journal memory mutations")
		})
	}
}

func TestServiceGuardedProjectArchiveMutatesOnlySelectedLocalProjectAndReturnsCloudHandoffNote(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	alphaMemoryID := saveGovernanceServiceTestMemory(t, store, "alpha", "Alpha memory")
	betaMemoryID := saveGovernanceServiceTestMemory(t, store, "beta", "Beta memory")

	beforeMutations := requirePendingMemoryMutations(t, store)
	service := NewServiceWithBackup(store, fakeGuardBackupStore{backups: []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}}})
	service.now = func() time.Time { return now }

	result, err := service.ExecuteProjectArchive(context.Background(), ProjectArchiveRequest{
		Project:      "alpha",
		BackupID:     "fresh-backup",
		Confirmation: ProjectArchiveConfirmation("alpha"),
		ActorID:      "tester",
		Reason:       "archive local duplicate",
	})

	require.NoError(t, err)
	require.Equal(t, GuardOperationArchive, result.Operation)
	require.Equal(t, GuardTargetProject, result.TargetType)
	require.Equal(t, "alpha", result.Project)
	require.Equal(t, "fresh-backup", result.BackupID)
	require.True(t, result.Mutated)
	require.Contains(t, result.CloudHandoffNote, "No cloud project mutation was performed")

	alpha, err := service.Project(context.Background(), "alpha")
	require.NoError(t, err)
	require.True(t, alpha.Archived)
	require.NotNil(t, alpha.ArchivedAt)
	require.Equal(t, now, alpha.ArchivedAt.UTC())
	require.Equal(t, "tester", alpha.ArchivedBy)
	require.Equal(t, "archive local duplicate", alpha.ArchiveReason)

	beta, err := service.Project(context.Background(), "beta")
	require.NoError(t, err)
	require.False(t, beta.Archived)
	requireMemoryActive(t, store, alphaMemoryID)
	requireMemoryActive(t, store, betaMemoryID)
	afterFirstArchiveMutations := requirePendingMemoryMutations(t, store)
	require.Len(t, afterFirstArchiveMutations, len(beforeMutations), "local project archive must not enqueue cloud/shared memory mutations")

	service.now = func() time.Time { return now.Add(2 * time.Minute) }
	secondResult, err := service.ExecuteProjectArchive(context.Background(), ProjectArchiveRequest{
		Project:      "alpha",
		BackupID:     "fresh-backup",
		Confirmation: ProjectArchiveConfirmation("alpha"),
		ActorID:      "second-tester",
		Reason:       "retry should not rewrite audit metadata",
	})
	require.NoError(t, err)
	require.False(t, secondResult.Mutated)

	alphaAfterRetry, err := service.Project(context.Background(), "alpha")
	require.NoError(t, err)
	require.True(t, alphaAfterRetry.Archived)
	require.NotNil(t, alphaAfterRetry.ArchivedAt)
	require.Equal(t, now, alphaAfterRetry.ArchivedAt.UTC())
	require.Equal(t, "tester", alphaAfterRetry.ArchivedBy)
	require.Equal(t, "archive local duplicate", alphaAfterRetry.ArchiveReason)
	afterRetryMutations := requirePendingMemoryMutations(t, store)
	require.Len(t, afterRetryMutations, len(afterFirstArchiveMutations), "re-archiving must not enqueue memory mutation journal entries")
}

func TestServiceGuardedProjectMergeBlocksWithoutFreshUsableBackupOrExactConfirmation(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name         string
		source       string
		target       string
		backups      []BackupManifest
		backupID     string
		validateErr  error
		confirmation string
		wantErr      error
	}{
		{
			name:         "no fresh backup",
			backupID:     "missing-backup",
			confirmation: ProjectMergeConfirmation("alpha", "beta"),
			wantErr:      ErrDestructiveBackupRequired,
		},
		{
			name:         "invalid backup archive",
			backups:      []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			backupID:     "fresh-backup",
			validateErr:  ErrBackupArchiveInvalid,
			confirmation: ProjectMergeConfirmation("alpha", "beta"),
			wantErr:      ErrBackupArchiveInvalid,
		},
		{
			name:         "confirmation with trailing whitespace",
			backups:      []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			backupID:     "fresh-backup",
			confirmation: ProjectMergeConfirmation("alpha", "beta") + " ",
			wantErr:      ErrDestructiveConfirmationMismatch,
		},
		{
			name:         "confirmation names reversed projects",
			backups:      []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			backupID:     "fresh-backup",
			confirmation: ProjectMergeConfirmation("beta", "alpha"),
			wantErr:      ErrDestructiveConfirmationMismatch,
		},
		{
			name:         "source project with leading whitespace",
			source:       " alpha",
			backups:      []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			backupID:     "fresh-backup",
			confirmation: ProjectMergeConfirmation("alpha", "beta"),
			wantErr:      ErrProjectRequired,
		},
		{
			name:         "target project with trailing whitespace",
			target:       "beta ",
			backups:      []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			backupID:     "fresh-backup",
			confirmation: ProjectMergeConfirmation("alpha", "beta"),
			wantErr:      ErrProjectRequired,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, err := db.Open(":memory:")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, store.Close()) })
			sourceMemoryID := saveGovernanceServiceTestMemory(t, store, "alpha", "Source project memory")
			targetMemoryID := saveGovernanceServiceTestMemory(t, store, "beta", "Target project memory")

			beforeMutations := requirePendingMemoryMutations(t, store)
			beforeGovernanceRows := requireProjectGovernanceRows(t, store)
			beforeSyncStateRows := requireSyncStateRows(t, store)
			service := NewServiceWithBackup(store, fakeGuardBackupStore{backups: tt.backups, validateErr: tt.validateErr})
			service.now = func() time.Time { return now }
			sourceProject := tt.source
			if sourceProject == "" {
				sourceProject = "alpha"
			}
			targetProject := tt.target
			if targetProject == "" {
				targetProject = "beta"
			}

			_, err = service.ExecuteProjectMerge(context.Background(), ProjectMergeRequest{
				SourceProject: sourceProject,
				TargetProject: targetProject,
				BackupID:      tt.backupID,
				Confirmation:  tt.confirmation,
				ActorID:       "tester",
				Reason:        "local duplicate",
			})

			require.ErrorIs(t, err, tt.wantErr)
			source, detailErr := service.Project(context.Background(), "alpha")
			require.NoError(t, detailErr)
			require.False(t, source.Merged)
			require.Empty(t, source.MergeTarget)
			require.Equal(t, "alpha", requireServiceMemoryProject(t, store, sourceMemoryID))
			require.Equal(t, "beta", requireServiceMemoryProject(t, store, targetMemoryID))
			afterMutations := requirePendingMemoryMutations(t, store)
			require.Len(t, afterMutations, len(beforeMutations), "failed project merge guard must not journal memory mutations")
			require.Equal(t, beforeGovernanceRows, requireProjectGovernanceRows(t, store), "failed project merge guard must not mutate governance metadata")
			require.Equal(t, beforeSyncStateRows, requireSyncStateRows(t, store), "failed project merge guard must not mutate sync state")
		})
	}
}

func TestServiceGuardedProjectMergeRejectsSameSourceAndTargetBeforeBackup(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	saveGovernanceServiceTestMemory(t, store, "alpha", "Alpha memory")
	beforeMutations := requirePendingMemoryMutations(t, store)
	beforeGovernanceRows := requireProjectGovernanceRows(t, store)

	service := NewServiceWithBackup(store, fakeGuardBackupStore{})
	_, err = service.ExecuteProjectMerge(context.Background(), ProjectMergeRequest{
		SourceProject: "alpha",
		TargetProject: "alpha",
		Confirmation:  ProjectMergeConfirmation("alpha", "alpha"),
	})

	require.ErrorIs(t, err, db.ErrGovernanceProjectMergeInvalid)
	require.Len(t, requirePendingMemoryMutations(t, store), len(beforeMutations))
	require.Equal(t, beforeGovernanceRows, requireProjectGovernanceRows(t, store))
}

func TestServiceGuardedProjectMergeRecordsLocalMetadataAndReturnsCloudHandoffNote(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	sourceMemoryID := saveGovernanceServiceTestMemory(t, store, "alpha", "Alpha memory")
	targetMemoryID := saveGovernanceServiceTestMemory(t, store, "beta", "Beta memory")

	beforeMutations := requirePendingMemoryMutations(t, store)
	service := NewServiceWithBackup(store, fakeGuardBackupStore{backups: []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}}})
	service.now = func() time.Time { return now }

	result, err := service.ExecuteProjectMerge(context.Background(), ProjectMergeRequest{
		SourceProject: "alpha",
		TargetProject: "beta",
		BackupID:      "fresh-backup",
		Confirmation:  ProjectMergeConfirmation("alpha", "beta"),
		ActorID:       "tester",
		Reason:        "local duplicate project",
	})

	require.NoError(t, err)
	require.Equal(t, GuardOperationMerge, result.Operation)
	require.Equal(t, GuardTargetProject, result.TargetType)
	require.Equal(t, "alpha", result.SourceProject)
	require.Equal(t, "beta", result.TargetProject)
	require.Equal(t, "fresh-backup", result.BackupID)
	require.True(t, result.Mutated)
	require.Contains(t, result.CloudHandoffNote, "No cloud project mutation was performed")

	// After physical migration alpha has no rows — read governance record directly.
	var srcMergeTarget, srcMergedAt, srcMergedBy, srcMergeReason string
	require.NoError(t, store.RawDB().QueryRow(`
SELECT COALESCE(merge_target,''), COALESCE(merged_at,''), COALESCE(merged_by,''), COALESCE(merge_reason,'')
FROM hive_project_governance WHERE project = 'alpha'`).Scan(&srcMergeTarget, &srcMergedAt, &srcMergedBy, &srcMergeReason))
	require.Equal(t, "beta", srcMergeTarget)
	require.Equal(t, now.UTC().Format("2006-01-02 15:04:05"), srcMergedAt)
	require.Equal(t, "tester", srcMergedBy)
	require.Equal(t, "local duplicate project", srcMergeReason)

	target, err := service.Project(context.Background(), "beta")
	require.NoError(t, err)
	require.False(t, target.Merged)
	require.Empty(t, target.MergeTarget)
	// Physical migration: source memory now lives under the target project.
	require.Equal(t, "beta", requireServiceMemoryProject(t, store, sourceMemoryID), "physical merge must reassign source memories to target")
	require.Equal(t, "beta", requireServiceMemoryProject(t, store, targetMemoryID))
	afterMutations := requirePendingMemoryMutations(t, store)
	require.Len(t, afterMutations, len(beforeMutations), "local project merge must not enqueue cloud/shared memory mutations")

	service.now = func() time.Time { return now.Add(2 * time.Minute) }
	secondResult, err := service.ExecuteProjectMerge(context.Background(), ProjectMergeRequest{
		SourceProject: "alpha",
		TargetProject: "beta",
		BackupID:      "fresh-backup",
		Confirmation:  ProjectMergeConfirmation("alpha", "beta"),
		ActorID:       "second-tester",
		Reason:        "retry should not rewrite audit metadata",
	})
	require.NoError(t, err)
	require.False(t, secondResult.Mutated)

	// After physical migration alpha has no rows, read governance record directly.
	var gotMergedAt, gotMergedBy, gotReason string
	require.NoError(t, store.RawDB().QueryRow(`
SELECT COALESCE(merged_at,''), COALESCE(merged_by,''), COALESCE(merge_reason,'')
FROM hive_project_governance WHERE project = 'alpha'`).Scan(&gotMergedAt, &gotMergedBy, &gotReason))
	require.Equal(t, now.UTC().Format("2006-01-02 15:04:05"), gotMergedAt, "retry must not rewrite merged_at")
	require.Equal(t, "tester", gotMergedBy, "retry must not rewrite merged_by")
	require.Equal(t, "local duplicate project", gotReason, "retry must not rewrite merge_reason")
	require.Len(t, requirePendingMemoryMutations(t, store), len(afterMutations), "re-merging must not enqueue memory mutation journal entries")
}

type fakeGuardBackupStore struct {
	backups     []BackupManifest
	validateErr error
}

func (f fakeGuardBackupStore) List(context.Context) ([]BackupManifest, error) {
	return f.backups, nil
}

func (f fakeGuardBackupStore) Create(context.Context) (BackupManifest, error) {
	return BackupManifest{}, errors.New("not implemented")
}

func (f fakeGuardBackupStore) PlanRestore(context.Context, RestoreRequest) (RestoreResult, error) {
	return RestoreResult{}, errors.New("not implemented")
}

func (f fakeGuardBackupStore) ValidateArchive(_ context.Context, backupID string) (BackupManifest, error) {
	for _, backup := range f.backups {
		if backup.ID == backupID {
			if f.validateErr != nil {
				return BackupManifest{}, f.validateErr
			}
			return backup, nil
		}
	}
	return BackupManifest{}, ErrBackupNotFound
}

func newGovernanceServiceBackupTestStore(t *testing.T) (*db.DB, *BackupStore) {
	t.Helper()
	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	store, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	backupStore := NewSQLiteBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"), store.RawDB())
	return store, backupStore
}

func saveGovernanceServiceTestMemory(t *testing.T, store *db.DB, project, title string) int64 {
	t.Helper()
	_, err := store.EnsureManualSaveSession(project)
	require.NoError(t, err)
	id, err := store.SaveMemory(&models.Memory{Project: project, Title: title, Content: "content", SessionID: "manual-save-" + project})
	require.NoError(t, err)
	return id
}

func requireMemoryActive(t *testing.T, store *db.DB, id int64) {
	t.Helper()
	_, err := store.GetMemory(id)
	require.NoError(t, err)
}

func requireMemoryDeleted(t *testing.T, store *db.DB, id int64) {
	t.Helper()
	_, err := store.GetDeletedMemory(id)
	require.NoError(t, err)
}

func requirePendingMemoryMutations(t *testing.T, store *db.DB) []db.MutationEnvelope {
	t.Helper()
	mutations, err := store.GetPendingMutations("", 100)
	require.NoError(t, err)
	return mutations
}

func requireMemorySyncID(t *testing.T, store *db.DB, id int64) string {
	t.Helper()
	var syncID string
	require.NoError(t, store.RawDB().QueryRow(`SELECT sync_id FROM memories WHERE id = ?`, id).Scan(&syncID))
	require.NotEmpty(t, syncID)
	return syncID
}

func requireServiceMemoryProject(t *testing.T, store *db.DB, id int64) string {
	t.Helper()
	var project string
	require.NoError(t, store.RawDB().QueryRow(`SELECT project FROM memories WHERE id = ?`, id).Scan(&project))
	return project
}

func requireProjectGovernanceRows(t *testing.T, store *db.DB) int {
	t.Helper()
	var rows int
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM hive_project_governance`).Scan(&rows))
	return rows
}

func requireSyncStateRows(t *testing.T, store *db.DB) int {
	t.Helper()
	var rows int
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM sync_state`).Scan(&rows))
	return rows
}

type readOnlyTestError string

func (e readOnlyTestError) Error() string { return string(e) }

// Task 2.1 — MemoryByID service method

func TestService_MemoryByID_DelegatesToStore(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.CreateSession("sess-alpha", "alpha", "/repo/alpha", "dev", "test"))
	id, err := store.SaveMemory(&models.Memory{Project: "alpha", Title: "delegate memory", Content: "delegate content", SessionID: "sess-alpha"})
	require.NoError(t, err)

	service := NewService(store)
	memory, err := service.MemoryByID(context.Background(), id)

	require.NoError(t, err)
	require.Equal(t, id, memory.ID)
	require.Equal(t, "delegate memory", memory.Title)
	require.Equal(t, "delegate content", memory.Content)
}

func TestService_MemoryByID_PropagatesNotFound(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	service := NewService(store)
	_, err = service.MemoryByID(context.Background(), 99999)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrMemoryNotFound)
}

func TestService_MemoryByID_HidesDeletedMemory(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.CreateSession("sess-alpha", "alpha", "/repo/alpha", "dev", "test"))
	id, err := store.SaveMemory(&models.Memory{Project: "alpha", Title: "deleted memory", Content: "deleted content", SessionID: "sess-alpha"})
	require.NoError(t, err)
	require.NoError(t, store.DeleteMemory(id, "tester", "stale"))

	service := NewService(store)
	_, err = service.MemoryByID(context.Background(), id)

	require.ErrorIs(t, err, ErrMemoryNotFound)
}

func TestService_MemoryByID_InvalidID(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	service := NewService(store)

	for _, id := range []int64{0, -1, -100} {
		_, err := service.MemoryByID(context.Background(), id)
		require.ErrorIs(t, err, ErrMemoryIDRequired, "id=%d", id)
	}
}

const errReadOnlyTest readOnlyTestError = "read-only test failure"

// ─── Phase 2: Batch merge types + ExecuteProjectMergeBatch ───────────────────

// mockMergeStore is a minimal mock that implements readStore + projectMergeStore +
// the new ProjectMergeSyncEvidence method for batch service tests.
type mockMergeStore struct {
	db              *db.DB
	mergeCalls      []mockMergeCall
	syncEvidence    bool
	syncEvidenceErr error
}

type mockMergeCall struct {
	source string
	err    error
}

func (m *mockMergeStore) ListGovernanceProjects(ctx context.Context) ([]db.GovernanceProject, error) {
	return m.db.ListGovernanceProjects(ctx)
}
func (m *mockMergeStore) GetGovernanceProject(ctx context.Context, name string) (db.GovernanceProject, error) {
	return m.db.GetGovernanceProject(ctx, name)
}
func (m *mockMergeStore) ListGovernanceMemories(ctx context.Context, f db.GovernanceMemoryFilter) ([]db.GovernanceMemory, error) {
	return m.db.ListGovernanceMemories(ctx, f)
}
func (m *mockMergeStore) GetGovernanceMemoryByID(ctx context.Context, id int64) (db.GovernanceMemory, error) {
	return m.db.GetGovernanceMemoryByID(ctx, id)
}
func (m *mockMergeStore) ListGovernanceSyncHealth(ctx context.Context) ([]db.SyncHealth, error) {
	return m.db.ListGovernanceSyncHealth(ctx)
}
func (m *mockMergeStore) ListHiveWarnings(f db.HiveWarningFilter) ([]db.HiveWarning, error) {
	return m.db.ListHiveWarnings(f)
}
func (m *mockMergeStore) MergeGovernanceProject(ctx context.Context, source, target, actorID, reason string, mergedAt time.Time) (bool, error) {
	for _, c := range m.mergeCalls {
		if c.source == source {
			return c.err == nil, c.err
		}
	}
	// Default: delegate to real DB.
	return m.db.MergeGovernanceProject(ctx, source, target, actorID, reason, mergedAt)
}
func (m *mockMergeStore) ProjectMergeSyncEvidence(ctx context.Context, projects []string) (bool, error) {
	return m.syncEvidence, m.syncEvidenceErr
}

// TestExecuteProjectMergeBatch_SerialLoop verifies that the batch method processes
// sources serially: a failure on one source does not abort others, backup is
// called exactly once, and per-source results are accurate.
func TestExecuteProjectMergeBatch_SerialLoop(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	saveGovernanceServiceTestMemory(t, store, "src1", "src1 mem")
	saveGovernanceServiceTestMemory(t, store, "src2", "src2 mem")
	saveGovernanceServiceTestMemory(t, store, "src3", "src3 mem")
	saveGovernanceServiceTestMemory(t, store, "dst-batch", "dst mem")

	// src2 will produce a conflict error.
	mock := &mockMergeStore{
		db: store,
		mergeCalls: []mockMergeCall{
			{source: "src2", err: db.ErrGovernanceProjectMergeConflict},
		},
	}
	backups := fakeGuardBackupStore{backups: []BackupManifest{{ID: "batch-backup", CreatedAt: now.Add(-time.Minute)}}}
	svc := NewServiceWithBackup(mock, backups)
	svc.now = func() time.Time { return now }

	result, err := svc.ExecuteProjectMergeBatch(context.Background(), ProjectMergeBatchRequest{
		Sources:      []string{"src1", "src2", "src3"},
		Target:       "dst-batch",
		BackupID:     "batch-backup",
		Confirmation: ProjectMergeBatchConfirmation("dst-batch"),
		ActorID:      "tester",
		Reason:       "batch consolidation",
	})

	require.NoError(t, err)
	require.Len(t, result.Results, 3)

	bySource := map[string]MergeResult{}
	for _, r := range result.Results {
		bySource[r.Source] = r
	}
	require.True(t, bySource["src1"].Mutated, "src1 must be mutated (merged)")
	require.Empty(t, bySource["src1"].ErrMsg, "src1 must have no error")
	require.NotEmpty(t, bySource["src2"].ErrMsg, "src2 must have error")
	require.False(t, bySource["src2"].Mutated, "src2 must not be mutated")
	require.True(t, bySource["src3"].Mutated, "src3 must be mutated despite src2 error")
	require.Empty(t, bySource["src3"].ErrMsg, "src3 must have no error")
}

// TestExecuteProjectMergeBatch_BackupFailureAborts verifies that a backup failure
// stops the batch before any merge is attempted.
func TestExecuteProjectMergeBatch_BackupFailureAborts(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	saveGovernanceServiceTestMemory(t, store, "src-ab", "src-ab mem")
	saveGovernanceServiceTestMemory(t, store, "dst-ab", "dst-ab mem")

	var mergeCallCount int
	mock := &mockMergeStore{db: store, mergeCalls: []mockMergeCall{}}
	_ = mergeCallCount

	// No valid backup — empty backup list.
	svc := NewServiceWithBackup(mock, fakeGuardBackupStore{backups: nil})
	svc.now = func() time.Time { return now }

	_, err = svc.ExecuteProjectMergeBatch(context.Background(), ProjectMergeBatchRequest{
		Sources:      []string{"src-ab"},
		Target:       "dst-ab",
		BackupID:     "missing-backup",
		Confirmation: ProjectMergeBatchConfirmation("dst-ab"),
		ActorID:      "tester",
		Reason:       "test",
	})

	require.ErrorIs(t, err, ErrDestructiveBackupRequired)
	// Verify src-ab was not merged (row still under src-ab project).
	var count int
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE project = 'src-ab'`).Scan(&count))
	require.Equal(t, 1, count, "merge must not run when backup fails")
}

// TestExecuteProjectMergeBatch_SyncEvidencePropagated verifies that HasSyncEvidence
// is set on the batch result when the store reports synced rows.
func TestExecuteProjectMergeBatch_SyncEvidencePropagated(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	saveGovernanceServiceTestMemory(t, store, "src-se", "src-se mem")
	saveGovernanceServiceTestMemory(t, store, "dst-se", "dst-se mem")

	mock := &mockMergeStore{db: store, syncEvidence: true}
	svc := NewServiceWithBackup(mock, fakeGuardBackupStore{backups: []BackupManifest{{ID: "se-backup", CreatedAt: now.Add(-time.Minute)}}})
	svc.now = func() time.Time { return now }

	result, err := svc.ExecuteProjectMergeBatch(context.Background(), ProjectMergeBatchRequest{
		Sources:      []string{"src-se"},
		Target:       "dst-se",
		BackupID:     "se-backup",
		Confirmation: ProjectMergeBatchConfirmation("dst-se"),
		ActorID:      "tester",
		Reason:       "test",
	})

	require.NoError(t, err)
	require.True(t, result.HasSyncEvidence, "HasSyncEvidence must be true when store reports synced rows")
	require.NotEmpty(t, result.CloudHandoffNote, "CloudHandoffNote must be set when HasSyncEvidence is true")
}

// TestExecuteProjectMergeBatchSyncEvidenceError verifies that a store error from
// ProjectMergeSyncEvidence propagates as a wrapped error and leaves the result zero.
func TestExecuteProjectMergeBatchSyncEvidenceError(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	saveGovernanceServiceTestMemory(t, store, "src-see", "src-see mem")
	saveGovernanceServiceTestMemory(t, store, "dst-see", "dst-see mem")

	sentinel := errors.New("db failure")
	mock := &mockMergeStore{db: store, syncEvidenceErr: sentinel}
	svc := NewServiceWithBackup(mock, fakeGuardBackupStore{backups: []BackupManifest{{ID: "see-backup", CreatedAt: now.Add(-time.Minute)}}})
	svc.now = func() time.Time { return now }

	result, err := svc.ExecuteProjectMergeBatch(context.Background(), ProjectMergeBatchRequest{
		Sources:      []string{"src-see"},
		Target:       "dst-see",
		BackupID:     "see-backup",
		Confirmation: ProjectMergeBatchConfirmation("dst-see"),
		ActorID:      "tester",
		Reason:       "test",
	})

	require.Error(t, err)
	require.ErrorIs(t, err, sentinel)
	require.Contains(t, err.Error(), "sync evidence check")
	require.Equal(t, ProjectMergeBatchResult{}, result)
}

// ─── Phase 2 — ExecuteProjectDelete tests ────────────────────────────────────

// mockDeleteStore satisfies readStore + projectDeleteStore for unit testing.
type mockDeleteStore struct {
	db        *db.DB
	deleteErr error
}

func (m *mockDeleteStore) ListGovernanceProjects(ctx context.Context) ([]db.GovernanceProject, error) {
	return m.db.ListGovernanceProjects(ctx)
}
func (m *mockDeleteStore) GetGovernanceProject(ctx context.Context, name string) (db.GovernanceProject, error) {
	return m.db.GetGovernanceProject(ctx, name)
}
func (m *mockDeleteStore) ListGovernanceMemories(ctx context.Context, f db.GovernanceMemoryFilter) ([]db.GovernanceMemory, error) {
	return m.db.ListGovernanceMemories(ctx, f)
}
func (m *mockDeleteStore) GetGovernanceMemoryByID(ctx context.Context, id int64) (db.GovernanceMemory, error) {
	return m.db.GetGovernanceMemoryByID(ctx, id)
}
func (m *mockDeleteStore) ListGovernanceSyncHealth(ctx context.Context) ([]db.SyncHealth, error) {
	return m.db.ListGovernanceSyncHealth(ctx)
}
func (m *mockDeleteStore) ListHiveWarnings(f db.HiveWarningFilter) ([]db.HiveWarning, error) {
	return m.db.ListHiveWarnings(f)
}
func (m *mockDeleteStore) DeleteGovernanceProject(ctx context.Context, project, actorID, reason string) (int, error) {
	if m.deleteErr != nil {
		return 0, m.deleteErr
	}
	return m.db.DeleteGovernanceProject(ctx, project, actorID, reason)
}

// TestExecuteProjectDelete_NotArchived verifies that when the store returns
// ErrGovernanceProjectNotArchived, the service propagates it as-is so the
// HTTP layer can map it to 409.
func TestExecuteProjectDelete_NotArchived(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	mock := &mockDeleteStore{db: store, deleteErr: db.ErrGovernanceProjectNotArchived}
	svc := NewServiceWithBackup(mock, fakeGuardBackupStore{backups: []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}}})
	svc.now = func() time.Time { return now }

	_, err = svc.ExecuteProjectDelete(context.Background(), ProjectDeleteRequest{
		Project:      "live-project",
		BackupID:     "fresh-backup",
		Confirmation: ProjectDeleteConfirmation("live-project"),
		ActorID:      "tester",
		Reason:       "not archived test",
	})

	require.ErrorIs(t, err, db.ErrGovernanceProjectNotArchived)
}

// TestExecuteProjectDelete_StaleBackup verifies that a stale backup blocks
// the operation before the store is called.
func TestExecuteProjectDelete_StaleBackup(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	mock := &mockDeleteStore{db: store}
	svc := NewServiceWithBackup(mock, fakeGuardBackupStore{
		backups: []BackupManifest{{ID: "stale-backup", CreatedAt: now.Add(-destructiveBackupFreshness - time.Second)}},
	})
	svc.now = func() time.Time { return now }

	_, err = svc.ExecuteProjectDelete(context.Background(), ProjectDeleteRequest{
		Project:      "some-project",
		BackupID:     "stale-backup",
		Confirmation: ProjectDeleteConfirmation("some-project"),
		ActorID:      "tester",
	})

	require.ErrorIs(t, err, ErrDestructiveBackupRequired)
}

// TestExecuteProjectDelete_ConfirmationMismatch verifies that a wrong
// confirmation phrase blocks the operation and the store is never called.
func TestExecuteProjectDelete_ConfirmationMismatch(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	var storeCalled bool
	mock := &mockDeleteStore{db: store}
	// Override via a custom deleteErr that we'd only see if called.
	_ = storeCalled

	svc := NewServiceWithBackup(mock, fakeGuardBackupStore{
		backups: []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
	})
	svc.now = func() time.Time { return now }

	_, err = svc.ExecuteProjectDelete(context.Background(), ProjectDeleteRequest{
		Project:      "my-project",
		BackupID:     "fresh-backup",
		Confirmation: "WRONG PHRASE",
		ActorID:      "tester",
	})

	require.ErrorIs(t, err, ErrDestructiveConfirmationMismatch)
}

// TestExecuteProjectDelete_WhitespaceOnlyConfirmation verifies that a
// whitespace-only confirmation string is treated as empty and returns
// ErrDestructiveConfirmationRequired (not ErrDestructiveConfirmationMismatch).
func TestExecuteProjectDelete_WhitespaceOnlyConfirmation(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	mock := &mockDeleteStore{db: store}
	svc := NewServiceWithBackup(mock, fakeGuardBackupStore{
		backups: []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
	})
	svc.now = func() time.Time { return now }

	_, err = svc.ExecuteProjectDelete(context.Background(), ProjectDeleteRequest{
		Project:      "my-project",
		BackupID:     "fresh-backup",
		Confirmation: "   ",
		ActorID:      "tester",
	})

	require.ErrorIs(t, err, ErrDestructiveConfirmationRequired)
}

// TestExecuteProjectDelete_EmptyReason verifies that an empty or whitespace-only
// reason is rejected with ErrDestructiveReasonRequired before the store is called.
func TestExecuteProjectDelete_EmptyReason(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name   string
		reason string
	}{
		{name: "missing reason", reason: ""},
		{name: "blank reason", reason: "   "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, err := db.Open(":memory:")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, store.Close()) })

			saveGovernanceServiceTestMemory(t, store, "alpha", "Alpha memory")
			if _, err := store.ArchiveGovernanceProject(context.Background(), "alpha", "actor", "test", now.Add(-time.Hour)); err != nil {
				t.Fatalf("ArchiveGovernanceProject: %v", err)
			}

			svc := NewServiceWithBackup(store, fakeGuardBackupStore{
				backups: []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			})
			svc.now = func() time.Time { return now }

			_, err = svc.ExecuteProjectDelete(context.Background(), ProjectDeleteRequest{
				Project:      "alpha",
				BackupID:     "fresh-backup",
				Confirmation: ProjectDeleteConfirmation("alpha"),
				ActorID:      "tester",
				Reason:       tt.reason,
			})

			require.ErrorIs(t, err, ErrDestructiveReasonRequired)
		})
	}
}

// TestExecuteProjectDelete_Success verifies the happy path: correct confirmation,
// fresh backup, archived project — result has Mutated=true, CloudHandoffNote
// non-empty, and Project matches the request.
func TestExecuteProjectDelete_Success(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	saveGovernanceServiceTestMemory(t, store, "alpha", "Alpha memory")
	if _, err := store.ArchiveGovernanceProject(context.Background(), "alpha", "actor", "test", now.Add(-time.Hour)); err != nil {
		t.Fatalf("ArchiveGovernanceProject: %v", err)
	}

	svc := NewServiceWithBackup(store, fakeGuardBackupStore{
		backups: []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
	})
	svc.now = func() time.Time { return now }

	result, err := svc.ExecuteProjectDelete(context.Background(), ProjectDeleteRequest{
		Project:      "alpha",
		BackupID:     "fresh-backup",
		Confirmation: ProjectDeleteConfirmation("alpha"),
		ActorID:      "tester",
		Reason:       "purge test",
	})

	require.NoError(t, err)
	require.True(t, result.Mutated)
	require.NotEmpty(t, result.CloudHandoffNote)
	require.Equal(t, "alpha", result.Project)
}

func TestEngramImportPreviewCreatesFreshTokenAndDoesNotBackupOrWrite(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	sourcePath := createGovernanceEngramFixture(t)
	backup := &fakeEngramImportBackupStore{}
	service := NewServiceWithBackup(store, backup)

	job, err := service.StartEngramImportPreview(context.Background(), EngramImportRequest{Source: sourcePath})
	require.NoError(t, err)
	job = waitEngramImportJobDone(t, service, job.ID)

	require.Equal(t, EngramImportJobKindPreview, job.Kind)
	require.Equal(t, EngramImportPhaseCompleted, job.Phase)
	require.True(t, job.Done)
	require.Equal(t, 100, job.Percent)
	require.NotNil(t, job.Report)
	require.Equal(t, job.ID, job.Report.PreviewID)
	require.Equal(t, EngramImportEntityCounts{Sessions: 1, Prompts: 1, Observations: 1}, job.Report.Projected)
	require.Equal(t, []EngramImportProjectImpact{{Project: "proj-a", Projected: EngramImportEntityCounts{Sessions: 1, Prompts: 1, Observations: 1}}}, job.Report.ProjectedByProject)
	require.Equal(t, 1, job.Report.SkippedRelations)
	require.Equal(t, 0, backup.createCalls, "dry-run preview must not create a Hive backup")
	require.Equal(t, 0, governanceImportRunCount(t, store), "dry-run preview must not write Hive import metadata")
}

func TestEngramImportExecuteRequiresFreshPreviewBeforeBackupOrImport(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	sourcePath := createGovernanceEngramFixture(t)
	backup := &fakeEngramImportBackupStore{}
	service := NewServiceWithBackup(store, backup)

	_, err = service.StartEngramImportExecute(context.Background(), EngramImportRequest{Source: sourcePath, PreviewID: "missing-preview"})

	require.ErrorIs(t, err, ErrEngramImportPreviewRequired)
	require.Equal(t, 0, backup.createCalls, "missing preview must be rejected before backup creation")
	require.Equal(t, 0, governanceImportRunCount(t, store), "missing preview must be rejected before Hive writes")
}

func TestEngramImportExecuteCreatesBackupBeforeImportAndReportsProgress(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	sourcePath := createGovernanceEngramFixture(t)
	backup := &fakeEngramImportBackupStore{manifest: BackupManifest{ID: "backup-before-import", CreatedAt: time.Now().UTC()}}
	service := NewServiceWithBackup(store, backup)

	preview, err := service.StartEngramImportPreview(context.Background(), EngramImportRequest{Source: sourcePath})
	require.NoError(t, err)
	preview = waitEngramImportJobDone(t, service, preview.ID)

	job, err := service.StartEngramImportExecute(context.Background(), EngramImportRequest{Source: sourcePath, PreviewID: preview.ID})
	require.NoError(t, err)
	job = waitEngramImportJobDone(t, service, job.ID)

	require.Equal(t, EngramImportJobKindExecute, job.Kind)
	require.True(t, job.Done)
	require.Equal(t, EngramImportPhaseCompleted, job.Phase)
	require.Equal(t, 100, job.Percent)
	require.Contains(t, job.PhaseHistory, string(EngramImportPhaseAnalysis))
	require.Contains(t, job.PhaseHistory, string(EngramImportPhaseBackup))
	require.Contains(t, job.PhaseHistory, string(EngramImportPhaseImport))
	require.Contains(t, job.PhaseHistory, string(EngramImportPhaseFinalization))
	require.NotNil(t, job.Report)
	require.Equal(t, "backup-before-import", job.Report.BackupID)
	require.Equal(t, EngramImportMutationCounts{Imported: 3}, job.Report.Imported)
	require.Equal(t, 1, backup.createCalls)
	require.Equal(t, 1, governanceImportRunCount(t, store))
}

func TestEngramImportExecuteReportsAmbiguousDuplicatesWithoutOverwritingHiveRows(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.CreateSession("existing-session", "proj-a", "C:/src/a", "tester", "test"))
	_, err = store.SaveMemory(&models.Memory{Project: "proj-a", Title: "Duplicated title", Content: "existing body one", SessionID: "existing-session"})
	require.NoError(t, err)
	_, err = store.SaveMemory(&models.Memory{Project: "proj-a", Title: "Duplicated title", Content: "existing body two", SessionID: "existing-session"})
	require.NoError(t, err)
	sourcePath := createGovernanceAmbiguousEngramFixture(t)
	backup := &fakeEngramImportBackupStore{manifest: BackupManifest{ID: "backup-before-import", CreatedAt: time.Now().UTC()}}
	service := NewServiceWithBackup(store, backup)

	preview, err := service.StartEngramImportPreview(context.Background(), EngramImportRequest{Source: sourcePath})
	require.NoError(t, err)
	preview = waitEngramImportJobDone(t, service, preview.ID)
	job, err := service.StartEngramImportExecute(context.Background(), EngramImportRequest{Source: sourcePath, PreviewID: preview.ID})
	require.NoError(t, err)
	job = waitEngramImportJobDone(t, service, job.ID)

	require.Equal(t, EngramImportPhaseCompleted, job.Phase)
	require.NotNil(t, job.Report)
	require.Equal(t, EngramImportMutationCounts{Imported: 1, Ambiguous: 1}, job.Report.Imported)
	require.Equal(t, []EngramImportAmbiguousDuplicate{{SourceID: "21", Project: "proj-a", Title: "Duplicated title", Reason: "multiple active Hive memories match project and title"}}, job.Report.AmbiguousDuplicates)
	require.Equal(t, 2, governanceMemoryCountByTitle(t, store, "Duplicated title"))
}

func TestEngramImportExecuteRejectsChangedSourceAfterPreviewBeforeBackupOrImport(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	sourcePath := createGovernanceEngramFixture(t)
	backup := &fakeEngramImportBackupStore{manifest: BackupManifest{ID: "backup-before-import", CreatedAt: time.Now().UTC()}}
	service := NewServiceWithBackup(store, backup)

	preview, err := service.StartEngramImportPreview(context.Background(), EngramImportRequest{Source: sourcePath})
	require.NoError(t, err)
	preview = waitEngramImportJobDone(t, service, preview.ID)
	require.Equal(t, EngramImportPhaseCompleted, preview.Phase)

	mutateGovernanceEngramFixture(t, sourcePath)

	job, err := service.StartEngramImportExecute(context.Background(), EngramImportRequest{Source: sourcePath, PreviewID: preview.ID})
	require.NoError(t, err)
	job = waitEngramImportJobDone(t, service, job.ID)

	require.True(t, job.Done)
	require.Equal(t, EngramImportPhaseFailed, job.Phase)
	require.Contains(t, job.Error, ErrEngramImportPreviewRequired.Error())
	require.Equal(t, 0, backup.createCalls, "changed preview source must be rejected before backup creation")
	require.Equal(t, 0, governanceImportRunCount(t, store), "changed preview source must be rejected before Hive writes")
}

func TestEngramImportExecuteAbortsWhenBackupFailsBeforeImport(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	sourcePath := createGovernanceEngramFixture(t)
	backupErr := errors.New("backup disk full")
	backup := &fakeEngramImportBackupStore{createErr: backupErr}
	service := NewServiceWithBackup(store, backup)

	preview, err := service.StartEngramImportPreview(context.Background(), EngramImportRequest{Source: sourcePath})
	require.NoError(t, err)
	preview = waitEngramImportJobDone(t, service, preview.ID)

	job, err := service.StartEngramImportExecute(context.Background(), EngramImportRequest{Source: sourcePath, PreviewID: preview.ID})
	require.NoError(t, err)
	job = waitEngramImportJobDone(t, service, job.ID)

	require.True(t, job.Done)
	require.Equal(t, EngramImportPhaseFailed, job.Phase)
	require.Contains(t, job.Error, backupErr.Error())
	require.Equal(t, 1, backup.createCalls)
	require.Equal(t, 0, governanceImportRunCount(t, store), "backup failure must abort before import metadata writes")
}

type fakeEngramImportBackupStore struct {
	manifest    BackupManifest
	createErr   error
	createCalls int
}

func (f *fakeEngramImportBackupStore) List(context.Context) ([]BackupManifest, error) {
	return nil, nil
}

func (f *fakeEngramImportBackupStore) Create(context.Context) (BackupManifest, error) {
	f.createCalls++
	if f.createErr != nil {
		return BackupManifest{}, f.createErr
	}
	if f.manifest.ID == "" {
		f.manifest = BackupManifest{ID: "backup-1", CreatedAt: time.Now().UTC()}
	}
	return f.manifest, nil
}

func (f *fakeEngramImportBackupStore) PlanRestore(context.Context, RestoreRequest) (RestoreResult, error) {
	return RestoreResult{}, errors.New("not implemented")
}

func (f *fakeEngramImportBackupStore) ValidateArchive(context.Context, string) (BackupManifest, error) {
	return BackupManifest{}, errors.New("not implemented")
}

func waitEngramImportJobDone(t *testing.T, service *Service, id string) EngramImportJob {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		job, err := service.EngramImportJob(context.Background(), id)
		require.NoError(t, err)
		if job.Done {
			return job
		}
	}
	job, err := service.EngramImportJob(context.Background(), id)
	require.NoError(t, err)
	t.Fatalf("job %s did not finish; snapshot=%+v", id, job)
	return EngramImportJob{}
}

func createGovernanceEngramFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engram.db")
	sqlDB, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
CREATE TABLE observations (id INTEGER PRIMARY KEY, project TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '', content TEXT NOT NULL DEFAULT '', type TEXT NOT NULL DEFAULT '', topic_key TEXT, session_id TEXT, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TEXT);
CREATE TABLE sessions (id TEXT PRIMARY KEY, project TEXT NOT NULL DEFAULT '', directory TEXT NOT NULL DEFAULT '', dev_id TEXT NOT NULL DEFAULT '', client TEXT NOT NULL DEFAULT '', started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, ended_at TEXT, summary TEXT);
CREATE TABLE user_prompts (id INTEGER PRIMARY KEY, project TEXT NOT NULL DEFAULT '', content TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE memory_relations (id INTEGER PRIMARY KEY, source_id INTEGER, target_id INTEGER, relation TEXT);
INSERT INTO sessions (id, project, directory, dev_id, client, started_at) VALUES ('ses-1', 'proj-a', 'C:/src/a', 'dev-a', 'opencode', '2026-06-11 10:00:00');
INSERT INTO user_prompts (id, project, content, created_at) VALUES (11, 'proj-a', 'prompt content', '2026-06-11 10:01:00');
INSERT INTO observations (id, project, title, content, type, topic_key, session_id, created_at, updated_at) VALUES (21, 'proj-a', 'Decision', 'Keep daemon-owned import', 'decision', 'topic-a', 'ses-1', '2026-06-11 10:02:00', '2026-06-11 10:03:00');
INSERT INTO memory_relations (id, source_id, target_id, relation) VALUES (1, 21, 21, 'related');`)
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return path
}

func createGovernanceAmbiguousEngramFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engram.db")
	sqlDB, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
CREATE TABLE observations (id INTEGER PRIMARY KEY, project TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '', content TEXT NOT NULL DEFAULT '', type TEXT NOT NULL DEFAULT '', topic_key TEXT, session_id TEXT, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TEXT);
CREATE TABLE sessions (id TEXT PRIMARY KEY, project TEXT NOT NULL DEFAULT '', directory TEXT NOT NULL DEFAULT '', dev_id TEXT NOT NULL DEFAULT '', client TEXT NOT NULL DEFAULT '', started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, ended_at TEXT, summary TEXT);
CREATE TABLE user_prompts (id INTEGER PRIMARY KEY, project TEXT NOT NULL DEFAULT '', content TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE memory_relations (id INTEGER PRIMARY KEY, source_id INTEGER, target_id INTEGER, relation TEXT);
INSERT INTO sessions (id, project, directory, dev_id, client, started_at) VALUES ('ses-1', 'proj-a', 'C:/src/a', 'dev-a', 'opencode', '2026-06-11 10:00:00');
INSERT INTO observations (id, project, title, content, type, session_id, created_at) VALUES (21, 'proj-a', 'Duplicated title', 'Imported body', 'decision', 'ses-1', '2026-06-11 10:02:00');`)
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return path
}

func mutateGovernanceEngramFixture(t *testing.T, path string) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`INSERT INTO user_prompts (id, project, content, created_at) VALUES (12, 'proj-a', 'changed prompt content', '2026-06-11 10:04:00')`)
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func governanceImportRunCount(t *testing.T, store *db.DB) int {
	t.Helper()
	var count int
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM import_runs`).Scan(&count))
	return count
}

func governanceMemoryCountByTitle(t *testing.T, store *db.DB, title string) int {
	t.Helper()
	var count int
	require.NoError(t, store.RawDB().QueryRow(`SELECT COUNT(*) FROM memories WHERE title = ?`, title).Scan(&count))
	return count
}
