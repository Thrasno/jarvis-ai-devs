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

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/google/uuid"
)

const (
	backupArchiveFile  = "memory.db"
	backupManifestFile = "manifest.json"
)

var (
	ErrBackupStoreRequired        = errors.New("backup store is not configured")
	ErrBackupIDRequired           = errors.New("backup id is required")
	ErrBackupConfirmationRequired = errors.New("backup restore confirmation is required")
	ErrBackupConfirmationMismatch = errors.New("backup restore confirmation mismatch")
	ErrBackupNotFound             = errors.New("backup not found")
	ErrBackupIDUnsafe             = errors.New("backup id is unsafe")
	ErrBackupLocationUnsafe       = errors.New("backup root must be outside the live database directory")
	ErrBackupArchiveInvalid       = errors.New("backup archive integrity check failed")
)

const RestoreStatusCoordinationRequired = "coordination_required"

const (
	ProjectMigrationBackupOperation = "canonical-project-identity-migration"
	ProjectMigrationBackupRetention = 24 * time.Hour
)

type BackupManifest struct {
	ID              string    `json:"id"`
	CreatedAt       time.Time `json:"created_at"`
	DBPath          string    `json:"db_path"`
	ArchivePath     string    `json:"archive_path"`
	ManifestPath    string    `json:"manifest_path"`
	Checksum        string    `json:"checksum"`
	SizeBytes       int64     `json:"size_bytes"`
	SourceOperation string    `json:"source_operation,omitempty"`
	Temporary       bool      `json:"temporary,omitempty"`
	RetainUntil     time.Time `json:"retain_until,omitempty"`
}

type RestoreRequest struct {
	BackupID     string `json:"backup_id"`
	Confirmation string `json:"confirmation"`
}

type RestoreResult struct {
	BackupID              string    `json:"backup_id"`
	DBPath                string    `json:"db_path"`
	RestoredAt            time.Time `json:"restored_at,omitempty"`
	ArchivePath           string    `json:"archive_path"`
	Status                string    `json:"status,omitempty"`
	RequiresDaemonRestart bool      `json:"requires_daemon_restart,omitempty"`
	Message               string    `json:"message,omitempty"`
}

type BackupStore struct {
	dbPath     string
	backupRoot string
	now        func() time.Time
	snapshot   backupSnapshotter
	rename     func(string, string) error
	remove     func(string) error
}

type backupSnapshotter func(context.Context, string, string) (string, int64, error)

func NewBackupStore(dbPath, backupRoot string) *BackupStore {
	if strings.TrimSpace(backupRoot) == "" {
		backupRoot = DefaultBackupRoot(dbPath)
	}
	return &BackupStore{dbPath: dbPath, backupRoot: backupRoot, now: time.Now, snapshot: copyFileSnapshot, rename: os.Rename, remove: os.Remove}
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

func RestoreConfirmation(backupID string) string {
	return "RESTORE " + strings.TrimSpace(backupID)
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
	if err := persistBackupManifest(manifest); err != nil {
		return BackupManifest{}, err
	}
	cleanupDir = false
	return manifest, nil
}

// ExecuteProjectMigrationWithBackup composes the migration executor with the
// daemon's SQLite-safe backup store. Failed migrations retain their new backup;
// only expired temporary migration backups are pruned before a later attempt.
func ExecuteProjectMigrationWithBackup(ctx context.Context, database *db.DB, plan db.ProjectMigrationPlan, backups *BackupStore) error {
	return executeProjectMigrationWithBackup(ctx, database, plan, backups, nil)
}

func executeProjectMigrationWithBackup(ctx context.Context, database *db.DB, plan db.ProjectMigrationPlan, backups *BackupStore, failpoint func() error) error {
	if backups == nil {
		return ErrBackupStoreRequired
	}
	if err := backups.PruneExpiredTemporaryMigrationBackups(ctx, backups.now().UTC()); err != nil {
		return fmt.Errorf("prune expired migration backups: %w", err)
	}
	return db.ExecuteProjectMigration(ctx, database, plan, func(ctx context.Context) error {
		_, err := backups.CreateTemporaryMigrationBackup(ctx)
		return err
	}, failpoint)
}

func (s *BackupStore) CreateTemporaryMigrationBackup(ctx context.Context) (BackupManifest, error) {
	backup, err := s.Create(ctx)
	if err != nil {
		return BackupManifest{}, err
	}
	backup.SourceOperation = ProjectMigrationBackupOperation
	backup.Temporary = true
	backup.RetainUntil = backup.CreatedAt.Add(ProjectMigrationBackupRetention)
	if err := persistBackupManifest(backup); err != nil {
		return BackupManifest{}, err
	}
	return backup, nil
}

func (s *BackupStore) PruneExpiredTemporaryMigrationBackups(ctx context.Context, now time.Time) error {
	backups, err := s.List(ctx)
	if err != nil {
		return err
	}
	for _, backup := range backups {
		if !backup.Temporary || backup.SourceOperation != ProjectMigrationBackupOperation || backup.RetainUntil.IsZero() || backup.RetainUntil.After(now) {
			continue
		}
		if err := os.RemoveAll(filepath.Dir(backup.ManifestPath)); err != nil {
			return fmt.Errorf("remove expired migration backup %s: %w", backup.ID, err)
		}
	}
	return nil
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

func (s *BackupStore) PlanRestore(ctx context.Context, req RestoreRequest) (RestoreResult, error) {
	backup, expectedArchivePath, err := s.validateRestore(ctx, req)
	if err != nil {
		return RestoreResult{}, err
	}
	if err := s.validateArchive(ctx, backup, expectedArchivePath); err != nil {
		return RestoreResult{}, err
	}
	return RestoreResult{
		BackupID:              backup.ID,
		DBPath:                s.dbPath,
		ArchivePath:           expectedArchivePath,
		Status:                RestoreStatusCoordinationRequired,
		RequiresDaemonRestart: true,
		Message:               "restore archive validated; stop/restart daemon coordination is required before replacing the live database",
	}, nil
}

func (s *BackupStore) ValidateArchive(ctx context.Context, backupID string) (BackupManifest, error) {
	if err := ctx.Err(); err != nil {
		return BackupManifest{}, err
	}
	id := strings.TrimSpace(backupID)
	if id == "" {
		return BackupManifest{}, ErrBackupIDRequired
	}
	if !isSafeBackupID(id) {
		return BackupManifest{}, fmt.Errorf("%w: %s", ErrBackupIDUnsafe, id)
	}
	backup, err := s.backup(id)
	if err != nil {
		return BackupManifest{}, err
	}
	backupDir := filepath.Join(s.backupRoot, id)
	expectedArchivePath := filepath.Join(backupDir, backupArchiveFile)
	if err := verifyManifestArchivePath(backup.ArchivePath, expectedArchivePath); err != nil {
		return BackupManifest{}, err
	}
	if err := s.validateArchive(ctx, backup, expectedArchivePath); err != nil {
		return BackupManifest{}, err
	}
	return backup, nil
}

func (s *BackupStore) Restore(ctx context.Context, req RestoreRequest) (RestoreResult, error) {
	if err := ctx.Err(); err != nil {
		return RestoreResult{}, err
	}
	backup, expectedArchivePath, err := s.validateRestore(ctx, req)
	if err != nil {
		return RestoreResult{}, err
	}
	stagedPath, checksum, size, err := stageRestoreArchive(expectedArchivePath, filepath.Dir(s.dbPath))
	if err != nil {
		return RestoreResult{}, fmt.Errorf("restore db backup: %w", err)
	}
	cleanupStage := true
	defer func() {
		if cleanupStage {
			_ = os.Remove(stagedPath)
		}
	}()
	if err := verifyArchiveIntegrity(backup, checksum, size); err != nil {
		return RestoreResult{}, err
	}
	quarantinedSidecars, err := s.quarantineSQLiteSidecars()
	if err != nil {
		return RestoreResult{}, err
	}
	if err := s.rename(stagedPath, s.dbPath); err != nil {
		restoreSQLiteSidecars(s.rename, quarantinedSidecars)
		return RestoreResult{}, fmt.Errorf("replace live db with staged restore: %w", err)
	}
	cleanupStage = false
	removeQuarantinedSQLiteSidecars(s.remove, quarantinedSidecars)
	return RestoreResult{BackupID: backup.ID, DBPath: s.dbPath, RestoredAt: s.now().UTC(), ArchivePath: expectedArchivePath}, nil
}

func (s *BackupStore) validateRestore(ctx context.Context, req RestoreRequest) (BackupManifest, string, error) {
	if err := ctx.Err(); err != nil {
		return BackupManifest{}, "", err
	}
	id := strings.TrimSpace(req.BackupID)
	confirmation := strings.TrimSpace(req.Confirmation)
	if id == "" {
		return BackupManifest{}, "", ErrBackupIDRequired
	}
	if !isSafeBackupID(id) {
		return BackupManifest{}, "", fmt.Errorf("%w: %s", ErrBackupIDUnsafe, id)
	}
	if confirmation == "" {
		return BackupManifest{}, "", ErrBackupConfirmationRequired
	}
	if confirmation != RestoreConfirmation(id) {
		return BackupManifest{}, "", ErrBackupConfirmationMismatch
	}
	backup, err := s.backup(id)
	if err != nil {
		return BackupManifest{}, "", err
	}
	backupDir := filepath.Join(s.backupRoot, id)
	expectedArchivePath := filepath.Join(backupDir, backupArchiveFile)
	if err := verifyManifestArchivePath(backup.ArchivePath, expectedArchivePath); err != nil {
		return BackupManifest{}, "", err
	}
	return backup, expectedArchivePath, nil
}

func (s *BackupStore) validateArchive(ctx context.Context, backup BackupManifest, archivePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	checksum, size, err := checksumFileWithSHA256(archivePath)
	if err != nil {
		return fmt.Errorf("validate backup archive: %w", err)
	}
	return verifyArchiveIntegrity(backup, checksum, size)
}

func (s *BackupStore) backup(id string) (BackupManifest, error) {
	if !isSafeBackupID(id) {
		return BackupManifest{}, fmt.Errorf("%w: %s", ErrBackupIDUnsafe, id)
	}
	path := filepath.Join(s.backupRoot, id, backupManifestFile)
	backup, err := readBackupManifest(path)
	if errors.Is(err, os.ErrNotExist) {
		return BackupManifest{}, fmt.Errorf("%w: %s", ErrBackupNotFound, id)
	}
	return backup, err
}

func isSafeBackupID(id string) bool {
	if id == "" || id == "." || id == ".." || filepath.IsAbs(id) || strings.ContainsAny(id, `/\`) {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
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

func persistBackupManifest(backup BackupManifest) error {
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return fmt.Errorf("encode backup manifest: %w", err)
	}
	if err := os.WriteFile(backup.ManifestPath, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write backup manifest: %w", err)
	}
	return nil
}

func verifyManifestArchivePath(manifestArchivePath, expectedArchivePath string) error {
	if filepath.Clean(manifestArchivePath) != filepath.Clean(expectedArchivePath) {
		return fmt.Errorf("%w: archive path does not match selected backup", ErrBackupArchiveInvalid)
	}
	return nil
}

func stageRestoreArchive(archivePath, liveDBDir string) (string, string, int64, error) {
	staged, err := os.CreateTemp(liveDBDir, ".memory.db.restore-*")
	if err != nil {
		return "", "", 0, fmt.Errorf("create staged restore file: %w", err)
	}
	stagedPath := staged.Name()
	if err := staged.Close(); err != nil {
		_ = os.Remove(stagedPath)
		return "", "", 0, fmt.Errorf("close staged restore file: %w", err)
	}
	checksum, size, err := copyFileWithSHA256(archivePath, stagedPath)
	if err != nil {
		_ = os.Remove(stagedPath)
		if errors.Is(err, os.ErrNotExist) {
			return "", "", 0, fmt.Errorf("%w: archive is missing", ErrBackupArchiveInvalid)
		}
		return "", "", 0, err
	}
	return stagedPath, checksum, size, nil
}

func verifyArchiveIntegrity(backup BackupManifest, checksum string, size int64) error {
	manifestChecksum := strings.TrimSpace(backup.Checksum)
	if manifestChecksum == "" {
		return fmt.Errorf("%w: missing checksum", ErrBackupArchiveInvalid)
	}
	if !strings.EqualFold(checksum, manifestChecksum) {
		return fmt.Errorf("%w: checksum mismatch", ErrBackupArchiveInvalid)
	}
	if backup.SizeBytes >= 0 && size != backup.SizeBytes {
		return fmt.Errorf("%w: size mismatch", ErrBackupArchiveInvalid)
	}
	return nil
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
	if errors.Is(err, os.ErrNotExist) {
		return "", 0, fmt.Errorf("%w: archive is missing", ErrBackupArchiveInvalid)
	}
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

type quarantinedSQLiteSidecar struct {
	livePath       string
	quarantinePath string
}

func (s *BackupStore) quarantineSQLiteSidecars() ([]quarantinedSQLiteSidecar, error) {
	sidecars := make([]quarantinedSQLiteSidecar, 0, 2)
	for _, suffix := range []string{"-wal", "-shm"} {
		livePath := s.dbPath + suffix
		if _, err := os.Stat(livePath); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			restoreSQLiteSidecars(s.rename, sidecars)
			return nil, fmt.Errorf("inspect sqlite sidecar %s: %w", suffix, err)
		}

		quarantinePath := sqliteSidecarQuarantinePath(livePath)
		if err := s.rename(livePath, quarantinePath); err != nil {
			restoreSQLiteSidecars(s.rename, sidecars)
			return nil, fmt.Errorf("quarantine sqlite sidecar %s: %w", suffix, err)
		}
		sidecars = append(sidecars, quarantinedSQLiteSidecar{livePath: livePath, quarantinePath: quarantinePath})
	}
	return sidecars, nil
}

func sqliteSidecarQuarantinePath(livePath string) string {
	dir := filepath.Dir(livePath)
	name := filepath.Base(livePath)
	return filepath.Join(dir, "."+name+".restore-quarantine-"+uuid.NewString())
}

func restoreSQLiteSidecars(rename func(string, string) error, sidecars []quarantinedSQLiteSidecar) {
	for i := len(sidecars) - 1; i >= 0; i-- {
		_ = rename(sidecars[i].quarantinePath, sidecars[i].livePath)
	}
}

func removeQuarantinedSQLiteSidecars(remove func(string) error, sidecars []quarantinedSQLiteSidecar) {
	for _, sidecar := range sidecars {
		_ = remove(sidecar.quarantinePath)
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
