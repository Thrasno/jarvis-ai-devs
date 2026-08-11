package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

// Machine-readable outcomes of an execute request. Each one has a different next
// step for the operator, which is the whole reason they are not one error: re-read
// and review again, retype the phrase, watch the fold that is already running, or
// nothing at all.
const (
	MigrationExecutionStateAccepted             = "fold-accepted"
	MigrationExecutionStatePlanStale            = "plan-stale"
	MigrationExecutionStateConfirmationMismatch = "confirmation-mismatch"
	MigrationExecutionStateNotNeeded            = "fold-not-needed"
	MigrationExecutionStateAlreadyRunning       = "fold-already-running"
	MigrationExecutionStatePlanUnsafe           = "plan-unsafe"
	MigrationExecutionStateUnavailable          = "fold-unavailable"
	MigrationExecutionStateInvalidRequest       = "invalid-request"
	MigrationExecutionStateFailed               = "fold-request-failed"
)

// MigrationExecutionService is the daemon-side owner of the operator's fold. It
// returns db types rather than payloads so the wire shape stays this package's
// concern and the runner never has to know about HTTP.
type MigrationExecutionService interface {
	MigrationPlan(context.Context) (db.ProjectMigrationPreflight, error)
	ExecuteMigration(context.Context, project.MigrationExecuteRequest) error
	MigrationProgress(context.Context) (db.ProjectMigrationRun, bool, error)
}

// SetMigrationExecution installs the fold owner. Without it the wizard routes
// answer "unavailable" rather than 404, so a client can tell a daemon that does
// not support the fold from one that does and is refusing.
func (s *Server) SetMigrationExecution(execution MigrationExecutionService) {
	s.execution = execution
}

// MigrationPlanPayload is the complete typed preflight. Nothing
// ProjectMigrationPlan carries is summarized away: the review screen has to name
// the canonical key, the display name the fold will keep, where that name came
// from and how many rows move, and a conflicted plan has to explain exactly what
// disagrees.
type MigrationPlanPayload struct {
	// State is the gate state the wizard branches on, so one read answers both
	// "what would the fold do" and "is the daemon still waiting for me".
	State           string                  `json:"state"`
	Executable      bool                    `json:"executable"`
	FoldsIdentities bool                    `json:"folds_identities"`
	PlanFingerprint string                  `json:"plan_fingerprint"`
	Confirmation    string                  `json:"confirmation"`
	Groups          []MigrationPlanGroup    `json:"groups"`
	Conflicts       []MigrationPlanConflict `json:"conflicts"`
	Actions         []MigrationPlanAction   `json:"actions"`
	// Phases is the executor's real order, sent with the plan so a progress view
	// can be laid out before the fold starts rather than growing as it runs.
	Phases []string `json:"phases"`
}

type MigrationPlanGroup struct {
	Key           string `json:"key"`
	Display       string `json:"display"`
	DisplaySource string `json:"display_source"`
	Records       int    `json:"records"`
	Coalesced     int    `json:"coalesced"`
	// Variants are the distinct stored spellings, sorted, each flagged when it is
	// already the canonical one. This is what the overview screen renders as
	// "jarvis-dev ← Jarvis-Dev, jarvis-dev"; Records alone is a number a human
	// cannot check anything against.
	Variants []MigrationPlanVariant `json:"variants"`
}

type MigrationPlanVariant struct {
	Spelling string `json:"spelling"`
	// Canonical marks the spelling the fold does not rewrite, so a client never has
	// to re-derive canonicalization to know which name survives.
	Canonical bool `json:"canonical"`
}

type MigrationPlanConflict struct {
	Kind     string `json:"kind"`
	Table    string `json:"table"`
	Key      string `json:"key"`
	Identity string `json:"identity"`
}

type MigrationPlanAction struct {
	Key string `json:"key"`
}

// MigrationProgressPayload is the durable progress and result of one fold.
type MigrationProgressPayload struct {
	State           string    `json:"state"`
	PlanFingerprint string    `json:"plan_fingerprint,omitempty"`
	Phase           string    `json:"phase,omitempty"`
	Phases          []string  `json:"phases"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	FinishedAt      time.Time `json:"finished_at,omitempty"`
	// Reason is machine-readable and Detail is for a human to read; a caller must
	// branch on Reason and never on Detail.
	Reason    string `json:"reason,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
	// BackupID is the archive that rolls this exact plan back, present on a failure
	// so recovery can be offered without a second lookup.
	BackupID string                   `json:"backup_id,omitempty"`
	Summary  *MigrationSummaryPayload `json:"summary,omitempty"`
}

type MigrationSummaryPayload struct {
	RowsRekeyed        int64 `json:"rows_rekeyed"`
	SessionsRequeued   int64 `json:"sessions_requeued"`
	PromptsRequeued    int64 `json:"prompts_requeued"`
	ReprojectsEnqueued int64 `json:"reprojects_enqueued"`
	// SyncPositionsReset names the projects that will re-pull their full window on
	// the next sync because the fold could not order their two watermarks. It is
	// safe and idempotent, and it is here so it does not look like a fault.
	SyncPositionsReset []string `json:"sync_positions_reset"`
}

func (s *Server) handleMigrationIdentityPlan(w http.ResponseWriter, r *http.Request) {
	if s.execution == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"state": MigrationExecutionStateUnavailable})
		return
	}
	preflight, err := s.execution.MigrationPlan(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, migrationPlanPayload(preflight, s.gate.Status().State))
}

func migrationPlanPayload(preflight db.ProjectMigrationPreflight, state string) MigrationPlanPayload {
	plan := preflight.Plan
	payload := MigrationPlanPayload{
		State:           state,
		Executable:      plan.Executable,
		FoldsIdentities: preflight.FoldsIdentities,
		PlanFingerprint: plan.Fingerprint,
		Confirmation:    db.ProjectMigrationConfirmation(plan),
		Groups:          make([]MigrationPlanGroup, 0, len(plan.Groups)),
		Conflicts:       make([]MigrationPlanConflict, 0, len(plan.Conflicts)),
		Actions:         make([]MigrationPlanAction, 0, len(plan.Actions)),
		Phases:          migrationPhaseNames(),
	}
	for _, group := range plan.Groups {
		variants := make([]MigrationPlanVariant, 0, len(group.Variants))
		for _, variant := range group.Variants {
			variants = append(variants, MigrationPlanVariant{Spelling: variant.Spelling, Canonical: variant.Canonical})
		}
		payload.Groups = append(payload.Groups, MigrationPlanGroup{
			Key:           group.Key,
			Display:       group.Display,
			DisplaySource: string(group.DisplaySource),
			Records:       group.Records,
			Coalesced:     group.Coalesced,
			Variants:      variants,
		})
	}
	for _, conflict := range plan.Conflicts {
		payload.Conflicts = append(payload.Conflicts, MigrationPlanConflict{
			Kind:     string(conflict.Kind),
			Table:    string(conflict.Table),
			Key:      conflict.Key,
			Identity: conflict.Identity,
		})
	}
	for _, action := range plan.Actions {
		payload.Actions = append(payload.Actions, MigrationPlanAction{Key: action.Key})
	}
	return payload
}

func migrationPhaseNames() []string {
	phases := db.ProjectMigrationPhases()
	names := make([]string, 0, len(phases))
	for _, phase := range phases {
		names = append(names, string(phase))
	}
	return names
}

func (s *Server) handleMigrationIdentityExecute(w http.ResponseWriter, r *http.Request) {
	if s.execution == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"state": MigrationExecutionStateUnavailable})
		return
	}
	var req project.MigrationExecuteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"state": MigrationExecutionStateInvalidRequest})
		return
	}
	if err := s.execution.ExecuteMigration(r.Context(), req); err != nil {
		status, state := migrationExecutionRefusal(err)
		writeJSON(w, status, map[string]string{"state": state, "detail": err.Error()})
		return
	}
	// Accepted, not done: the fold runs detached and the result arrives through the
	// progress route.
	writeJSON(w, http.StatusAccepted, map[string]string{"state": MigrationExecutionStateAccepted})
}

func migrationExecutionRefusal(err error) (int, string) {
	switch {
	case errors.Is(err, db.ErrProjectMigrationPlanStale):
		return http.StatusConflict, MigrationExecutionStatePlanStale
	case errors.Is(err, governance.ErrProjectMigrationConfirmationMismatch):
		return http.StatusBadRequest, MigrationExecutionStateConfirmationMismatch
	case errors.Is(err, governance.ErrProjectMigrationNotPending):
		return http.StatusConflict, MigrationExecutionStateNotNeeded
	case errors.Is(err, governance.ErrProjectMigrationAlreadyRunning):
		return http.StatusConflict, MigrationExecutionStateAlreadyRunning
	case errors.Is(err, db.ErrProjectMigrationPlanUnsafe):
		return http.StatusConflict, MigrationExecutionStatePlanUnsafe
	default:
		return http.StatusInternalServerError, MigrationExecutionStateFailed
	}
}

func (s *Server) handleMigrationIdentityProgress(w http.ResponseWriter, r *http.Request) {
	if s.execution == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"state": MigrationExecutionStateUnavailable})
		return
	}
	run, found, err := s.execution.MigrationProgress(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !found {
		// Absent is its own answer. A caller that could not tell "never asked for a
		// fold" from "failed silently" would show an alarm on a healthy database.
		writeJSON(w, http.StatusOK, MigrationProgressPayload{
			State:  string(db.ProjectMigrationRunNone),
			Phases: migrationPhaseNames(),
		})
		return
	}
	writeJSON(w, http.StatusOK, migrationProgressPayload(run))
}

func migrationProgressPayload(run db.ProjectMigrationRun) MigrationProgressPayload {
	payload := MigrationProgressPayload{
		State:           string(run.Outcome),
		PlanFingerprint: run.PlanFingerprint,
		Phase:           string(run.Phase),
		Phases:          migrationPhaseNames(),
		StartedAt:       run.StartedAt,
		FinishedAt:      run.FinishedAt,
		Reason:          run.Reason,
		Detail:          run.Detail,
		Retryable:       run.Retryable,
		BackupID:        run.BackupID,
	}
	if run.Outcome == db.ProjectMigrationRunSucceeded {
		payload.Summary = &MigrationSummaryPayload{
			RowsRekeyed:        run.Summary.RowsRekeyed,
			SessionsRequeued:   run.Summary.SessionsRequeued,
			PromptsRequeued:    run.Summary.PromptsRequeued,
			ReprojectsEnqueued: run.Summary.ReprojectsEnqueued,
			SyncPositionsReset: run.Summary.SyncPositionsReset,
		}
		if payload.Summary.SyncPositionsReset == nil {
			payload.Summary.SyncPositionsReset = []string{}
		}
	}
	return payload
}
