package governance

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestBackupStoreCreatesRestorableBackupOutsideDBDirectory(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("live hive db"), 0o600))

	backupRoot := filepath.Join(tempDir, "hive-backups")
	store := NewBackupStore(dbPath, backupRoot)

	backup, err := store.Create(context.Background())

	require.NoError(t, err)
	require.NotEmpty(t, backup.ID)
	require.Equal(t, dbPath, backup.DBPath)
	require.NotEmpty(t, backup.Checksum)
	require.Equal(t, int64(len("live hive db")), backup.SizeBytes)
	requirePathOutsideDir(t, dbDir, backup.ArchivePath)
	require.FileExists(t, backup.ArchivePath)
	require.FileExists(t, backup.ManifestPath)

	contents, err := os.ReadFile(backup.ArchivePath)
	require.NoError(t, err)
	require.Equal(t, []byte("live hive db"), contents)

	backups, err := store.List(context.Background())
	require.NoError(t, err)
	require.Len(t, backups, 1)
	require.Equal(t, backup.ID, backups[0].ID)
}

func TestBackupStorePlanRestoreValidatesArchiveWithoutStagingBesideLiveDB(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("planned backup"), 0o600))

	store := NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))
	backup, err := store.Create(context.Background())
	require.NoError(t, err)

	result, err := store.PlanRestore(context.Background(), RestoreRequest{BackupID: backup.ID, Confirmation: RestoreConfirmation(backup.ID)})

	require.NoError(t, err)
	require.Equal(t, RestoreStatusCoordinationRequired, result.Status)
	require.True(t, result.RequiresDaemonRestart)
	require.Equal(t, backup.ID, result.BackupID)
	require.Equal(t, backup.ArchivePath, result.ArchivePath)
	entries, err := os.ReadDir(dbDir)
	require.NoError(t, err)
	for _, entry := range entries {
		require.False(t, strings.HasPrefix(entry.Name(), ".memory.db.restore-"), "plan restore must not stage copies beside the live db")
	}
}

func TestBackupStorePlanRestoreRejectsInvalidSelectionAndConfirmation(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("before restore"), 0o600))

	store := NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))
	backup, err := store.Create(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dbPath, []byte("after corruption"), 0o600))

	_, err = store.PlanRestore(context.Background(), RestoreRequest{Confirmation: RestoreConfirmation(backup.ID)})
	require.ErrorIs(t, err, ErrBackupIDRequired)
	requireDBFileContent(t, dbPath, "after corruption")

	_, err = store.PlanRestore(context.Background(), RestoreRequest{BackupID: backup.ID, Confirmation: "RESTORE wrong-backup"})
	require.ErrorIs(t, err, ErrBackupConfirmationMismatch)
	requireDBFileContent(t, dbPath, "after corruption")
}

func TestBackupStorePlanRestoreRejectsUnsafeBackupIDs(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("current live db"), 0o600))

	store := NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))

	for _, tt := range []struct {
		name string
		id   string
	}{
		{name: "dot", id: "."},
		{name: "dot dot", id: ".."},
		{name: "parent traversal", id: "../x"},
		{name: "nested path", id: "a/b"},
		{name: "absolute path", id: filepath.Join(string(filepath.Separator), "tmp", "backup")},
		{name: "unsafe name", id: "backup id with spaces"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.PlanRestore(context.Background(), RestoreRequest{BackupID: tt.id, Confirmation: RestoreConfirmation(tt.id)})

			require.ErrorIs(t, err, ErrBackupIDUnsafe)
			requireDBFileContent(t, dbPath, "current live db")
		})
	}
}

func TestBackupStorePlanRestoreRejectsArchiveIntegrityFailures(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		mutate func(BackupManifest) BackupManifest
	}{
		{name: "checksum mismatch", mutate: func(backup BackupManifest) BackupManifest {
			require.NoError(t, os.WriteFile(backup.ArchivePath, []byte("tampered backup archive"), 0o600))
			return backup
		}},
		{name: "size mismatch", mutate: func(backup BackupManifest) BackupManifest {
			backup.SizeBytes++
			require.NoError(t, writeBackupManifest(backup))
			return backup
		}},
		{name: "tampered manifest archive path", mutate: func(backup BackupManifest) BackupManifest {
			backup.ArchivePath = filepath.Join(filepath.Dir(filepath.Dir(backup.ArchivePath)), "attacker.db")
			require.NoError(t, os.WriteFile(backup.ArchivePath, []byte("attacker data"), 0o600))
			require.NoError(t, writeBackupManifest(backup))
			return backup
		}},
		{name: "missing archive", mutate: func(backup BackupManifest) BackupManifest {
			require.NoError(t, os.Remove(backup.ArchivePath))
			return backup
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			dbDir := filepath.Join(tempDir, "live-db")
			require.NoError(t, os.MkdirAll(dbDir, 0o755))
			dbPath := filepath.Join(dbDir, "memory.db")
			require.NoError(t, os.WriteFile(dbPath, []byte("trusted backup source"), 0o600))

			store := NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))
			backup, err := store.Create(context.Background())
			require.NoError(t, err)
			backup = tt.mutate(backup)
			require.NoError(t, os.WriteFile(dbPath, []byte("current live db"), 0o600))

			_, err = store.PlanRestore(context.Background(), RestoreRequest{BackupID: backup.ID, Confirmation: RestoreConfirmation(backup.ID)})

			require.ErrorIs(t, err, ErrBackupArchiveInvalid)
			requireDBFileContent(t, dbPath, "current live db")
		})
	}
}

func TestBackupStoreRestoreRequiresExplicitSelectionAndConfirmation(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("before restore"), 0o600))

	store := NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))
	backup, err := store.Create(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dbPath, []byte("after corruption"), 0o600))

	_, err = store.Restore(context.Background(), RestoreRequest{Confirmation: RestoreConfirmation(backup.ID)})
	require.ErrorIs(t, err, ErrBackupIDRequired)
	requireDBFileContent(t, dbPath, "after corruption")

	_, err = store.Restore(context.Background(), RestoreRequest{BackupID: backup.ID, Confirmation: "RESTORE wrong-backup"})
	require.ErrorIs(t, err, ErrBackupConfirmationMismatch)
	requireDBFileContent(t, dbPath, "after corruption")

	restored, err := store.Restore(context.Background(), RestoreRequest{BackupID: backup.ID, Confirmation: RestoreConfirmation(backup.ID)})
	require.NoError(t, err)
	require.Equal(t, backup.ID, restored.BackupID)
	requireDBFileContent(t, dbPath, "before restore")
}

func TestBackupStoreRestoreRejectsArchiveIntegrityFailuresWithoutMutatingLiveDB(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		mutate func(BackupManifest) BackupManifest
	}{
		{name: "checksum mismatch", mutate: func(backup BackupManifest) BackupManifest {
			require.NoError(t, os.WriteFile(backup.ArchivePath, []byte("tampered backup archive"), 0o600))
			return backup
		}},
		{name: "size mismatch", mutate: func(backup BackupManifest) BackupManifest {
			backup.SizeBytes++
			require.NoError(t, writeBackupManifest(backup))
			return backup
		}},
		{name: "missing archive", mutate: func(backup BackupManifest) BackupManifest {
			require.NoError(t, os.Remove(backup.ArchivePath))
			return backup
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			dbDir := filepath.Join(tempDir, "live-db")
			require.NoError(t, os.MkdirAll(dbDir, 0o755))
			dbPath := filepath.Join(dbDir, "memory.db")
			require.NoError(t, os.WriteFile(dbPath, []byte("trusted backup source"), 0o600))
			store := NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))
			backup, err := store.Create(context.Background())
			require.NoError(t, err)
			backup = tt.mutate(backup)
			require.NoError(t, os.WriteFile(dbPath, []byte("current live db"), 0o600))

			_, err = store.Restore(context.Background(), RestoreRequest{BackupID: backup.ID, Confirmation: RestoreConfirmation(backup.ID)})

			require.Error(t, err)
			requireDBFileContent(t, dbPath, "current live db")
		})
	}
}

func TestBackupStoreRestoreRemovesSQLiteSidecarsAfterReplacingDB(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("backup source"), 0o600))

	store := NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))
	backup, err := store.Create(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dbPath, []byte("current live db"), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-wal", []byte("stale wal"), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-shm", []byte("stale shm"), 0o600))

	_, err = store.Restore(context.Background(), RestoreRequest{BackupID: backup.ID, Confirmation: RestoreConfirmation(backup.ID)})

	require.NoError(t, err)
	require.NoFileExists(t, dbPath+"-wal")
	require.NoFileExists(t, dbPath+"-shm")
	requireDBFileContent(t, dbPath, "backup source")
}

func TestBackupStoreRestorePreservesSQLiteSidecarsWhenReplaceFails(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("backup source"), 0o600))

	store := NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))
	backup, err := store.Create(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dbPath, []byte("current live db"), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-wal", []byte("committed wal"), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-shm", []byte("shared memory"), 0o600))
	store.rename = func(oldPath, newPath string) error {
		if newPath == dbPath {
			return os.ErrPermission
		}
		return os.Rename(oldPath, newPath)
	}

	_, err = store.Restore(context.Background(), RestoreRequest{BackupID: backup.ID, Confirmation: RestoreConfirmation(backup.ID)})

	require.Error(t, err)
	requireDBFileContent(t, dbPath, "current live db")
	requireDBFileContent(t, dbPath+"-wal", "committed wal")
	requireDBFileContent(t, dbPath+"-shm", "shared memory")
}

func TestBackupStoreRestorePreservesLiveDBAndSidecarsWhenSidecarMoveFails(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("backup source"), 0o600))

	store := NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))
	backup, err := store.Create(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dbPath, []byte("current live db"), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-wal", []byte("committed wal"), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-shm", []byte("shared memory"), 0o600))
	store.rename = func(oldPath, newPath string) error {
		if oldPath == dbPath+"-wal" {
			return os.ErrPermission
		}
		return os.Rename(oldPath, newPath)
	}

	_, err = store.Restore(context.Background(), RestoreRequest{BackupID: backup.ID, Confirmation: RestoreConfirmation(backup.ID)})

	require.Error(t, err)
	requireDBFileContent(t, dbPath, "current live db")
	requireDBFileContent(t, dbPath+"-wal", "committed wal")
	requireDBFileContent(t, dbPath+"-shm", "shared memory")
}

func TestBackupStoreRestoreDoesNotFailWhenQuarantinedSidecarCleanupFails(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("backup source"), 0o600))

	store := NewBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"))
	backup, err := store.Create(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dbPath, []byte("current live db"), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-wal", []byte("stale wal"), 0o600))
	require.NoError(t, os.WriteFile(dbPath+"-shm", []byte("stale shm"), 0o600))
	store.remove = func(string) error { return os.ErrPermission }

	_, err = store.Restore(context.Background(), RestoreRequest{BackupID: backup.ID, Confirmation: RestoreConfirmation(backup.ID)})

	require.NoError(t, err)
	require.NoFileExists(t, dbPath+"-wal")
	require.NoFileExists(t, dbPath+"-shm")
	requireDBFileContent(t, dbPath, "backup source")
}

func TestBackupStoreListSkipsPartialOrCorruptBackupEntries(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("listable backup"), 0o600))

	backupRoot := filepath.Join(tempDir, "hive-backups")
	store := NewBackupStore(dbPath, backupRoot)
	backup, err := store.Create(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(backupRoot, "partial-backup"), 0o700))
	corruptDir := filepath.Join(backupRoot, "corrupt-backup")
	require.NoError(t, os.MkdirAll(corruptDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(corruptDir, backupManifestFile), []byte("{"), 0o600))

	backups, err := store.List(context.Background())

	require.NoError(t, err)
	require.Len(t, backups, 1)
	require.Equal(t, backup.ID, backups[0].ID)
}

func TestBackupStoreCreateWithSQLiteSnapshotCapturesCommittedWALData(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping())
	_, err = sqlDB.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE memories (id INTEGER PRIMARY KEY, content TEXT NOT NULL); PRAGMA wal_checkpoint(TRUNCATE); INSERT INTO memories(content) VALUES ('committed wal content');`)
	require.NoError(t, err)
	require.FileExists(t, dbPath+"-wal")

	store := NewSQLiteBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"), sqlDB)
	backup, err := store.Create(context.Background())
	require.NoError(t, err)

	backupDB, err := sql.Open("sqlite", backup.ArchivePath)
	require.NoError(t, err)
	defer backupDB.Close()
	var got string
	require.NoError(t, backupDB.QueryRow(`SELECT content FROM memories WHERE id = 1`).Scan(&got))
	require.Equal(t, "committed wal content", got)
	require.NoError(t, sqlDB.Close())
}

func TestBackupStoreCreateWithSQLiteSnapshotSucceedsWhileReaderPreventsCheckpoint(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbDir := filepath.Join(tempDir, "live-db")
	require.NoError(t, os.MkdirAll(dbDir, 0o755))
	dbPath := filepath.Join(dbDir, "memory.db")
	writerDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer writerDB.Close()
	require.NoError(t, writerDB.Ping())
	_, err = writerDB.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE memories (id INTEGER PRIMARY KEY, content TEXT NOT NULL); INSERT INTO memories(content) VALUES ('before reader');`)
	require.NoError(t, err)

	readerDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer readerDB.Close()
	readerTx, err := readerDB.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	require.NoError(t, err)
	defer readerTx.Rollback()
	var snapshotContent string
	require.NoError(t, readerTx.QueryRow(`SELECT content FROM memories WHERE id = 1`).Scan(&snapshotContent))
	require.Equal(t, "before reader", snapshotContent)

	_, err = writerDB.Exec(`INSERT INTO memories(content) VALUES ('blocked checkpoint frame')`)
	require.NoError(t, err)
	require.FileExists(t, dbPath+"-wal")

	store := NewSQLiteBackupStore(dbPath, filepath.Join(tempDir, "hive-backups"), writerDB)
	backup, err := store.Create(context.Background())

	require.NoError(t, err)
	backupDB, err := sql.Open("sqlite", backup.ArchivePath)
	require.NoError(t, err)
	defer backupDB.Close()
	var count int
	require.NoError(t, backupDB.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&count))
	require.Equal(t, 2, count)
}

func TestEnsureBackupRootOutsideDBDirDoesNotTreatDotDotPrefixAsOutside(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "memory.db")

	require.ErrorIs(t, ensureBackupRootOutsideDBDir(dbPath, filepath.Join(tempDir, "db", "..backups")), ErrBackupLocationUnsafe)
	require.NoError(t, ensureBackupRootOutsideDBDir(dbPath, filepath.Join(tempDir, "..backups")))
}

func requirePathOutsideDir(t *testing.T, dir, path string) {
	t.Helper()
	rel, err := filepath.Rel(dir, path)
	require.NoError(t, err)
	if rel == "." || rel == "" || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
		t.Fatalf("expected %q to be outside %q; relative path is %q", path, dir, rel)
	}
}

func requireDBFileContent(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(contents))
}

func writeBackupManifest(backup BackupManifest) error {
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(backup.ManifestPath, append(data, '\n'), 0o600)
}
