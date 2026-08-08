package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	hivemcp "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/mcp"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	os.Exit(run())
}

func run() int {
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbPath := dbFilePath()
	if restored, err := governance.ExecuteScheduledRestore(rootCtx, dbPath); err != nil {
		logger.Log.Printf("pending migration restore: %v", err)
	} else if restored {
		logger.Log.Printf("restored pending migration backup before opening database")
	}

	store, err := db.Open(dbPath)
	if err != nil {
		logger.Log.Fatalf("open database: %v", err)
	}
	defer func() { _ = store.Close() }()

	logger.Log.Printf("database: %s", dbPath)

	recoveryTTL, err := project.ParseRecoveryTokenTTL(os.Getenv("HIVE_RECOVERY_TOKEN_TTL"))
	if err != nil {
		logger.Log.Fatalf("invalid HIVE_RECOVERY_TOKEN_TTL: %v", err)
	}
	project.SetDefaultRecoveryTokenTTL(recoveryTTL)

	// Sync is optional. Invalid or malformed config is recorded as a persistent
	// warning and startup continues in local-only mode.
	var syncer hivemcp.SyncRunner
	cfg := applyStartupSyncConfig(store, hivesync.LoadWithStatus)
	if cfg != nil {
		syncer = hivesync.New(cfg, store)
		logger.Log.Printf("sync habilitado → %s", cfg.APIURL)
	} else {
		logger.Log.Printf("sync desactivado (define HIVE_API_URL/HIVE_API_EMAIL/HIVE_API_PASSWORD o crea ~/.jarvis/sync.json)")
	}
	gate := runStartupMigration(rootCtx, store, dbPath)
	if err := gate.Check(); err != nil {
		logger.Log.Printf("project identity migration: %v", err)
	}

	httpDone := make(chan struct{})
	go func() {
		defer close(httpDone)
		backupStore := governance.NewSQLiteBackupStore(dbPath, "", store.RawDB())
		govSvc := governance.NewServiceWithBackup(store, backupStore)
		configSvc := httpapi.NewSyncServiceAdapter(hivesync.NewService())
		healthSvc := httpapi.NewHealthServiceAdapter(hivesync.NewHealthService(store, nil))
		srv := httpapi.NewServerWithAll(httpAddr(), store, store, govSvc, configSvc, healthSvc, store)
		srv.SetMigrationGate(gate)
		srv.SetMigrationRetry(stop)
		srv.SetMigrationRestore(func(_ context.Context, req governance.RestoreRequest) error {
			return governance.ScheduleRestore(dbPath, req)
		})
		srv.SetMigrationIdentityResolver(newMigrationIdentityResolver(store, gate))
		if err := srv.Start(rootCtx); err != nil {
			logger.Log.Printf("http server stopped: %v (mcp continues)", err)
		}
	}()

	closed, err := runStartup(store)
	if err != nil {
		logger.Log.Fatalf("startup: %v", err)
	}
	if closed > 0 {
		logger.Log.Printf("auto-closed %d stale session(s)", closed)
	}

	server := hivemcp.NewServerWithMigrationGate(store, store, syncer, cfg, store, gate)

	runErr := server.Run(rootCtx, &sdkmcp.StdioTransport{})
	stop()

	// Always wait for HTTP goroutine before closing DB or exiting.
	<-httpDone

	if isCleanServerShutdown(rootCtx, runErr) {
		return 0
	}
	if runErr != nil {
		logger.Log.Printf("server stopped: %v", runErr)
		return 1
	}
	return 0
}

func isCleanServerShutdown(ctx context.Context, runErr error) bool {
	return ctx.Err() != nil && errors.Is(runErr, context.Canceled)
}

// httpAddr returns the address for the HTTP server, preferring HIVE_HTTP_PORT env var
// (default 7438).
func httpAddr() string {
	port := os.Getenv("HIVE_HTTP_PORT")
	if strings.TrimSpace(port) == "" {
		return "127.0.0.1:7438"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		logger.Log.Fatalf("invalid HIVE_HTTP_PORT %q: must be a number between 1 and 65535", port)
	}
	return "127.0.0.1:" + port
}

// runStartup runs pre-server initialization hooks and returns the number of
// stale sessions that were auto-closed. Extracted for testability.
func runStartup(store *db.DB) (int64, error) {
	return store.AutoCloseStale(24*time.Hour, time.Now)
}

// newMigrationIdentityResolver binds resolution to the migration plan the
// operator was shown. The preflight-conflict path never mutates the database and
// therefore never creates a rollback archive, so a backup id can never authorize
// resolution there; the plan fingerprint is the invariant the guard wants.
func newMigrationIdentityResolver(store *db.DB, gate *project.MigrationGate) func(context.Context, project.IdentityResolutionRequest) error {
	return func(ctx context.Context, req project.IdentityResolutionRequest) error {
		status := gate.Status()
		if status.PlanFingerprint == "" || req.PlanFingerprint != status.PlanFingerprint {
			return project.ErrIdentityResolutionStale
		}
		return store.ResolveProjectIdentityConflict(ctx, req.SourceProject, req.TargetProject)
	}
}

func runStartupMigration(ctx context.Context, store *db.DB, dbPath string) *project.MigrationGate {
	backups := governance.NewSQLiteBackupStore(dbPath, "", store.RawDB())
	preexisting := existingBackupIDs(ctx, backups)
	return runStartupMigrationWithBackup(ctx, store, func(ctx context.Context, plan db.ProjectMigrationPlan) error {
		return governance.ExecuteProjectMigrationWithBackup(ctx, store, plan, backups)
	}, func() string {
		return newestMigrationBackupID(ctx, backups, preexisting)
	})
}

// existingBackupIDs snapshots the backups present before migration runs so a
// later block can only report a backup this migration itself created.
func existingBackupIDs(ctx context.Context, backups *governance.BackupStore) map[string]struct{} {
	existing := make(map[string]struct{})
	created, err := backups.List(ctx)
	if err != nil {
		return existing
	}
	for _, backup := range created {
		existing[backup.ID] = struct{}{}
	}
	return existing
}

// newestMigrationBackupID returns the newest backup absent from the pre-migration
// snapshot, or empty when this migration created none.
func newestMigrationBackupID(ctx context.Context, backups *governance.BackupStore, preexisting map[string]struct{}) string {
	created, err := backups.List(ctx)
	if err != nil {
		return ""
	}
	for _, backup := range created {
		if _, existed := preexisting[backup.ID]; !existed {
			return backup.ID
		}
	}
	return ""
}

func runStartupMigrationWith(ctx context.Context, store *db.DB, execute func(context.Context, db.ProjectMigrationPlan) error) *project.MigrationGate {
	return runStartupMigrationWithBackup(ctx, store, execute, nil)
}

func runStartupMigrationWithBackup(ctx context.Context, store *db.DB, execute func(context.Context, db.ProjectMigrationPlan) error, backupID func() string) *project.MigrationGate {
	plan, err := db.ReadProjectMigrationPlan(ctx, store)
	if err == nil && !plan.Executable {
		err = db.ErrProjectMigrationPlanUnsafe
	}
	if err == nil {
		err = execute(ctx, plan)
	}
	if err == nil {
		return project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateReady})
	}
	status := project.MigrationStatus{
		State:           project.MigrationStateBlocked,
		Reason:          err.Error(),
		Continuation:    "hive project identity status",
		PlanFingerprint: plan.Fingerprint,
	}
	if backupID != nil {
		status.BackupID = backupID()
	}
	if encoded, marshalErr := json.Marshal(status); marshalErr == nil {
		if _, warningErr := store.SaveHiveWarning(db.HiveWarningInput{
			Severity: "error",
			Source:   "startup/project-identity-migration",
			Message:  string(encoded),
		}); warningErr != nil {
			logger.Log.Printf("could not persist project migration block: %v", warningErr)
		}
	}
	return project.NewMigrationGate(status)
}

// applyStartupSyncConfig loads sync configuration. On error it records a
// persistent warning and returns nil so startup continues in local-only mode.
// In-memory warnings from LoadWithStatus (e.g. partial env fallback) are also
// persisted. The caller is responsible for creating a SyncRunner when cfg is non-nil.
func applyStartupSyncConfig(store *db.DB, load func() (*hivesync.Config, hivesync.SyncConfigStatus, error)) *hivesync.Config {
	cfg, status, err := load()
	if err != nil {
		logger.Log.Printf("sync config invalid, starting in local-only mode: %v", err)
		if _, warnErr := store.SaveHiveWarning(db.HiveWarningInput{
			Severity: "warning",
			Source:   "startup/sync-config",
			Message:  fmt.Sprintf("sync config invalid, sync disabled: %v", err),
		}); warnErr != nil {
			logger.Log.Printf("could not persist sync config warning: %v", warnErr)
		}
		return nil
	}
	for _, w := range status.Warnings {
		if _, warnErr := store.SaveHiveWarning(db.HiveWarningInput{
			Severity: "warning",
			Source:   "startup/sync-config",
			Message:  w,
		}); warnErr != nil {
			logger.Log.Printf("could not persist sync config warning: %v", warnErr)
		}
	}
	return cfg
}

// dbFilePath returns the SQLite path, preferring HIVE_DB_PATH env var
// (used in tests) over the default ~/.jarvis/memory.db.
func dbFilePath() string {
	if p := os.Getenv("HIVE_DB_PATH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		logger.Log.Fatalf("cannot determine home directory: %v", err)
	}
	dbDir := filepath.Join(home, ".jarvis")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		logger.Log.Fatalf("cannot create DB directory %q: %v", dbDir, err)
	}
	return filepath.Join(dbDir, "memory.db")
}
