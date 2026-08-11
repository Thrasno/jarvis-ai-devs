package hiveclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Gate and execute states the daemon reports for the project-identity fold.
// They are machine-readable branch keys: each one has a different next step for
// the operator, which is exactly why they are not collapsed into one error.
const (
	MigrationStatePendingReview = "migration-pending-operator-review"

	MigrationStateFoldAccepted         = "fold-accepted"
	MigrationStatePlanStale            = "plan-stale"
	MigrationStateConfirmationMismatch = "confirmation-mismatch"
	MigrationStateNotNeeded            = "fold-not-needed"
	MigrationStateAlreadyRunning       = "fold-already-running"
	MigrationStatePlanUnsafe           = "plan-unsafe"
	MigrationStateFoldUnavailable      = "fold-unavailable"
	MigrationStateInvalidRequest       = "invalid-request"
	MigrationStateRequestFailed        = "fold-request-failed"
)

// Run states reported by the progress route.
const (
	MigrationRunNone      = "no-run"
	MigrationRunRunning   = "running"
	MigrationRunSucceeded = "succeeded"
	MigrationRunFailed    = "failed"
)

// ErrMigrationRefused is the sentinel every typed daemon refusal matches, so a
// caller that only needs "the daemon said no" does not have to enumerate states.
var ErrMigrationRefused = errors.New("hive daemon refused the project-identity fold")

// MigrationRefusalError is a refusal from a project-identity wizard route. The
// daemon answers these with {"state","detail"} rather than {"error"}, and the
// wizard branches on State — never on Detail, which is human prose.
type MigrationRefusalError struct {
	StatusCode int
	State      string
	Detail     string
}

func (e *MigrationRefusalError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("hive daemon refused the project-identity fold: %s (status %d)", e.State, e.StatusCode)
	}
	return fmt.Sprintf("hive daemon refused the project-identity fold: %s: %s", e.State, e.Detail)
}

func (e *MigrationRefusalError) Unwrap() error { return ErrMigrationRefused }

// MigrationPlanVariant is one stored spelling of a project identity. Canonical
// marks the spelling the fold does not rewrite, so no caller ever has to
// re-derive canonicalization by comparing strings.
type MigrationPlanVariant struct {
	Spelling  string `json:"spelling"`
	Canonical bool   `json:"canonical"`
}

// MigrationPlanGroup is one project identity the fold would collapse. A group
// can carry zero canonical variants: when every stored spelling needs
// rewriting, the surviving name exists nowhere on disk yet.
type MigrationPlanGroup struct {
	Key           string                 `json:"key"`
	Display       string                 `json:"display"`
	DisplaySource string                 `json:"display_source"`
	Records       int                    `json:"records"`
	Coalesced     int                    `json:"coalesced"`
	Variants      []MigrationPlanVariant `json:"variants"`
}

// MigrationPlanConflict is one disagreement that makes the plan non-executable.
type MigrationPlanConflict struct {
	Kind     string `json:"kind"`
	Table    string `json:"table"`
	Key      string `json:"key"`
	Identity string `json:"identity"`
}

type MigrationPlanAction struct {
	Key string `json:"key"`
}

// MigrationPlan is the daemon's complete preflight for the fold. Confirmation is
// the exact phrase the execute route compares against; it must be echoed back
// verbatim and never rebuilt locally.
type MigrationPlan struct {
	State           string                  `json:"state"`
	Executable      bool                    `json:"executable"`
	FoldsIdentities bool                    `json:"folds_identities"`
	PlanFingerprint string                  `json:"plan_fingerprint"`
	Confirmation    string                  `json:"confirmation"`
	Groups          []MigrationPlanGroup    `json:"groups"`
	Conflicts       []MigrationPlanConflict `json:"conflicts"`
	Actions         []MigrationPlanAction   `json:"actions"`
	Phases          []string                `json:"phases"`
}

// MigrationExecuteRequest authorizes exactly the plan the operator reviewed.
type MigrationExecuteRequest struct {
	PlanFingerprint string `json:"plan_fingerprint"`
	Confirmation    string `json:"confirmation"`
}

// MigrationExecuteResult is the accepted-but-not-done answer: the fold runs
// detached and its outcome arrives through the progress route.
type MigrationExecuteResult struct {
	State string `json:"state"`
}

// MigrationSummary is present only on a succeeded run. SyncPositionsReset names
// the projects that re-check their full history on the next sync; it is safe and
// idempotent, and it is reported so a large re-download does not look like a fault.
type MigrationSummary struct {
	RowsRekeyed        int64    `json:"rows_rekeyed"`
	SessionsRequeued   int64    `json:"sessions_requeued"`
	PromptsRequeued    int64    `json:"prompts_requeued"`
	ReprojectsEnqueued int64    `json:"reprojects_enqueued"`
	SyncPositionsReset []string `json:"sync_positions_reset"`
}

// MigrationProgress is the durable progress and result of one fold. Reason is
// machine-readable and Detail is human prose: branch on Reason, render Detail.
type MigrationProgress struct {
	State           string            `json:"state"`
	PlanFingerprint string            `json:"plan_fingerprint,omitempty"`
	Phase           string            `json:"phase,omitempty"`
	Phases          []string          `json:"phases"`
	StartedAt       time.Time         `json:"started_at,omitempty"`
	FinishedAt      time.Time         `json:"finished_at,omitempty"`
	Reason          string            `json:"reason,omitempty"`
	Detail          string            `json:"detail,omitempty"`
	Retryable       bool              `json:"retryable,omitempty"`
	BackupID        string            `json:"backup_id,omitempty"`
	Summary         *MigrationSummary `json:"summary,omitempty"`
}

// MigrationPlan reads the fold preflight from GET /governance/project-identity/plan.
func (c *Client) MigrationPlan(ctx context.Context) (MigrationPlan, error) {
	var plan MigrationPlan
	if err := c.migrationRequest(ctx, http.MethodGet, "/governance/project-identity/plan", nil, &plan); err != nil {
		return MigrationPlan{}, err
	}
	return plan, nil
}

// ExecuteMigration authorizes the reviewed fold through
// POST /governance/project-identity/execute. A 202 means accepted, not done.
func (c *Client) ExecuteMigration(ctx context.Context, req MigrationExecuteRequest) (MigrationExecuteResult, error) {
	var result MigrationExecuteResult
	if err := c.migrationRequest(ctx, http.MethodPost, "/governance/project-identity/execute", req, &result); err != nil {
		return MigrationExecuteResult{}, err
	}
	return result, nil
}

// MigrationProgress reads the durable run state from
// GET /governance/project-identity/progress.
func (c *Client) MigrationProgress(ctx context.Context) (MigrationProgress, error) {
	var progress MigrationProgress
	if err := c.migrationRequest(ctx, http.MethodGet, "/governance/project-identity/progress", nil, &progress); err != nil {
		return MigrationProgress{}, err
	}
	return progress, nil
}

// migrationRequest is a dedicated transport for the wizard routes because they
// carry a typed {"state","detail"} refusal body that the shared get/post
// helpers, which only understand {"error"}, would flatten into a bare status code.
func (c *Client) migrationRequest(ctx context.Context, method, path string, payload, out any) error {
	var body []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = encoded
	}
	u := *c.baseURL
	setURLPath(&u, path)
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	var req *http.Request
	var err error
	if reader != nil {
		req, err = http.NewRequestWithContext(ctx, method, u.String(), reader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, u.String(), nil)
	}
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var refusal struct {
			State  string `json:"state"`
			Detail string `json:"detail"`
			Error  string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&refusal)
		if refusal.State != "" {
			return &MigrationRefusalError{StatusCode: resp.StatusCode, State: refusal.State, Detail: refusal.Detail}
		}
		return &APIError{StatusCode: resp.StatusCode, Message: refusal.Error}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
