package hiveclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientReadsMigrationPlanWithVariantsAndPhases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/governance/project-identity/plan" {
			t.Fatalf("request = %s %s, want GET /governance/project-identity/plan", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"state":"migration-pending-operator-review",
			"executable":true,
			"folds_identities":true,
			"plan_fingerprint":"a1b2c3d4",
			"confirmation":"NORMALIZE 1 PROJECT a1b2c3d4",
			"groups":[{"key":"jarvis-dev","display":"Jarvis-Dev","display_source":"remote","records":148,"coalesced":1,
				"variants":[{"spelling":"Jarvis-Dev","canonical":false},{"spelling":"jarvis-dev","canonical":true}]}],
			"conflicts":[],
			"actions":[{"key":"jarvis-dev"}],
			"phases":["backup","revalidate","commit"]
		}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	plan, err := client.MigrationPlan(context.Background())
	if err != nil {
		t.Fatalf("MigrationPlan: %v", err)
	}
	if plan.State != "migration-pending-operator-review" || !plan.Executable || !plan.FoldsIdentities {
		t.Fatalf("plan header = %+v, want the pending executable fold", plan)
	}
	if plan.PlanFingerprint != "a1b2c3d4" || plan.Confirmation != "NORMALIZE 1 PROJECT a1b2c3d4" {
		t.Fatalf("plan identity = %q/%q, want the daemon values verbatim", plan.PlanFingerprint, plan.Confirmation)
	}
	if len(plan.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(plan.Groups))
	}
	group := plan.Groups[0]
	if group.Display != "Jarvis-Dev" || group.DisplaySource != "remote" || group.Records != 148 || group.Coalesced != 1 {
		t.Fatalf("group = %+v, want the decoded display metadata", group)
	}
	if len(group.Variants) != 2 || group.Variants[0].Spelling != "Jarvis-Dev" || group.Variants[0].Canonical {
		t.Fatalf("variants = %+v, want the non-canonical spelling first", group.Variants)
	}
	if !group.Variants[1].Canonical {
		t.Fatalf("variants = %+v, want jarvis-dev flagged canonical", group.Variants)
	}
	if len(plan.Phases) != 3 || plan.Phases[0] != "backup" || plan.Phases[2] != "commit" {
		t.Fatalf("phases = %v, want the daemon order preserved", plan.Phases)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Key != "jarvis-dev" {
		t.Fatalf("actions = %+v, want the single keyed action", plan.Actions)
	}
}

func TestClientDecodesConflictedMigrationPlan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"state":"migration-pending-operator-review","executable":false,"folds_identities":true,
			"plan_fingerprint":"ff00","confirmation":"NORMALIZE 1 PROJECT ff00",
			"groups":[],
			"conflicts":[{"kind":"divergent-global-entity","table":"memories","key":"mem_1","identity":"jarvis-dev"}],
			"actions":[],
			"phases":["backup"]
		}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	plan, err := client.MigrationPlan(context.Background())
	if err != nil {
		t.Fatalf("MigrationPlan: %v", err)
	}
	if plan.Executable || len(plan.Actions) != 0 {
		t.Fatalf("plan = %+v, want a non-executable plan with no actions", plan)
	}
	if len(plan.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v, want 1", plan.Conflicts)
	}
	conflict := plan.Conflicts[0]
	if conflict.Kind != "divergent-global-entity" || conflict.Table != "memories" || conflict.Key != "mem_1" || conflict.Identity != "jarvis-dev" {
		t.Fatalf("conflict = %+v, want every field decoded", conflict)
	}
}

func TestClientSurfacesFoldUnavailableAsTypedRefusal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"state":"fold-unavailable"}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = client.MigrationPlan(context.Background())
	var refusal *MigrationRefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want *MigrationRefusalError", err)
	}
	if refusal.State != MigrationStateFoldUnavailable || refusal.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("refusal = %+v, want fold-unavailable/503", refusal)
	}
}

func TestClientExecutesMigrationWithFingerprintAndConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/governance/project-identity/execute" {
			t.Fatalf("request = %s %s, want POST /governance/project-identity/execute", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode execute: %v", err)
		}
		if body["plan_fingerprint"] != "a1b2c3d4" || body["confirmation"] != "NORMALIZE 1 PROJECT a1b2c3d4" {
			t.Fatalf("body = %#v, want the exact reviewed plan identity", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"state":"fold-accepted"}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := client.ExecuteMigration(context.Background(), MigrationExecuteRequest{
		PlanFingerprint: "a1b2c3d4",
		Confirmation:    "NORMALIZE 1 PROJECT a1b2c3d4",
	})
	if err != nil {
		t.Fatalf("ExecuteMigration: %v", err)
	}
	if result.State != MigrationStateFoldAccepted {
		t.Fatalf("state = %q, want fold-accepted", result.State)
	}
}

func TestClientMapsEveryExecuteRefusalToItsState(t *testing.T) {
	cases := []struct {
		status int
		state  string
	}{
		{http.StatusConflict, MigrationStatePlanStale},
		{http.StatusBadRequest, MigrationStateConfirmationMismatch},
		{http.StatusConflict, MigrationStateNotNeeded},
		{http.StatusConflict, MigrationStateAlreadyRunning},
		{http.StatusConflict, MigrationStatePlanUnsafe},
		{http.StatusBadRequest, MigrationStateInvalidRequest},
		{http.StatusInternalServerError, MigrationStateRequestFailed},
	}
	for _, c := range cases {
		t.Run(c.state, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(`{"state":"` + c.state + `","detail":"the daemon explanation"}`))
			}))
			defer server.Close()

			client, err := New(server.URL)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			_, err = client.ExecuteMigration(context.Background(), MigrationExecuteRequest{PlanFingerprint: "f", Confirmation: "c"})
			var refusal *MigrationRefusalError
			if !errors.As(err, &refusal) {
				t.Fatalf("err = %v, want *MigrationRefusalError", err)
			}
			if refusal.State != c.state || refusal.Detail != "the daemon explanation" {
				t.Fatalf("refusal = %+v, want state %q with its detail", refusal, c.state)
			}
			if !errors.Is(err, ErrMigrationRefused) {
				t.Fatalf("refusal does not match ErrMigrationRefused")
			}
		})
	}
}

func TestClientReadsMigrationProgressSummaryOnlyWhenSucceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/governance/project-identity/progress" {
			t.Fatalf("request = %s %s, want GET /governance/project-identity/progress", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"state":"succeeded","plan_fingerprint":"a1b2c3d4","phase":"commit",
			"phases":["backup","commit"],
			"started_at":"2026-08-11T10:00:00Z","finished_at":"2026-08-11T10:00:05Z",
			"backup_id":"hive-20260811-100000",
			"summary":{"rows_rekeyed":148,"sessions_requeued":3,"prompts_requeued":4,
				"reprojects_enqueued":2,"sync_positions_reset":["jarvis-dev"]}
		}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	progress, err := client.MigrationProgress(context.Background())
	if err != nil {
		t.Fatalf("MigrationProgress: %v", err)
	}
	if progress.State != MigrationRunSucceeded || progress.Phase != "commit" || progress.BackupID != "hive-20260811-100000" {
		t.Fatalf("progress = %+v, want the succeeded run", progress)
	}
	if progress.Summary == nil {
		t.Fatal("summary is nil, want the succeeded counters")
	}
	if progress.Summary.RowsRekeyed != 148 || progress.Summary.SessionsRequeued != 3 ||
		progress.Summary.PromptsRequeued != 4 || progress.Summary.ReprojectsEnqueued != 2 {
		t.Fatalf("summary = %+v, want every counter decoded", progress.Summary)
	}
	if len(progress.Summary.SyncPositionsReset) != 1 || progress.Summary.SyncPositionsReset[0] != "jarvis-dev" {
		t.Fatalf("sync_positions_reset = %v, want the named project", progress.Summary.SyncPositionsReset)
	}
	if progress.StartedAt.IsZero() || progress.FinishedAt.IsZero() {
		t.Fatalf("timestamps = %v/%v, want both decoded", progress.StartedAt, progress.FinishedAt)
	}
}

func TestClientReadsFailedMigrationProgressWithoutSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"failed","phase":"rekey","phases":["backup","rekey"],
			"reason":"contention","detail":"another writer held the lock","retryable":true,"backup_id":"b-1"}`))
	}))
	defer server.Close()

	client, err := New(server.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	progress, err := client.MigrationProgress(context.Background())
	if err != nil {
		t.Fatalf("MigrationProgress: %v", err)
	}
	if progress.State != MigrationRunFailed || progress.Reason != "contention" || !progress.Retryable {
		t.Fatalf("progress = %+v, want the retryable contention failure", progress)
	}
	if progress.Summary != nil {
		t.Fatalf("summary = %+v, want nil on a failed run", progress.Summary)
	}
}
