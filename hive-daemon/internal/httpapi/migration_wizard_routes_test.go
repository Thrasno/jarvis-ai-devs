package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/governance"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
)

// wizardFixture wires the real runner over a real database, because the whole
// value of these routes is that the payload a TUI renders describes the fold that
// would actually run. A fake service would let the payload drift from the plan.
func wizardFixture(t *testing.T, seed func(*testing.T, *db.DB)) (*httpapi.Server, *db.DB, *project.MigrationGate) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seed(t, store)
	backups := governance.NewSQLiteBackupStore(path, "", store.RawDB())
	preflight, err := db.ReadProjectMigrationPreflight(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	gate := project.NewMigrationGate(project.MigrationStatus{
		State:           project.MigrationStatePendingOperatorReview,
		PlanFingerprint: preflight.Plan.Fingerprint,
	})
	srv := httpapi.NewServerWithAll("127.0.0.1:0", store, store,
		governance.NewServiceWithBackup(store, backups), nil, mockHealthSummaryService{}, store)
	srv.SetMigrationGate(gate)
	srv.SetMigrationExecution(governance.NewProjectMigrationRunner(store, backups, gate))
	return srv, store, gate
}

func seedWizardFold(t *testing.T, store *db.DB) {
	t.Helper()
	for _, spelling := range []string{"Foo.Bar", "foo-bar"} {
		if _, err := store.RawDB().Exec(
			`INSERT INTO sessions (id, sync_id, project, dev_id, client) VALUES (?, ?, ?, 'dev', 'test')`,
			"session-"+spelling, "sync-"+spelling, spelling); err != nil {
			t.Fatal(err)
		}
	}
}

func seedWizardConflict(t *testing.T, store *db.DB) {
	t.Helper()
	for _, spelling := range []string{"Foo.Bar", "foo-bar"} {
		if _, err := store.RawDB().Exec(
			`INSERT INTO user_prompts (sync_id, project, session_id, content) VALUES ('', ?, 'session', ?)`,
			spelling, spelling); err != nil {
			t.Fatal(err)
		}
	}
}

func readWizardPlan(t *testing.T, srv *httpapi.Server) httpapi.MigrationPlanPayload {
	t.Helper()
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/governance/project-identity/plan", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("plan = %d %s, want 200", rr.Code, rr.Body.String())
	}
	var payload httpapi.MigrationPlanPayload
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode plan payload: %v: %s", err, rr.Body.String())
	}
	return payload
}

func postWizardExecute(t *testing.T, srv *httpapi.Server, req project.MigrationExecuteRequest) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/governance/project-identity/execute", bytes.NewReader(body)))
	decoded := map[string]any{}
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode execute response: %v: %s", err, rr.Body.String())
	}
	return rr.Code, decoded
}

func readWizardProgress(t *testing.T, srv *httpapi.Server) httpapi.MigrationProgressPayload {
	t.Helper()
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/governance/project-identity/progress", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("progress = %d %s, want 200", rr.Code, rr.Body.String())
	}
	var payload httpapi.MigrationProgressPayload
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode progress payload: %v: %s", err, rr.Body.String())
	}
	return payload
}

func awaitWizardTerminal(t *testing.T, srv *httpapi.Server) httpapi.MigrationProgressPayload {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		payload := readWizardProgress(t, srv)
		if payload.State != "" && payload.State != string(db.ProjectMigrationRunNone) && payload.State != string(db.ProjectMigrationRunRunning) {
			return payload
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("fold never reached a terminal outcome over HTTP")
	return httpapi.MigrationProgressPayload{}
}

// TestPlanRouteCarriesEveryFieldTheWizardHasToRender is the transport contract.
// The review screen has to name the canonical key, the display name it will keep,
// where that name came from, and how many rows move — so every one of those has to
// be on the wire, not summarized into a count.
func TestPlanRouteCarriesEveryFieldTheWizardHasToRender(t *testing.T) {
	srv, store, _ := wizardFixture(t, seedWizardFold)
	payload := readWizardPlan(t, srv)

	preflight, err := db.ReadProjectMigrationPreflight(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	if payload.PlanFingerprint != preflight.Plan.Fingerprint {
		t.Fatalf("fingerprint = %q, want %q", payload.PlanFingerprint, preflight.Plan.Fingerprint)
	}
	if payload.Confirmation != db.ProjectMigrationConfirmation(preflight.Plan) {
		t.Fatalf("confirmation = %q, want the daemon-derived phrase %q", payload.Confirmation, db.ProjectMigrationConfirmation(preflight.Plan))
	}
	if !payload.FoldsIdentities {
		t.Fatal("folds_identities = false on a plan built from two spellings of one project")
	}
	if !payload.Executable {
		t.Fatal("executable = false on a conflict-free plan")
	}
	if payload.State != project.MigrationStatePendingOperatorReview {
		t.Fatalf("state = %q, want the gate state the wizard branches on", payload.State)
	}
	if len(payload.Groups) != len(preflight.Plan.Groups) {
		t.Fatalf("groups = %d, want %d", len(payload.Groups), len(preflight.Plan.Groups))
	}
	group, want := payload.Groups[0], preflight.Plan.Groups[0]
	if group.Key != want.Key || group.Display != want.Display ||
		group.DisplaySource != string(want.DisplaySource) ||
		group.Records != want.Records || group.Coalesced != want.Coalesced {
		t.Fatalf("group = %+v, want every field of %+v", group, want)
	}
	if group.Key == "" || group.Display == "" || group.DisplaySource == "" || group.Records == 0 {
		t.Fatalf("group = %+v, want no field left empty by the mapping", group)
	}
	// The overview screen exists to show the operator their own spellings. A group
	// that reached the wire as a record count would be unreadable to a human.
	if len(group.Variants) != len(want.Variants) {
		t.Fatalf("variants = %+v, want %+v", group.Variants, want.Variants)
	}
	canonical := 0
	for i, variant := range group.Variants {
		if variant.Spelling != want.Variants[i].Spelling || variant.Canonical != want.Variants[i].Canonical {
			t.Fatalf("variant %d = %+v, want %+v", i, variant, want.Variants[i])
		}
		if variant.Canonical {
			canonical++
			if variant.Spelling != group.Key {
				t.Fatalf("variant %+v is marked canonical but is not the group key %q", variant, group.Key)
			}
		}
	}
	if canonical != 1 {
		t.Fatalf("variants = %+v, want exactly the already-canonical spelling flagged", group.Variants)
	}
	if len(group.Variants) != 2 {
		t.Fatalf("variants = %+v, want the two spellings the fixture wrote", group.Variants)
	}
	if len(payload.Actions) != len(preflight.Plan.Actions) || payload.Actions[0].Key != preflight.Plan.Actions[0].Key {
		t.Fatalf("actions = %+v, want %+v", payload.Actions, preflight.Plan.Actions)
	}
	if len(payload.Conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none", payload.Conflicts)
	}
	if len(payload.Phases) != len(db.ProjectMigrationPhases()) {
		t.Fatalf("phases = %v, want the executor's phase order so a progress view can be laid out before it starts", payload.Phases)
	}
}

// TestPlanRouteCarriesEveryConflictField is the other half of the review screen.
// A conflicted plan cannot be folded at all, so the wizard has to explain what
// exactly disagrees — kind, table, project key and the colliding identity.
func TestPlanRouteCarriesEveryConflictField(t *testing.T) {
	srv, store, _ := wizardFixture(t, seedWizardConflict)
	payload := readWizardPlan(t, srv)

	preflight, err := db.ReadProjectMigrationPreflight(context.Background(), store)
	if err != nil {
		t.Fatal(err)
	}
	if len(preflight.Plan.Conflicts) == 0 {
		t.Fatal("fixture produced no conflict; the payload assertion would prove nothing")
	}
	if payload.Executable {
		t.Fatal("executable = true on a conflicted plan")
	}
	if len(payload.Conflicts) != len(preflight.Plan.Conflicts) {
		t.Fatalf("conflicts = %d, want %d", len(payload.Conflicts), len(preflight.Plan.Conflicts))
	}
	conflict, want := payload.Conflicts[0], preflight.Plan.Conflicts[0]
	if conflict.Kind != string(want.Kind) || conflict.Table != string(want.Table) ||
		conflict.Key != want.Key || conflict.Identity != want.Identity {
		t.Fatalf("conflict = %+v, want every field of %+v", conflict, want)
	}
	if conflict.Kind == "" || conflict.Table == "" || conflict.Key == "" {
		t.Fatalf("conflict = %+v, want no field left empty by the mapping", conflict)
	}
	if len(payload.Actions) != 0 {
		t.Fatalf("actions = %+v, want none; a conflicted plan offers no safe subset", payload.Actions)
	}
}

// TestExecuteRouteRefusalsAreEachTheirOwnMachineReadableOutcome is what keeps the
// wizard from showing "error" four different times. Each refusal has a different
// next step for the operator: re-review, retype the phrase, wait, or nothing at all.
func TestExecuteRouteRefusalsAreEachTheirOwnMachineReadableOutcome(t *testing.T) {
	t.Run("stale fingerprint", func(t *testing.T) {
		srv, _, _ := wizardFixture(t, seedWizardFold)
		plan := readWizardPlan(t, srv)
		code, body := postWizardExecute(t, srv, project.MigrationExecuteRequest{
			PlanFingerprint: "0000000000000000000000000000000000000000000000000000000000000000",
			Confirmation:    plan.Confirmation,
		})
		if code != http.StatusConflict || body["state"] != httpapi.MigrationExecutionStatePlanStale {
			t.Fatalf("execute = %d %v, want 409 %q", code, body, httpapi.MigrationExecutionStatePlanStale)
		}
	})

	t.Run("wrong confirmation phrase", func(t *testing.T) {
		srv, _, _ := wizardFixture(t, seedWizardFold)
		plan := readWizardPlan(t, srv)
		code, body := postWizardExecute(t, srv, project.MigrationExecuteRequest{
			PlanFingerprint: plan.PlanFingerprint,
			Confirmation:    plan.Confirmation + " NOT",
		})
		if code != http.StatusBadRequest || body["state"] != httpapi.MigrationExecutionStateConfirmationMismatch {
			t.Fatalf("execute = %d %v, want 400 %q", code, body, httpapi.MigrationExecutionStateConfirmationMismatch)
		}
	})

	t.Run("gate already ready", func(t *testing.T) {
		srv, _, gate := wizardFixture(t, seedWizardFold)
		plan := readWizardPlan(t, srv)
		gate.Adopt(project.MigrationStatus{State: project.MigrationStateReady})
		code, body := postWizardExecute(t, srv, project.MigrationExecuteRequest{
			PlanFingerprint: plan.PlanFingerprint,
			Confirmation:    plan.Confirmation,
		})
		if code != http.StatusConflict || body["state"] != httpapi.MigrationExecutionStateNotNeeded {
			t.Fatalf("execute = %d %v, want 409 %q", code, body, httpapi.MigrationExecutionStateNotNeeded)
		}
	})

	t.Run("second concurrent run", func(t *testing.T) {
		srv, _, _ := wizardFixture(t, seedWizardFold)
		plan := readWizardPlan(t, srv)
		request := project.MigrationExecuteRequest{
			PlanFingerprint: plan.PlanFingerprint,
			Confirmation:    plan.Confirmation,
		}
		if code, body := postWizardExecute(t, srv, request); code != http.StatusAccepted {
			t.Fatalf("first execute = %d %v, want 202", code, body)
		}
		code, body := postWizardExecute(t, srv, request)
		// The first fold may already be finished, in which case the gate is open
		// and the honest answer is "nothing left to do"; while it runs, the honest
		// answer is "already running". Either way it must never be a fault.
		if code != http.StatusConflict {
			t.Fatalf("second execute = %d %v, want 409", code, body)
		}
		if body["state"] != httpapi.MigrationExecutionStateAlreadyRunning &&
			body["state"] != httpapi.MigrationExecutionStateNotNeeded {
			t.Fatalf("second execute state = %v, want already-running or not-needed", body["state"])
		}
		awaitWizardTerminal(t, srv)
	})
}

// TestExecuteRouteReturnsImmediatelyAndTheResultArrivesThroughProgress locks the
// asynchronous contract. The fold rebuilds tables and reindexes, so a request that
// waited for it would sit past any client timeout and the operator would lose the
// only channel that could tell them what happened.
func TestExecuteRouteReturnsImmediatelyAndTheResultArrivesThroughProgress(t *testing.T) {
	srv, store, gate := wizardFixture(t, seedWizardFold)
	plan := readWizardPlan(t, srv)

	if before := readWizardProgress(t, srv); before.State != string(db.ProjectMigrationRunNone) {
		t.Fatalf("progress before any fold = %q, want %q", before.State, db.ProjectMigrationRunNone)
	}
	code, body := postWizardExecute(t, srv, project.MigrationExecuteRequest{
		PlanFingerprint: plan.PlanFingerprint,
		Confirmation:    plan.Confirmation,
	})
	if code != http.StatusAccepted || body["state"] != httpapi.MigrationExecutionStateAccepted {
		t.Fatalf("execute = %d %v, want 202 %q", code, body, httpapi.MigrationExecutionStateAccepted)
	}

	final := awaitWizardTerminal(t, srv)
	if final.State != string(db.ProjectMigrationRunSucceeded) {
		t.Fatalf("progress = %+v, want a successful result", final)
	}
	if final.Summary == nil {
		t.Fatal("summary = nil on a successful fold")
	}
	if final.Summary.RowsRekeyed == 0 {
		t.Fatalf("summary = %+v, want the executor's counters", final.Summary)
	}
	if final.BackupID == "" {
		t.Fatal("backup_id = empty; the rollback point has to reach the operator")
	}
	if final.Phase != string(db.ProjectMigrationPhaseCommit) {
		t.Fatalf("phase = %q, want the committed phase", final.Phase)
	}
	if len(final.Phases) != len(db.ProjectMigrationPhases()) {
		t.Fatalf("phases = %v, want the full ordered list", final.Phases)
	}
	if final.StartedAt.IsZero() || final.FinishedAt.IsZero() {
		t.Fatalf("progress timestamps = %s/%s, want both set", final.StartedAt, final.FinishedAt)
	}
	if err := gate.Check(); err != nil {
		t.Fatalf("gate = %v, want ready in place", err)
	}

	// The gate opening in place is what the operator experiences as "Hive works
	// again", so prove it through the same HTTP surface that was answering 503.
	write := httptest.NewRecorder()
	srv.ServeHTTP(write, httptest.NewRequest(http.MethodPost, "/prompts",
		bytes.NewBufferString(`{"content":"after the fold","project":"foo-bar"}`)))
	if write.Code != http.StatusOK && write.Code != http.StatusCreated {
		t.Fatalf("prompt write after the fold = %d %s, want a served write", write.Code, write.Body.String())
	}
	var spellings int
	if err := store.RawDB().QueryRow(`SELECT COUNT(DISTINCT project) FROM sessions`).Scan(&spellings); err != nil || spellings != 1 {
		t.Fatalf("distinct session spellings = %d, %v; want one canonical key", spellings, err)
	}
}
