package governance

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	backupArchiveFile  = "memory.db"
	backupManifestFile = "manifest.json"
)

var ErrBackupLocationUnsafe = errors.New("backup root must be outside the live database directory")

type BackupManifest struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	DBPath       string    `json:"db_path"`
	ArchivePath  string    `json:"archive_path"`
	ManifestPath string    `json:"manifest_path"`
	Checksum     string    `json:"checksum"`
	SizeBytes    int64     `json:"size_bytes"`
}

type BackupStore struct {
	dbPath     string
	backupRoot string
	now        func() time.Time
	snapshot   backupSnapshotter
}

type backupSnapshotter func(context.Context, string, string) (string, int64, error)

func NewBackupStore(dbPath, backupRoot string) *BackupStore {
	if strings.TrimSpace(backupRoot) == "" {
		backupRoot = DefaultBackupRoot(dbPath)
	}
	return &BackupStore{dbPath: dbPath, backupRoot: backupRoot, now: time.Now, snapshot: copyFileSnapshot}
}

func NewSQLiteBackupStore(dbPath, backupRoot string, sqlDB *sql.DB) *BackupStore {
	store := NewBackupStore(dbPath, backupRoot)
	store.snapshot = sqliteVacuumIntoSnapshot(sqlDB)
	return store
}

func DefaultBackupRoot(dbPath string) string {
	dbDir := filepath.Dir(dbPath)
	name := strings.TrimPrefix(filepath.Base(dbDir), ".")
	if name == "" || name == "." {
		name = "hive"
	}
	return filepath.Join(filepath.Dir(dbDir), "."+name+"-hive-backups")
}

func (s *BackupStore) Create(ctx context.Context) (BackupManifest, error) {
	if err := ctx.Err(); err != nil {
		return BackupManifest{}, err
	}
	if err := ensureBackupRootOutsideDBDir(s.dbPath, s.backupRoot); err != nil {
		return BackupManifest{}, err
	}
	if err := os.MkdirAll(s.backupRoot, 0o700); err != nil {
		return BackupManifest{}, fmt.Errorf("create backup root: %w", err)
	}

	createdAt := s.now().UTC()
	id := createdAt.Format("20060102T150405.000000000Z") + "-" + uuid.NewString()
	dir := filepath.Join(s.backupRoot, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return BackupManifest{}, fmt.Errorf("create backup dir: %w", err)
	}
	cleanupDir := true
	defer func() {
		if cleanupDir {
			_ = os.RemoveAll(dir)
		}
	}()

	archivePath := filepath.Join(dir, backupArchiveFile)
	checksum, size, err := s.snapshot(ctx, s.dbPath, archivePath)
	if err != nil {
		return BackupManifest{}, err
	}
	manifest := BackupManifest{
		ID:           id,
		CreatedAt:    createdAt,
		DBPath:       s.dbPath,
		ArchivePath:  archivePath,
		ManifestPath: filepath.Join(dir, backupManifestFile),
		Checksum:     checksum,
		SizeBytes:    size,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BackupManifest{}, fmt.Errorf("encode backup manifest: %w", err)
	}
	if err := os.WriteFile(manifest.ManifestPath, append(data, '\n'), 0o600); err != nil {
		return BackupManifest{}, fmt.Errorf("write backup manifest: %w", err)
	}
	cleanupDir = false
	return manifest, nil
}

func (s *BackupStore) List(ctx context.Context) ([]BackupManifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.backupRoot)
	if errors.Is(err, os.ErrNotExist) {
		return []BackupManifest{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	backups := make([]BackupManifest, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			backup, err := readBackupManifest(filepath.Join(s.backupRoot, entry.Name(), backupManifestFile))
			if err != nil {
				continue
			}
			backups = append(backups, backup)
		}
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].CreatedAt.After(backups[j].CreatedAt) })
	return backups, nil
}

func readBackupManifest(path string) (BackupManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return BackupManifest{}, fmt.Errorf("read backup manifest: %w", err)
	}
	var backup BackupManifest
	if err := json.Unmarshal(data, &backup); err != nil {
		return BackupManifest{}, fmt.Errorf("decode backup manifest: %w", err)
	}
	return backup, nil
}

func ensureBackupRootOutsideDBDir(dbPath, backupRoot string) error {
	dbDir, err := filepath.Abs(filepath.Dir(dbPath))
	if err != nil {
		return fmt.Errorf("resolve db dir: %w", err)
	}
	root, err := filepath.Abs(backupRoot)
	if err != nil {
		return fmt.Errorf("resolve backup root: %w", err)
	}
	rel, err := filepath.Rel(dbDir, root)
	if err != nil {
		return fmt.Errorf("compare backup root: %w", err)
	}
	outside := rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
	if rel == "." || !outside {
		return ErrBackupLocationUnsafe
	}
	return nil
}

func checksumFileWithSHA256(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open archive file: %w", err)
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	written, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, fmt.Errorf("read archive file: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func copyFileSnapshot(ctx context.Context, srcPath, dstPath string) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	return copyFileWithSHA256(srcPath, dstPath)
}

func sqliteVacuumIntoSnapshot(sqlDB *sql.DB) backupSnapshotter {
	return func(ctx context.Context, _ string, dstPath string) (string, int64, error) {
		if sqlDB == nil {
			return "", 0, errors.New("sqlite backup connection is not configured")
		}
		if err := ctx.Err(); err != nil {
			return "", 0, err
		}
		if err := os.Remove(dstPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", 0, fmt.Errorf("prepare sqlite backup destination: %w", err)
		}
		if _, err := sqlDB.ExecContext(ctx, `VACUUM main INTO ?`, dstPath); err != nil {
			return "", 0, fmt.Errorf("snapshot sqlite database: %w", err)
		}
		return checksumFileWithSHA256(dstPath)
	}
}

func copyFileWithSHA256(srcPath, dstPath string) (string, int64, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return "", 0, fmt.Errorf("open source file: %w", err)
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", 0, fmt.Errorf("create destination file: %w", err)
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(dst, hash), src)
	if err != nil {
		_ = dst.Close()
		return "", 0, fmt.Errorf("copy file: %w", err)
	}
	if err := dst.Close(); err != nil {
		return "", 0, fmt.Errorf("close destination file: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}
