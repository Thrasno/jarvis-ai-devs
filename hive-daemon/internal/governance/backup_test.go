package governance

import (
	"context"
	"database/sql"
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
