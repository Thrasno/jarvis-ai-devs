package hiveui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
)

// fakeNormalizationService serves queued plan and progress answers so a test can
// drive a whole fold — including a re-read that returns a different plan — without
// a daemon. Each queue clamps to its last entry so a repeated poll keeps working.
type fakeNormalizationService struct {
	plans        []hiveclient.MigrationPlan
	planErr      error
	planCalls    int
	executeReqs  []hiveclient.MigrationExecuteRequest
	executeState string
	executeErr   error
	progresses   []hiveclient.MigrationProgress
	progressErr  error
	// progressErrAfter lets a test serve real answers first and fail only later,
	// so a mid-fold transport failure can be driven without breaking the entry read.
	progressErrAfter int
	progressCalls    int
}

func (f *fakeNormalizationService) MigrationPlan(context.Context) (hiveclient.MigrationPlan, error) {
	f.planCalls++
	if f.planErr != nil {
		return hiveclient.MigrationPlan{}, f.planErr
	}
	if len(f.plans) == 0 {
		return hiveclient.MigrationPlan{}, nil
	}
	return f.plans[clampIndex(f.planCalls-1, len(f.plans))], nil
}

func (f *fakeNormalizationService) ExecuteMigration(_ context.Context, req hiveclient.MigrationExecuteRequest) (hiveclient.MigrationExecuteResult, error) {
	f.executeReqs = append(f.executeReqs, req)
	if f.executeErr != nil {
		return hiveclient.MigrationExecuteResult{}, f.executeErr
	}
	state := f.executeState
	if state == "" {
		state = hiveclient.MigrationStateFoldAccepted
	}
	return hiveclient.MigrationExecuteResult{State: state}, nil
}

func (f *fakeNormalizationService) MigrationProgress(context.Context) (hiveclient.MigrationProgress, error) {
	f.progressCalls++
	if f.progressErr != nil && f.progressCalls > f.progressErrAfter {
		return hiveclient.MigrationProgress{}, f.progressErr
	}
	if len(f.progresses) == 0 {
		return hiveclient.MigrationProgress{State: hiveclient.MigrationRunNone, Phases: normalizationTestPhases()}, nil
	}
	return f.progresses[clampIndex(f.progressCalls-1, len(f.progresses))], nil
}

func clampIndex(i, length int) int {
	if i >= length {
		return length - 1
	}
	return i
}

// noRunThen models the honest sequence: nothing is running when the operator
// opens the wizard, and the queued answers follow once the fold is accepted.
func noRunThen(progresses ...hiveclient.MigrationProgress) []hiveclient.MigrationProgress {
	queue := []hiveclient.MigrationProgress{{State: hiveclient.MigrationRunNone, Phases: normalizationTestPhases()}}
	return append(queue, progresses...)
}

func normalizationTestPhases() []string {
	return []string{"backup", "revalidate", "rekey", "commit"}
}

// executablePlan is one group whose stored spellings include the surviving one.
func executablePlan() hiveclient.MigrationPlan {
	return hiveclient.MigrationPlan{
		State:           hiveclient.MigrationStatePendingReview,
		Executable:      true,
		FoldsIdentities: true,
		PlanFingerprint: "a1b2c3d4",
		Confirmation:    "NORMALIZE 1 PROJECT a1b2c3d4",
		Groups: []hiveclient.MigrationPlanGroup{{
			Key:           "jarvis-dev",
			Display:       "Jarvis-Dev",
			DisplaySource: "remote",
			Records:       148,
			Coalesced:     1,
			Variants: []hiveclient.MigrationPlanVariant{
				{Spelling: "Jarvis-Dev", Canonical: false},
				{Spelling: "jarvis-dev", Canonical: true},
			},
		}},
		Conflicts: []hiveclient.MigrationPlanConflict{},
		Actions:   []hiveclient.MigrationPlanAction{{Key: "jarvis-dev"}},
		Phases:    normalizationTestPhases(),
	}
}

// zeroCanonicalPlan is the case where every stored spelling gets rewritten, so
// the surviving name exists nowhere on disk yet.
func zeroCanonicalPlan() hiveclient.MigrationPlan {
	plan := executablePlan()
	// Sorted ascending by spelling, as the daemon always sends them.
	plan.Groups[0].Variants = []hiveclient.MigrationPlanVariant{
		{Spelling: "JARVIS_DEV", Canonical: false},
		{Spelling: "Jarvis-Dev", Canonical: false},
	}
	return plan
}

func conflictedPlan() hiveclient.MigrationPlan {
	plan := executablePlan()
	plan.Executable = false
	plan.Actions = []hiveclient.MigrationPlanAction{}
	plan.Conflicts = []hiveclient.MigrationPlanConflict{
		{Kind: "divergent-global-entity", Table: "memories", Key: "mem_1", Identity: "jarvis-dev"},
		{Kind: "non-monotonic-cursor-protocol", Table: "pull_cursors", Key: "cursor_1", Identity: "jarvis-dev"},
	}
	return plan
}

func normalizationSnapshot() Snapshot {
	snapshot := sampleNavigationSnapshot()
	snapshot.MigrationState = hiveclient.MigrationStatePendingReview
	return snapshot
}

func dashboardActionIndex(t *testing.T, label string) int {
	t.Helper()
	for i, action := range dashboardActions() {
		if action.label == label {
			return i
		}
	}
	t.Fatalf("dashboard has no %q action", label)
	return -1
}

// openNormalization walks the real path: dashboard row → enter → entry load.
func openNormalization(t *testing.T, service *fakeNormalizationService) Model {
	t.Helper()
	m := NewModelWithSnapshotAndNormalizationService(normalizationSnapshot(), service)
	m.cursor = dashboardActionIndex(t, normalizationActionLabel)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).Screen() != ScreenProjectNormalization {
		t.Fatalf("screen = %v, want ScreenProjectNormalization", updated.(Model).Screen())
	}
	if cmd == nil {
		t.Fatal("cmd is nil, want the entry load")
	}
	updated, _ = updated.Update(cmd())
	return updated.(Model)
}

func normalizationAtReview(t *testing.T, service *fakeNormalizationService) Model {
	t.Helper()
	m := openNormalization(t, service)
	return sendKey(m, tea.KeyEnter)
}

func normalizationAtConfirm(t *testing.T, service *fakeNormalizationService) Model {
	t.Helper()
	return sendKey(normalizationAtReview(t, service), tea.KeyEnter)
}

// normalizationAtProgress submits a matching confirmation and applies the
// accepted execute answer, leaving the model on the live progress screen.
func normalizationAtProgress(t *testing.T, service *fakeNormalizationService) Model {
	t.Helper()
	m := normalizationAtConfirm(t, service)
	m = sendText(m, service.plans[0].Confirmation)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("cmd is nil, want the execute dispatch")
	}
	updated, _ = updated.Update(cmd())
	return updated.(Model)
}

// --- Screen 1: dashboard -----------------------------------------------------

func TestDashboardShowsNormalizationPendingAndDisablesEveryOtherAction(t *testing.T) {
	m := NewModelWithSnapshotAndNormalizationService(normalizationSnapshot(), &fakeNormalizationService{})
	view := m.View()

	assertContains(t, view, normalizationActionLabel, "pending review")

	for _, action := range m.dashboardActionRows() {
		if action.label == normalizationActionLabel {
			if action.disabled {
				t.Fatal("Project normalization is disabled while normalization is pending")
			}
			continue
		}
		if !action.disabled {
			t.Fatalf("%q stays enabled while normalization is pending", action.label)
		}
	}
	// Disabled, not hidden: the operator must still see their data is there.
	assertContains(t, view, "Project viewer", "(disabled)")
}

func TestDashboardDisablesNormalizationWhenNoMigrationIsPending(t *testing.T) {
	snapshot := sampleNavigationSnapshot()
	m := NewModelWithSnapshotAndNormalizationService(snapshot, &fakeNormalizationService{})

	for _, action := range m.dashboardActionRows() {
		if action.label == normalizationActionLabel {
			if !action.disabled {
				t.Fatal("Project normalization is offered when nothing needs normalizing")
			}
			return
		}
	}
	t.Fatal("dashboard has no Project normalization action")
}

func TestDashboardExplainsWhyOtherActionsAreBlockedByNormalization(t *testing.T) {
	m := NewModelWithSnapshotAndNormalizationService(normalizationSnapshot(), &fakeNormalizationService{})
	m.cursor = dashboardActionIndex(t, "Project viewer")
	m = sendKey(m, tea.KeyEnter)

	if m.Screen() != ScreenDashboard {
		t.Fatalf("screen = %v, want the dashboard to stay put", m.Screen())
	}
	assertContains(t, m.View(), "until project normalization", "Your data is intact")
}

// --- Screen 2: overview ------------------------------------------------------

func TestNormalizationOverviewExplainsTheBlockAndRendersEveryVariant(t *testing.T) {
	service := &fakeNormalizationService{plans: []hiveclient.MigrationPlan{executablePlan()}}
	view := openNormalization(t, service).View()

	assertContains(t, view,
		"different spellings",
		"indexes memories by project name",
		"nothing new is written under an ambiguous identity",
	)
	assertContains(t, view, "Jarvis-Dev", "jarvis-dev", "148")
	assertContains(t, view, "rewritten: Jarvis-Dev", "survives: jarvis-dev")
	assertContains(t, view, "the team server")
	assertContains(t, view, "No decisions needed.")
}

func TestNormalizationOverviewRendersOldestRegistrationDisplaySource(t *testing.T) {
	plan := executablePlan()
	plan.Groups[0].DisplaySource = "oldest-registration"
	service := &fakeNormalizationService{plans: []hiveclient.MigrationPlan{plan}}

	assertContains(t, openNormalization(t, service).View(), "the oldest local registration")
}

func TestNormalizationOverviewRendersGroupWithZeroCanonicalVariants(t *testing.T) {
	service := &fakeNormalizationService{plans: []hiveclient.MigrationPlan{zeroCanonicalPlan()}}
	view := openNormalization(t, service).View()

	assertContains(t, view,
		"rewritten: JARVIS_DEV, Jarvis-Dev",
		"survives: jarvis-dev",
		"no stored row uses it yet",
	)
}

// --- Screen 2b: conflicts ----------------------------------------------------

func TestNormalizationConflictsAreAnHonestDeadEnd(t *testing.T) {
	service := &fakeNormalizationService{plans: []hiveclient.MigrationPlan{conflictedPlan()}}
	m := openNormalization(t, service)

	assertContains(t, m.View(), "2 conflicts need a decision")

	m = sendKey(m, tea.KeyEnter)
	view := m.View()
	assertContains(t, view, "memories", "mem_1", "pull_cursors", "cursor_1")
	// Plain language for the operator, raw kind kept for a bug report.
	assertContains(t, view,
		"the same record exists twice with different content",
		"the sync cursors cannot be ordered safely",
		"raw kind: divergent-global-entity",
		"raw kind: non-monotonic-cursor-protocol",
	)
	assertContains(t, view, "not supported here yet", "Warnings")

	// No forward transition exists: enter must never reach the confirmation.
	for i := 0; i < 5; i++ {
		m = sendKey(m, tea.KeyEnter)
		if m.normalizationStep == normalizationStepConfirm {
			t.Fatalf("reached the confirmation screen from a conflicted plan after %d enters", i+1)
		}
	}
	assertNotContains(t, m.View(), conflictedPlan().Confirmation)
	if len(service.executeReqs) != 0 {
		t.Fatalf("execute dispatched %d times from a conflicted plan", len(service.executeReqs))
	}
}

// --- Screen 3: review --------------------------------------------------------

func TestNormalizationReviewShowsTotalBackupAndTeamServerNotice(t *testing.T) {
	service := &fakeNormalizationService{plans: []hiveclient.MigrationPlan{executablePlan()}}
	view := normalizationAtReview(t, service).View()

	assertContains(t, view, "Records to be modified: 148")
	assertContains(t, view, "A backup is taken before anything changes", "rolled back")
	assertContains(t, view, "reported to the team server", "do not stay split under the old names")
}

// --- Screen 4: confirm ------------------------------------------------------

func TestNormalizationConfirmUsesTheDaemonPhraseVerbatim(t *testing.T) {
	service := &fakeNormalizationService{plans: []hiveclient.MigrationPlan{executablePlan()}}
	view := normalizationAtConfirm(t, service).View()

	assertContains(t, view, "NORMALIZE 1 PROJECT a1b2c3d4", "Records to be modified: 148")
}

func TestNormalizationConfirmMismatchDoesNotDispatch(t *testing.T) {
	service := &fakeNormalizationService{plans: []hiveclient.MigrationPlan{executablePlan()}}
	m := sendText(normalizationAtConfirm(t, service), "NORMALIZE 1 PROJECT deadbeef")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("cmd is non-nil, want no dispatch on a mismatched confirmation")
	}
	if len(service.executeReqs) != 0 {
		t.Fatalf("execute dispatched %d times on a mismatch", len(service.executeReqs))
	}
	assertContains(t, updated.(Model).View(), "Confirmation mismatch")
}

func TestNormalizationConfirmMatchDispatchesThePlanIdentityExactly(t *testing.T) {
	service := &fakeNormalizationService{plans: []hiveclient.MigrationPlan{executablePlan()}}
	normalizationAtProgress(t, service)

	if len(service.executeReqs) != 1 {
		t.Fatalf("execute dispatched %d times, want 1", len(service.executeReqs))
	}
	req := service.executeReqs[0]
	if req.PlanFingerprint != "a1b2c3d4" {
		t.Fatalf("fingerprint = %q, want the reviewed plan fingerprint", req.PlanFingerprint)
	}
	if req.Confirmation != "NORMALIZE 1 PROJECT a1b2c3d4" {
		t.Fatalf("confirmation = %q, want the daemon phrase verbatim", req.Confirmation)
	}
}

// --- Screen 5: progress -----------------------------------------------------

func TestNormalizationProgressPollsOnTickAndAdvancesThePhase(t *testing.T) {
	service := &fakeNormalizationService{
		plans: []hiveclient.MigrationPlan{executablePlan()},
		progresses: noRunThen(
			hiveclient.MigrationProgress{State: hiveclient.MigrationRunRunning, Phase: "backup", Phases: normalizationTestPhases()},
			hiveclient.MigrationProgress{State: hiveclient.MigrationRunRunning, Phase: "rekey", Phases: normalizationTestPhases()},
		),
	}
	m := normalizationAtProgress(t, service)
	assertContains(t, m.View(), "Closing this screen is safe")
	// Never a percentage: discrete phases only.
	assertNotContains(t, m.View(), "%")

	m, calls := pollNormalizationOnce(t, m, service)
	if calls != 1 {
		t.Fatalf("progress reads = %d, want exactly 1 for the first tick", calls)
	}
	assertContains(t, m.View(), "backup", "revalidate", "rekey", "commit")

	m, calls = pollNormalizationOnce(t, m, service)
	if calls != 1 {
		t.Fatalf("progress reads = %d, want exactly 1 for the second tick", calls)
	}
	view := m.View()
	if !strings.Contains(view, normalizationPhaseDoneMark+" backup") {
		t.Fatalf("view =\n%s\nwant backup marked done once the run reached rekey", view)
	}
	if !strings.Contains(view, normalizationPhaseCurrentMark+" rekey") {
		t.Fatalf("view =\n%s\nwant rekey marked as the current phase", view)
	}
}

func TestNormalizationProgressStopsPollingOnceTheScreenIsLeft(t *testing.T) {
	service := &fakeNormalizationService{
		plans:      []hiveclient.MigrationPlan{executablePlan()},
		progresses: noRunThen(hiveclient.MigrationProgress{State: hiveclient.MigrationRunRunning, Phase: "backup", Phases: normalizationTestPhases()}),
	}
	m := normalizationAtProgress(t, service)
	staleTick := normalizationTickMsg{seq: m.normalizationPollSeq}

	m, _ = pollNormalizationOnce(t, m, service)
	before := service.progressCalls

	m = sendKey(m, tea.KeyEsc)
	if m.Screen() != ScreenDashboard {
		t.Fatalf("screen = %v, want the dashboard after leaving progress", m.Screen())
	}

	updated, cmd := m.Update(staleTick)
	if cmd != nil {
		t.Fatal("cmd is non-nil, want no poll from a tick that outlived its screen")
	}
	if service.progressCalls != before {
		t.Fatalf("progress calls = %d, want %d — the daemon was polled after the screen closed", service.progressCalls, before)
	}
	_ = updated
}

// pollNormalizationOnce drives one tick → poll → apply cycle and returns the
// model plus the number of progress reads that one cycle caused.
func pollNormalizationOnce(t *testing.T, m Model, service *fakeNormalizationService) (Model, int) {
	t.Helper()
	before := service.progressCalls
	updated, cmd := m.Update(normalizationTickMsg{seq: m.normalizationPollSeq})
	if cmd == nil {
		t.Fatal("cmd is nil, want a progress poll on tick")
	}
	msg := cmd()
	updated, _ = updated.Update(msg)
	return updated.(Model), service.progressCalls - before
}

// --- Screen 6: terminal states ----------------------------------------------

func TestNormalizationSucceededScreenReportsCountersBackupAndUndo(t *testing.T) {
	service := &fakeNormalizationService{
		plans: []hiveclient.MigrationPlan{executablePlan()},
		progresses: noRunThen(hiveclient.MigrationProgress{
			State:    hiveclient.MigrationRunSucceeded,
			Phase:    "commit",
			Phases:   normalizationTestPhases(),
			BackupID: "hive-20260811-100000",
			Summary: &hiveclient.MigrationSummary{
				RowsRekeyed: 148, SessionsRequeued: 3, PromptsRequeued: 4, ReprojectsEnqueued: 2,
				SyncPositionsReset: []string{},
			},
		}),
	}
	m, _ := pollNormalizationOnce(t, normalizationAtProgress(t, service), service)

	if m.normalizationStep != normalizationStepSucceeded {
		t.Fatalf("step = %v, want the succeeded screen", m.normalizationStep)
	}
	view := m.View()
	assertContains(t, view, "Rows rekeyed: 148", "Sessions requeued: 3", "Prompts requeued: 4", "Reprojections enqueued: 2")
	assertContains(t, view, "hive-20260811-100000", "Hive is unblocked", "restore backup")
	assertNotContains(t, view, "re-check the full history")
}

func TestNormalizationSucceededScreenExplainsResetSyncPositions(t *testing.T) {
	service := &fakeNormalizationService{
		plans: []hiveclient.MigrationPlan{executablePlan()},
		progresses: noRunThen(hiveclient.MigrationProgress{
			State:    hiveclient.MigrationRunSucceeded,
			Phases:   normalizationTestPhases(),
			BackupID: "b-1",
			Summary: &hiveclient.MigrationSummary{
				RowsRekeyed:        148,
				SyncPositionsReset: []string{"jarvis-dev", "core-api"},
			},
		}),
	}
	m, _ := pollNormalizationOnce(t, normalizationAtProgress(t, service), service)

	assertContains(t, m.View(),
		"re-check the full history",
		"jarvis-dev, core-api",
		"Nothing is duplicated",
	)
}

func TestNormalizationFailedScreenReportsRollbackReasonAndRetryability(t *testing.T) {
	service := &fakeNormalizationService{
		plans: []hiveclient.MigrationPlan{executablePlan()},
		progresses: noRunThen(hiveclient.MigrationProgress{
			State:     hiveclient.MigrationRunFailed,
			Phase:     "rekey",
			Phases:    normalizationTestPhases(),
			Reason:    "contention",
			Detail:    "another writer held the database",
			Retryable: true,
			BackupID:  "b-1",
		}),
	}
	m, _ := pollNormalizationOnce(t, normalizationAtProgress(t, service), service)

	if m.normalizationStep != normalizationStepFailed {
		t.Fatalf("step = %v, want the failed screen", m.normalizationStep)
	}
	view := m.View()
	assertContains(t, view, "No changes were applied", "rolled back")
	assertContains(t, view, "Another writer held the database", "b-1")
	assertContains(t, view, "can be retried", "retry")
	assertNotContains(t, view, "Hive is unblocked")
}

func TestNormalizationFailedScreenDoesNotOfferRetryWhenNotRetryable(t *testing.T) {
	service := &fakeNormalizationService{
		plans: []hiveclient.MigrationPlan{executablePlan()},
		progresses: noRunThen(hiveclient.MigrationProgress{
			State:     hiveclient.MigrationRunFailed,
			Phases:    normalizationTestPhases(),
			Reason:    "fault",
			Retryable: false,
		}),
	}
	m, _ := pollNormalizationOnce(t, normalizationAtProgress(t, service), service)

	view := m.View()
	assertContains(t, view, "cannot be retried")
	assertNotContains(t, view, "press r to retry")

	// r must not silently start anything.
	before := service.planCalls
	m = sendRune(m, 'r')
	if service.planCalls != before {
		t.Fatalf("plan calls = %d, want %d — retry ran on a non-retryable failure", service.planCalls, before)
	}
}

func TestNormalizationRetryRereadsThePlanWhenRetryable(t *testing.T) {
	service := &fakeNormalizationService{
		plans: []hiveclient.MigrationPlan{executablePlan()},
		progresses: noRunThen(hiveclient.MigrationProgress{
			State: hiveclient.MigrationRunFailed, Phases: normalizationTestPhases(), Reason: "contention", Retryable: true,
		}),
	}
	m, _ := pollNormalizationOnce(t, normalizationAtProgress(t, service), service)

	before := service.planCalls
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("cmd is nil, want a plan re-read on retry")
	}
	updated, _ = updated.Update(cmd())
	if service.planCalls != before+1 {
		t.Fatalf("plan calls = %d, want %d", service.planCalls, before+1)
	}
	if updated.(Model).normalizationStep != normalizationStepOverview {
		t.Fatalf("step = %v, want the overview after a retry re-read", updated.(Model).normalizationStep)
	}
}

func TestNormalizationStalePlanDiscardsThePlanAndRereadsIt(t *testing.T) {
	fresh := executablePlan()
	fresh.PlanFingerprint = "99887766"
	fresh.Confirmation = "NORMALIZE 1 PROJECT 99887766"
	fresh.Groups[0].Records = 200

	service := &fakeNormalizationService{
		plans:      []hiveclient.MigrationPlan{executablePlan(), fresh},
		executeErr: &hiveclient.MigrationRefusalError{StatusCode: 409, State: hiveclient.MigrationStatePlanStale, Detail: "the database changed"},
	}
	m := normalizationAtConfirm(t, service)
	m = sendText(m, executablePlan().Confirmation)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("cmd is nil, want the execute dispatch")
	}
	updated, _ = updated.Update(cmd())
	m = updated.(Model)

	if m.normalizationStep != normalizationStepStale {
		t.Fatalf("step = %v, want the stale-plan screen", m.normalizationStep)
	}
	if m.normalizationPlan != nil {
		t.Fatal("the stale plan was carried forward instead of discarded")
	}
	assertContains(t, m.View(), "Nothing was modified", "out of date")

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("cmd is nil, want enter to re-read the plan")
	}
	updated, _ = updated.Update(cmd())
	m = updated.(Model)

	if service.planCalls != 2 {
		t.Fatalf("plan calls = %d, want 2 (entry plus the re-read)", service.planCalls)
	}
	if m.normalizationStep != normalizationStepOverview {
		t.Fatalf("step = %v, want the overview on the fresh plan", m.normalizationStep)
	}
	assertContains(t, m.View(), "200")
	assertNotContains(t, m.View(), "a1b2c3d4")
}

// --- Screen 7: re-entry while running ---------------------------------------

func TestNormalizationEntryWhileRunningLandsOnTheAlreadyRunningScreen(t *testing.T) {
	started := time.Now().Add(-90 * time.Second)
	service := &fakeNormalizationService{
		plans: []hiveclient.MigrationPlan{executablePlan()},
		progresses: []hiveclient.MigrationProgress{{
			State: hiveclient.MigrationRunRunning, Phase: "rekey", Phases: normalizationTestPhases(), StartedAt: started,
		}},
	}
	m := openNormalization(t, service)

	if m.normalizationStep != normalizationStepAlreadyRunning {
		t.Fatalf("step = %v, want the already-running screen", m.normalizationStep)
	}
	view := m.View()
	assertContains(t, view, "already running", "rekey", "Elapsed", "second run cannot be started")
	// Information, not an error.
	assertNotContains(t, view, "failed", "Error")
}

func TestNormalizationExecuteAlreadyRunningLandsOnTheAlreadyRunningScreen(t *testing.T) {
	service := &fakeNormalizationService{
		plans: []hiveclient.MigrationPlan{executablePlan()},
		progresses: noRunThen(
			hiveclient.MigrationProgress{State: hiveclient.MigrationRunRunning, Phase: "backup", Phases: normalizationTestPhases(), StartedAt: time.Now()},
		),
		executeErr: &hiveclient.MigrationRefusalError{StatusCode: 409, State: hiveclient.MigrationStateAlreadyRunning},
	}
	m := normalizationAtConfirm(t, service)
	m = sendText(m, executablePlan().Confirmation)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("cmd is nil, want the execute dispatch")
	}
	updated, cmd = updated.Update(cmd())
	if cmd == nil {
		t.Fatal("cmd is nil, want a progress read for the running fold")
	}
	updated, _ = updated.Update(cmd())
	m = updated.(Model)

	if m.normalizationStep != normalizationStepAlreadyRunning {
		t.Fatalf("step = %v, want the already-running screen", m.normalizationStep)
	}
	assertContains(t, m.View(), "already running", "backup")
}

// --- Transport failures ------------------------------------------------------

func TestNormalizationPlanTransportFailureSurfacesAMessage(t *testing.T) {
	service := &fakeNormalizationService{planErr: assertErr("dial tcp: connection refused")}
	m := openNormalization(t, service)

	if m.normalizationStep != normalizationStepUnavailable {
		t.Fatalf("step = %v, want the unavailable screen", m.normalizationStep)
	}
	view := m.View()
	assertContains(t, view, "connection refused", "Nothing was changed")
	assertNotContains(t, view, "Hive is unblocked")
}

func TestNormalizationExecuteTransportFailureNeverClaimsSuccess(t *testing.T) {
	service := &fakeNormalizationService{
		plans:      []hiveclient.MigrationPlan{executablePlan()},
		executeErr: assertErr("dial tcp: connection refused"),
	}
	m := normalizationAtConfirm(t, service)
	m = sendText(m, executablePlan().Confirmation)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("cmd is nil, want the execute dispatch")
	}
	updated, _ = updated.Update(cmd())
	m = updated.(Model)

	if m.normalizationStep == normalizationStepSucceeded || m.normalizationStep == normalizationStepProgress {
		t.Fatalf("step = %v, want a screen that does not claim the fold started", m.normalizationStep)
	}
	assertContains(t, m.View(), "connection refused", "Nothing was changed")
}

func TestNormalizationProgressTransportFailureSurfacesAMessageAndKeepsWatching(t *testing.T) {
	service := &fakeNormalizationService{
		plans:            []hiveclient.MigrationPlan{executablePlan()},
		progresses:       noRunThen(),
		progressErr:      assertErr("dial tcp: connection refused"),
		progressErrAfter: 1,
	}
	m := normalizationAtProgress(t, service)
	m, _ = pollNormalizationOnce(t, m, service)

	if m.normalizationStep != normalizationStepProgress {
		t.Fatalf("step = %v, want to stay on progress while the daemon is unreachable", m.normalizationStep)
	}
	assertContains(t, m.View(), "connection refused")
	assertNotContains(t, m.View(), "Hive is unblocked")
}

func TestNormalizationRefusalStatesEachExplainThemselves(t *testing.T) {
	for _, state := range []string{
		hiveclient.MigrationStateConfirmationMismatch,
		hiveclient.MigrationStateNotNeeded,
		hiveclient.MigrationStatePlanUnsafe,
		hiveclient.MigrationStateInvalidRequest,
		hiveclient.MigrationStateRequestFailed,
		hiveclient.MigrationStateFoldUnavailable,
	} {
		t.Run(state, func(t *testing.T) {
			service := &fakeNormalizationService{
				plans:      []hiveclient.MigrationPlan{executablePlan()},
				executeErr: &hiveclient.MigrationRefusalError{StatusCode: 409, State: state, Detail: "daemon detail"},
			}
			m := normalizationAtConfirm(t, service)
			m = sendText(m, executablePlan().Confirmation)
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("cmd is nil, want the execute dispatch")
			}
			updated, _ = updated.Update(cmd())
			m = updated.(Model)

			if m.normalizationStep == normalizationStepSucceeded || m.normalizationStep == normalizationStepProgress {
				t.Fatalf("state %q reached %v, want a refusal screen", state, m.normalizationStep)
			}
			assertContains(t, m.View(), "Nothing was changed")
		})
	}
}

// --- Wiring ------------------------------------------------------------------

func TestNewModelWithConfigWiresTheNormalizationService(t *testing.T) {
	service := &fakeNormalizationService{}
	m := NewModelWithConfig(Snapshot{DashboardState: DashboardHealthy}, nil, nil, nil, nil, nil, nil, nil, service)

	if m.normalizationService == nil {
		t.Fatal("normalizationService is nil, want the injected service")
	}
}

func TestHiveClientSatisfiesProjectNormalizationService(t *testing.T) {
	client, err := hiveclient.New("http://127.0.0.1:7438")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var _ ProjectNormalizationService = client
}
