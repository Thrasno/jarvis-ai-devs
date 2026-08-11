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
	restored, restoreErr := governance.ExecuteScheduledRestore(rootCtx, dbPath)
	if err := pendingRestoreStartupError(restored, restoreErr, dbPath); err != nil {
		logger.Log.Fatalf("pending migration restore: %v", err)
	}
	if restoreErr != nil {
		logger.Log.Printf("pending migration restore: %v", restoreErr)
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

// pendingRestoreStartupError reports the restore outcomes the daemon must not
// survive. A restore that replaced the live database but could not clear its own
// request would run again on the next start and discard everything this session
// writes; a local-first product must fail loudly there instead of serving. Every
// other failure left the live database untouched, so startup continues and the
// operator still sees the logged error.
//
// The stop can be permanent: if the completion marker could not be written at
// all for a lasting reason (a full or read-only ~/.jarvis), every following
// start replays the same restore and stops again. It can also be a single stop:
// when only the marker's durability flush failed, the rename already put the
// marker on disk and the next start short-circuits on it and serves normally.
// The message reports the replay as possible rather than certain, because an
// operator told to expect a permanent stop that never returns stops trusting
// the line that does mean it. Deleting the request file is correct either way,
// so this message names that absolute path and the step.
func pendingRestoreStartupError(restored bool, err error, dbPath string) error {
	if !restored || !errors.Is(err, governance.ErrPendingRestoreReplayable) {
		return nil
	}
	return fmt.Errorf(
		"%w; while this request is on disk a following start can replay this restore and stop here again, discarding whatever was written in between; to recover, stop the daemon and delete %s, then start it again",
		err,
		absolutePathForOperator(governance.PendingRestorePath(dbPath)),
	)
}

// absolutePathForOperator resolves a path the operator has to act on. HIVE_DB_PATH
// may be relative, and this instruction is read from a log rather than from the
// daemon's working directory.
func absolutePathForOperator(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
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
	return runStartupMigrationWithBackup(ctx, store, func(ctx context.Context, plan db.ProjectMigrationPlan) error {
		return governance.ExecuteProjectMigrationWithBackup(ctx, store, plan, backups)
	}, backups)
}

// migrationBackupIDForPlan returns the archive that rolls back the plan this
// block reports, or empty when the plan never reached a mutation and therefore
// never took one.
//
// The backup is identified by the plan it was taken for rather than by being new
// on disk. A blocked migration is re-attempted on every daemon start and reuses
// the archive it already took, so "created during this run" stops being true
// from the second start onward while the validated archive is still sitting
// there. Binding to the fingerprint also keeps BackupID and PlanFingerprint
// describing the same plan after a contention retry re-plans: an archive taken
// for a superseded plan no longer matches the live database and must not be
// offered as its rollback. Unrelated backups carry no migration fingerprint at
// all and stay unreportable.
func migrationBackupIDForPlan(ctx context.Context, backups *governance.BackupStore, planFingerprint string) string {
	backup, found, err := backups.MigrationBackupForPlan(ctx, planFingerprint)
	if err != nil || !found {
		return ""
	}
	return backup.ID
}

// preflightAndMaintainMigration looks before it writes, and writes only what
// needs no permission.
//
// Startup used to plan and execute in one breath, which meant a daemon start
// silently folded two spellings of a project into one. That is a decision about
// the operator's own names and it is not startup's to make, so a preflight that
// finds duplicate spellings — or a plan the planner refused — returns without
// executing and lets the caller install a gate.
//
// The rest of the executor's work is not a decision: populating the canonical
// identity registry is an idempotent INSERT ... ON CONFLICT DO NOTHING, and the
// schema-ownership rebuild only moves the same rows into tables that declare the
// foreign key. Neither folds an identity, so both still run unattended with the
// executor's own mandatory pre-mutation backup. Holding them for a confirmation
// would wedge Hive on a routine upgrade with nothing for the operator to decide.
func preflightAndMaintainMigration(ctx context.Context, store *db.DB, execute func(context.Context, db.ProjectMigrationPlan) error) (db.ProjectMigrationPreflight, error) {
	preflight, err := db.ReadProjectMigrationPreflight(ctx, store)
	if err != nil || preflight.NeedsOperatorReview() {
		return preflight, err
	}
	return preflight, execute(ctx, preflight.Plan)
}

// pendingProjectMigrationStatus describes a plan nobody has approved yet.
//
// BackupID is deliberately absent: this path never reached a mutation, so it
// never took an archive, and an unrelated older backup must never be offered as
// this plan's rollback. PlanFingerprint stays, because it is the guard the
// operator's resolution is checked against.
func pendingProjectMigrationStatus(preflight db.ProjectMigrationPreflight) project.MigrationStatus {
	reason := "project identity migration would fold duplicate project spellings and needs an explicit operator decision"
	if len(preflight.Plan.Conflicts) != 0 {
		reason = fmt.Sprintf(
			"project identity migration reports %d unresolved identity conflict(s) and needs an explicit operator decision",
			len(preflight.Plan.Conflicts))
	}
	return project.MigrationStatus{
		State:           project.MigrationStatePendingOperatorReview,
		Reason:          reason,
		PlanFingerprint: preflight.Plan.Fingerprint,
	}
}

func runStartupMigrationWith(ctx context.Context, store *db.DB, execute func(context.Context, db.ProjectMigrationPlan) error) *project.MigrationGate {
	return runStartupMigrationWithBackup(ctx, store, execute, nil)
}

// logProjectMigrationSummary reports a migration that actually moved rows.
//
// A successful migration used to be entirely silent, which is exactly what makes
// a slow one undiagnosable: this runs before the MCP transport is served, so an
// operator staring at a client that has not come up cannot tell a hung daemon
// from a working one. "migrated 5,200 rows in 2.1s" turns "did it hang?" into
// "it is working".
//
// A no-op stays quiet — every daemon start after the first one is a no-op, and a
// line per start about zero work trains an operator to ignore the line that
// matters. logger.Log, not stdout: this is an MCP stdio server.
func logProjectMigrationSummary(summary db.ProjectMigrationSummary, elapsed time.Duration) {
	if !summary.Ran {
		return
	}
	logger.Log.Printf(
		"project identity migration: rows rekeyed=%d, reprojects enqueued=%d, sessions re-queued=%d, prompts re-queued=%d in %s",
		summary.RowsRekeyed, summary.ReprojectsEnqueued, summary.SessionsRequeued, summary.PromptsRequeued,
		elapsed.Round(time.Millisecond))
}

func runStartupMigrationWithBackup(ctx context.Context, store *db.DB, execute func(context.Context, db.ProjectMigrationPlan) error, backups *governance.BackupStore) *project.MigrationGate {
	started := time.Now()
	preflight, err := preflightAndMaintainMigration(ctx, store, execute)
	if db.IsProjectMigrationContention(err) {
		// Another daemon process was writing the same database. Its transaction
		// has committed or rolled back by now, so a fresh preflight either finds
		// nothing left to do or applies cleanly; only a second failure is a real
		// block on this session. The whole preflight repeats rather than just the
		// execution, because the peer may have changed what the plan should be —
		// including turning maintenance into something that now needs review.
		preflight, err = preflightAndMaintainMigration(ctx, store, execute)
	}
	plan := preflight.Plan
	if err == nil && preflight.NeedsOperatorReview() {
		// Nothing was written, so nothing is persisted either: a HiveWarning row
		// would itself be the mutation this path promises not to make. The gate
		// carries the whole story to every surface instead.
		return project.NewMigrationGate(pendingProjectMigrationStatus(preflight))
	}
	if err == nil {
		logProjectMigrationSummary(store.LastProjectMigrationSummary(), time.Since(started))
		return project.NewMigrationGate(project.MigrationStatus{State: project.MigrationStateReady})
	}
	status := project.MigrationStatus{
		State:           project.MigrationStateBlocked,
		Reason:          err.Error(),
		Continuation:    "hive project identity status",
		PlanFingerprint: plan.Fingerprint,
	}
	if backups != nil {
		status.BackupID = migrationBackupIDForPlan(ctx, backups, plan.Fingerprint)
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
