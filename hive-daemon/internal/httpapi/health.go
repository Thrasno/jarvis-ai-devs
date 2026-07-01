package httpapi

import (
	"context"
	"net/http"
	"time"

	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
)

// HealthService is the interface the health summary handler uses.
// httpapi.HealthServiceAdapter implements it by wrapping sync.HealthServicer.
type HealthService interface {
	Summary(ctx context.Context) (HealthSummaryResponse, error)
}

// HealthSummaryResponse is the wire DTO for GET /governance/health/summary.
// It is frozen in PR 1 — PR 2 (TUI) depends on this shape.
type HealthSummaryResponse struct {
	Reachable           bool       `json:"reachable"`
	AuthOK              bool       `json:"auth_ok"`
	AutoSync            bool       `json:"auto_sync"`
	LastSuccessAt       *time.Time `json:"last_success_at"` // null when zero
	LastFailureAt       *time.Time `json:"last_failure_at"` // null when zero
	BackoffUntil        *time.Time `json:"backoff_until"`   // null when zero
	LastError           string     `json:"last_error"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	UnsyncedMemories    int        `json:"unsynced_memories"`
	UnsyncedPrompts     int        `json:"unsynced_prompts"`
	UnsyncedSessions    int        `json:"unsynced_sessions"`

	// LastDrainState/LastDrainReason/LastDrainRemaining are ADDITIVE fields
	// (PR 3, task 3.3, hive-sync-batched-drain) surfacing the most recently
	// persisted Drain outcome. omitempty so an older daemon DB / a project
	// that never ran Drain does not add noise to the response. Do NOT
	// rename/remove/reorder the existing fields above — this DTO is frozen
	// for the TUI (PR 1 note).
	LastDrainState     string `json:"last_drain_state,omitempty"`
	LastDrainReason    string `json:"last_drain_reason,omitempty"`
	LastDrainRemaining int    `json:"last_drain_remaining,omitempty"`
}

// HealthServiceAdapter adapts a sync.HealthServicer to the httpapi.HealthService
// interface, mapping sync.HealthSummary to HealthSummaryResponse.
type HealthServiceAdapter struct {
	svc hivesync.HealthServicer
}

// NewHealthServiceAdapter constructs a HealthServiceAdapter.
func NewHealthServiceAdapter(svc hivesync.HealthServicer) *HealthServiceAdapter {
	return &HealthServiceAdapter{svc: svc}
}

// Summary calls the underlying HealthServicer and maps the domain type to the
// wire DTO.
func (a *HealthServiceAdapter) Summary(ctx context.Context) (HealthSummaryResponse, error) {
	s, err := a.svc.Summary(ctx)
	if err != nil {
		return HealthSummaryResponse{}, err
	}
	return healthSummaryToResponse(s), nil
}

// healthSummaryToResponse converts a sync.HealthSummary to HealthSummaryResponse.
// Zero time.Time values become nil pointers (JSON null).
func healthSummaryToResponse(s hivesync.HealthSummary) HealthSummaryResponse {
	resp := HealthSummaryResponse{
		Reachable:           s.Reachable,
		AuthOK:              s.AuthOK,
		AutoSync:            s.AutoSync,
		LastError:           s.LastError,
		ConsecutiveFailures: s.ConsecutiveFailures,
		UnsyncedMemories:    s.UnsyncedMemories,
		UnsyncedPrompts:     s.UnsyncedPrompts,
		UnsyncedSessions:    s.UnsyncedSessions,
		LastDrainState:      s.LastDrainState,
		LastDrainReason:     s.LastDrainReason,
		LastDrainRemaining:  s.LastDrainRemaining,
	}
	if !s.LastSuccessAt.IsZero() {
		t := s.LastSuccessAt.UTC()
		resp.LastSuccessAt = &t
	}
	if !s.LastFailureAt.IsZero() {
		t := s.LastFailureAt.UTC()
		resp.LastFailureAt = &t
	}
	// BackoffUntil is null only when zero. The domain (aggregate) already
	// guarantees that only future values reach HealthSummary.BackoffUntil,
	// so the DTO trusts the domain and forwards any non-zero value directly.
	if !s.BackoffUntil.IsZero() {
		t := s.BackoffUntil.UTC()
		resp.BackoffUntil = &t
	}
	return resp
}

// handleHealthSummary handles GET /governance/health/summary.
// Method constraint enforced by mux pattern "GET /governance/health/summary".
func (s *Server) handleHealthSummary(w http.ResponseWriter, r *http.Request) {
	if !requireLoopback(w, r) {
		return
	}
	summary, err := s.health.Summary(r.Context())
	if err != nil {
		writeInternalError(w, "health summary", err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// NewServerWithHealth constructs a Server with both prompts and health endpoints.
// This constructor is provided for tests; in production main.go uses NewServerWithAll.
func NewServerWithHealth(addr string, prompts PromptStore, health HealthService) *Server {
	return NewServerWithAll(addr, prompts, nil, nil, nil, health)
}
