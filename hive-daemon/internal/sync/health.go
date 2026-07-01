package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
)

// HealthSummary is the aggregated sync health across all projects.
type HealthSummary struct {
	Reachable           bool
	AuthOK              bool
	AutoSync            bool
	LastSuccessAt       time.Time
	LastFailureAt       time.Time
	BackoffUntil        time.Time
	LastError           string
	ConsecutiveFailures int
	UnsyncedMemories    int
	UnsyncedPrompts     int
	UnsyncedSessions    int

	// LastDrainState/LastDrainReason/LastDrainRemaining (PR 3, task 3.3/3.4,
	// hive-sync-batched-drain) surface the most recently persisted Drain
	// outcome across the aggregated projects — see db.SyncHealth's
	// LastDrainState/LastDrainReason/LastDrainRemaining doc for the exact
	// per-project persistence and the aggregate() doc for which row's value
	// wins when multiple projects have drained. Empty/zero when no Drain run
	// has persisted an outcome yet (e.g. a fresh install, or a DB predating
	// this migration).
	LastDrainState     string
	LastDrainReason    string
	LastDrainRemaining int
}

// HealthServicer is the interface callers use to retrieve the aggregate sync
// health summary. httpapi.HealthServiceAdapter wraps a concrete implementation.
type HealthServicer interface {
	Summary(ctx context.Context) (HealthSummary, error)
}

// HealthStore is the minimal DB interface required by HealthService.
type HealthStore interface {
	ListGovernanceSyncHealth(ctx context.Context) ([]db.SyncHealth, error)
	CountUnsyncedMemories(ctx context.Context) (int, error)
	CountUnsyncedPrompts(ctx context.Context) (int, error)
	CountUnsyncedSessions(ctx context.Context) (int, error)
}

// ConfigLoader abstracts config loading so HealthService is testable without
// touching the filesystem.
type ConfigLoader interface {
	Load() (*Config, SyncConfigStatus, error)
}

// loadWithStatusAdapter wraps the package-level LoadWithStatus function so it
// satisfies ConfigLoader. Used as the default in NewHealthService.
type loadWithStatusAdapter struct{}

func (l *loadWithStatusAdapter) Load() (*Config, SyncConfigStatus, error) {
	return LoadWithStatus()
}

// HealthService is the concrete HealthServicer. Inject HealthStore and
// ConfigLoader at construction for testability.
type HealthService struct {
	store  HealthStore
	loader ConfigLoader
}

// NewHealthService constructs a HealthService with the provided store and
// loader. Pass nil for loader to use the default LoadWithStatus from disk.
func NewHealthService(store HealthStore, loader ConfigLoader) *HealthService {
	if loader == nil {
		loader = &loadWithStatusAdapter{}
	}
	return &HealthService{store: store, loader: loader}
}

// Summary aggregates sync_state rows and derives the HealthSummary fields.
func (h *HealthService) Summary(ctx context.Context) (HealthSummary, error) {
	rows, err := h.store.ListGovernanceSyncHealth(ctx)
	if err != nil {
		return HealthSummary{}, err
	}

	summary := aggregate(rows, time.Now().UTC())

	// Load unsynced counts.
	summary.UnsyncedMemories, err = h.store.CountUnsyncedMemories(ctx)
	if err != nil {
		return HealthSummary{}, err
	}
	summary.UnsyncedPrompts, err = h.store.CountUnsyncedPrompts(ctx)
	if err != nil {
		return HealthSummary{}, err
	}
	summary.UnsyncedSessions, err = h.store.CountUnsyncedSessions(ctx)
	if err != nil {
		return HealthSummary{}, err
	}

	// Derive AuthOK from sync history BEFORE config block can overwrite LastError.
	summary.AuthOK = !authError(summary.LastError)

	// Load config for auto_sync and reachability (configured = has APIURL).
	cfg, status, cfgErr := h.loader.Load()
	if cfgErr != nil {
		logger.Log.Printf("warn: HealthService.Summary: config unavailable: %v", cfgErr)
		// Append config error without losing the sync-derived LastError.
		if summary.LastError == "" {
			summary.LastError = fmt.Sprintf("config unavailable: %v", cfgErr)
		} else {
			summary.LastError = fmt.Sprintf("%s; config unavailable: %v", summary.LastError, cfgErr)
		}
	}
	summary.AutoSync = status.Configured && cfg != nil && cfg.AutoSync

	configured := cfg != nil && cfg.APIURL != ""
	summary.Reachable = deriveReachable(configured, summary.LastError, !summary.AuthOK)

	return summary, nil
}

// aggregate computes the aggregate fields from a set of SyncHealth rows.
func aggregate(rows []db.SyncHealth, now time.Time) HealthSummary {
	var s HealthSummary
	var mostRecentFailureAt time.Time

	for _, r := range rows {
		// MAX consecutive failures.
		if r.ConsecutiveFailures > s.ConsecutiveFailures {
			s.ConsecutiveFailures = r.ConsecutiveFailures
		}
		// Most recent success.
		if r.LastSuccessAt.After(s.LastSuccessAt) {
			s.LastSuccessAt = r.LastSuccessAt
		}
		// Most recent failure.
		if r.LastFailureAt.After(s.LastFailureAt) {
			s.LastFailureAt = r.LastFailureAt
		}
		// Latest active backoff (in the future only).
		if r.BackoffUntil.After(now) && r.BackoffUntil.After(s.BackoffUntil) {
			s.BackoffUntil = r.BackoffUntil
		}
		// LastError from the project with the most recent failure.
		if r.LastFailureAt.After(mostRecentFailureAt) {
			mostRecentFailureAt = r.LastFailureAt
			s.LastError = r.LastError
		}
	}

	return s
}

// authError returns true when the error string signals an authentication
// failure (contains "401" or "unauthorized", case-insensitive).
func authError(lastError string) bool {
	lower := strings.ToLower(lastError)
	return strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized")
}

// deriveReachable computes the reachable boolean from the spec §2c truth table.
//
//	| configured | lastError   | authErr | reachable |
//	|------------|-------------|---------|-----------|
//	| false      | any         | any     | false     |
//	| true       | ""          | true    | true      |
//	| true       | non-empty   | true    | true      |  (auth error — server reachable)
//	| true       | non-empty   | false   | false     |  (network error)
func deriveReachable(configured bool, lastError string, authErr bool) bool {
	if !configured {
		return false
	}
	if lastError == "" {
		return true
	}
	// Non-empty error: if it is an auth error the server IS reachable.
	// If it is a non-auth error the server is unreachable.
	return authErr // authErr == true means the error IS an auth error → reachable
}
