package governance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/require"
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
			wantErr:      ErrDestructiveConfirmationMismatch,
		},
		{
			name:         "target project with trailing whitespace",
			target:       "beta ",
			backups:      []BackupManifest{{ID: "fresh-backup", CreatedAt: now.Add(-time.Minute)}},
			backupID:     "fresh-backup",
			confirmation: ProjectMergeConfirmation("alpha", "beta"),
			wantErr:      ErrDestructiveConfirmationMismatch,
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

	source, err := service.Project(context.Background(), "alpha")
	require.NoError(t, err)
	require.True(t, source.Merged)
	require.Equal(t, "beta", source.MergeTarget)
	require.NotNil(t, source.MergedAt)
	require.Equal(t, now, source.MergedAt.UTC())
	require.Equal(t, "tester", source.MergedBy)
	require.Equal(t, "local duplicate project", source.MergeReason)

	target, err := service.Project(context.Background(), "beta")
	require.NoError(t, err)
	require.False(t, target.Merged)
	require.Empty(t, target.MergeTarget)
	require.Equal(t, "alpha", requireServiceMemoryProject(t, store, sourceMemoryID), "local project merge metadata must not reassign memories in this slice")
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

	sourceAfterRetry, err := service.Project(context.Background(), "alpha")
	require.NoError(t, err)
	require.Equal(t, now, sourceAfterRetry.MergedAt.UTC())
	require.Equal(t, "tester", sourceAfterRetry.MergedBy)
	require.Equal(t, "local duplicate project", sourceAfterRetry.MergeReason)
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
