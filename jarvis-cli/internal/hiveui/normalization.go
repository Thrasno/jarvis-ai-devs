package hiveui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/terminalui"
)

// normalizationActionLabel is the dashboard row that opens this wizard. It is a
// constant because both the row and the pending-block message name it.
const normalizationActionLabel = "Project normalization"

// normalizationPollInterval is how often the live progress screen re-reads the
// daemon. The tick exists only while that screen is open.
const normalizationPollInterval = time.Second

// Phase marks for the progress list. Discrete phases are the whole point: the
// daemon reports which phase it is in and never a percentage, so neither does this.
const (
	normalizationPhaseDoneMark    = "[x]"
	normalizationPhaseCurrentMark = "[>]"
	normalizationPhasePendingMark = "[ ]"
)

// ProjectNormalizationService is the daemon-owned project-identity fold as the
// TUI consumes it. *hiveclient.Client satisfies it.
type ProjectNormalizationService interface {
	MigrationPlan(context.Context) (hiveclient.MigrationPlan, error)
	ExecuteMigration(context.Context, hiveclient.MigrationExecuteRequest) (hiveclient.MigrationExecuteResult, error)
	MigrationProgress(context.Context) (hiveclient.MigrationProgress, error)
}

// normalizationStep is the wizard's screen enum.
type normalizationStep int

const (
	normalizationStepLoading normalizationStep = iota
	normalizationStepOverview
	normalizationStepConflicts
	normalizationStepReview
	normalizationStepConfirm
	normalizationStepProgress
	normalizationStepSucceeded
	normalizationStepFailed
	normalizationStepStale
	normalizationStepAlreadyRunning
	normalizationStepUnavailable
)

// normalizationEntryMsg carries the one read that decides which screen the
// wizard opens on: a fold already running takes priority over the plan.
type normalizationEntryMsg struct {
	plan     hiveclient.MigrationPlan
	progress hiveclient.MigrationProgress
	err      error
}

type normalizationExecuteMsg struct {
	result hiveclient.MigrationExecuteResult
	err    error
}

type normalizationProgressMsg struct {
	progress hiveclient.MigrationProgress
	err      error
}

// normalizationTickMsg is seq-stamped so a tick scheduled by a screen that has
// since been closed is recognized as stale and polls nothing.
type normalizationTickMsg struct {
	seq int
}

// NewModelWithSnapshotAndNormalizationService wires only the normalization
// service, for tests that drive the wizard in isolation.
func NewModelWithSnapshotAndNormalizationService(snapshot Snapshot, service ProjectNormalizationService) Model {
	m := NewModelWithSnapshot(snapshot)
	m.normalizationService = service
	return m
}

// normalizationPending reports whether the daemon is holding writes closed
// waiting for the operator to review the fold.
func normalizationPending(snapshot Snapshot) bool {
	return snapshot.MigrationState == hiveclient.MigrationStatePendingReview
}

// dashboardActionRows decorates the static action list with the normalization
// state. Order and length never change, so the cursor index stays valid.
func (m Model) dashboardActionRows() []dashboardAction {
	rows := dashboardActions()
	pending := normalizationPending(m.snapshot)
	for i := range rows {
		if rows[i].label == normalizationActionLabel {
			rows[i].disabled = !pending
			if pending {
				rows[i].badge = "pending review"
			}
			continue
		}
		if pending {
			// Disabled, never hidden: the operator has to see the data is still there.
			rows[i].disabled = true
		}
	}
	return rows
}

// startNormalizationEntry reads progress and the plan in one command so the
// wizard can open on the already-running screen without a second round trip.
func (m Model) startNormalizationEntry() (tea.Model, tea.Cmd) {
	service := m.normalizationService
	if service == nil {
		return m, nil
	}
	m.normalizationStep = normalizationStepLoading
	m.normalizationPlan = nil
	m.normalizationProgress = nil
	m.normalizationConfirm = ""
	m.normalizationDetail = ""
	m.message = ""
	return m, func() tea.Msg {
		ctx := context.Background()
		progress, err := service.MigrationProgress(ctx)
		if err != nil {
			return normalizationEntryMsg{err: err}
		}
		if progress.State == hiveclient.MigrationRunRunning {
			return normalizationEntryMsg{progress: progress}
		}
		plan, err := service.MigrationPlan(ctx)
		if err != nil {
			return normalizationEntryMsg{progress: progress, err: err}
		}
		return normalizationEntryMsg{plan: plan, progress: progress}
	}
}

func (m Model) applyNormalizationEntry(msg normalizationEntryMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.normalizationStep = normalizationStepUnavailable
		m.normalizationDetail = normalizationFailureDetail(msg.err)
		return m, nil
	}
	progress := msg.progress
	if progress.State == hiveclient.MigrationRunRunning {
		m.normalizationProgress = &progress
		m.normalizationStep = normalizationStepAlreadyRunning
		return m, nil
	}
	plan := msg.plan
	m.normalizationPlan = &plan
	m.normalizationStep = normalizationStepOverview
	return m, nil
}

func (m Model) updateProjectNormalization(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.normalizationSubmitting {
		// The execute request is in flight. Swallow keys so a second enter cannot
		// authorize the same fold twice.
		return m, nil
	}
	switch key.Type {
	case tea.KeyEsc, tea.KeyBackspace:
		if m.normalizationStep == normalizationStepConfirm && m.normalizationConfirm != "" {
			m.normalizationConfirm = trimLastRune(m.normalizationConfirm)
			return m, nil
		}
		return m.back(), nil
	case tea.KeyTab, tea.KeySpace:
		if m.normalizationStep == normalizationStepConfirm {
			m.normalizationConfirm += " "
		}
		return m, nil
	case tea.KeyRunes:
		if m.normalizationStep == normalizationStepConfirm {
			m.normalizationConfirm += sanitizeInputText(string(key.Runes))
			return m, nil
		}
		if runeKey(key, 'r') && m.normalizationStep == normalizationStepFailed && m.normalizationRetryable() {
			return m.startNormalizationEntry()
		}
		return m, nil
	case tea.KeyEnter:
		return m.submitNormalizationStep()
	}
	return m, nil
}

func (m Model) normalizationRetryable() bool {
	return m.normalizationProgress != nil && m.normalizationProgress.Retryable
}

func (m Model) submitNormalizationStep() (tea.Model, tea.Cmd) {
	m.message = ""
	switch m.normalizationStep {
	case normalizationStepOverview:
		if m.normalizationPlan == nil {
			return m, nil
		}
		if !m.normalizationPlan.Executable {
			// The dead end is structural: there is no transition out of it forward.
			m.normalizationStep = normalizationStepConflicts
			return m, nil
		}
		m.normalizationStep = normalizationStepReview
		return m, nil

	case normalizationStepReview:
		m.normalizationStep = normalizationStepConfirm
		return m, nil

	case normalizationStepConfirm:
		return m.dispatchNormalization()

	case normalizationStepStale, normalizationStepFailed:
		// The reviewed plan is gone; re-read it from the start.
		return m.startNormalizationEntry()

	case normalizationStepSucceeded, normalizationStepUnavailable, normalizationStepAlreadyRunning:
		return m.back(), nil
	}
	// normalizationStepConflicts and normalizationStepProgress have no forward move.
	return m, nil
}

func (m Model) dispatchNormalization() (tea.Model, tea.Cmd) {
	plan := m.normalizationPlan
	if plan == nil {
		m.normalizationStep = normalizationStepStale
		return m, nil
	}
	// The daemon compares the phrase exactly; it is echoed, never rebuilt here.
	if !confirmationMatches(m.normalizationConfirm, plan.Confirmation) {
		m.message = confirmationMismatchMessage
		return m, nil
	}
	service := m.normalizationService
	if service == nil {
		m.normalizationStep = normalizationStepUnavailable
		m.normalizationDetail = "The normalization service is not wired into this TUI."
		return m, nil
	}
	req := hiveclient.MigrationExecuteRequest{
		PlanFingerprint: plan.PlanFingerprint,
		Confirmation:    plan.Confirmation,
	}
	m.normalizationSubmitting = true
	return m, func() tea.Msg {
		result, err := service.ExecuteMigration(context.Background(), req)
		return normalizationExecuteMsg{result: result, err: err}
	}
}

func (m Model) applyNormalizationExecute(msg normalizationExecuteMsg) (tea.Model, tea.Cmd) {
	if !m.normalizationSubmitting {
		return m, nil
	}
	m.normalizationSubmitting = false

	var refusal *hiveclient.MigrationRefusalError
	if errors.As(msg.err, &refusal) {
		switch refusal.State {
		case hiveclient.MigrationStatePlanStale:
			// Discard the plan: it describes a database that no longer exists.
			m.normalizationPlan = nil
			m.normalizationConfirm = ""
			m.normalizationStep = normalizationStepStale
			return m, nil
		case hiveclient.MigrationStateAlreadyRunning:
			return m.startNormalizationProgressRead()
		case hiveclient.MigrationStateConfirmationMismatch:
			m.normalizationConfirm = ""
			m.message = "The daemon rejected the confirmation phrase. Nothing was changed. Type the phrase exactly as shown."
			return m, nil
		}
		m.normalizationStep = normalizationStepUnavailable
		m.normalizationDetail = normalizationRefusalDetail(refusal)
		return m, nil
	}
	if msg.err != nil {
		m.normalizationStep = normalizationStepUnavailable
		m.normalizationDetail = normalizationFailureDetail(msg.err)
		return m, nil
	}
	if msg.result.State != hiveclient.MigrationStateFoldAccepted {
		m.normalizationStep = normalizationStepUnavailable
		m.normalizationDetail = "Nothing was changed. The daemon answered " + msg.result.State + " instead of accepting the fold."
		return m, nil
	}
	m.normalizationStep = normalizationStepProgress
	m.normalizationPollSeq++
	return m, m.pollNormalizationProgress()
}

// startNormalizationProgressRead reads progress once, without starting a poll.
func (m Model) startNormalizationProgressRead() (tea.Model, tea.Cmd) {
	return m, m.pollNormalizationProgress()
}

func (m Model) pollNormalizationProgress() tea.Cmd {
	service := m.normalizationService
	if service == nil {
		return nil
	}
	return func() tea.Msg {
		progress, err := service.MigrationProgress(context.Background())
		return normalizationProgressMsg{progress: progress, err: err}
	}
}

func normalizationTick(seq int) tea.Cmd {
	return tea.Tick(normalizationPollInterval, func(time.Time) tea.Msg {
		return normalizationTickMsg{seq: seq}
	})
}

// applyNormalizationTick polls only for the screen that scheduled the tick. A
// tick that outlived its screen — the operator left, or the run finished — is
// dropped, so the daemon is never polled off-screen.
func (m Model) applyNormalizationTick(msg normalizationTickMsg) (tea.Model, tea.Cmd) {
	if m.screen != ScreenProjectNormalization ||
		m.normalizationStep != normalizationStepProgress ||
		msg.seq != m.normalizationPollSeq {
		return m, nil
	}
	return m, m.pollNormalizationProgress()
}

func (m Model) applyNormalizationProgress(msg normalizationProgressMsg) (tea.Model, tea.Cmd) {
	if m.screen != ScreenProjectNormalization {
		return m, nil
	}
	if msg.err != nil {
		// The fold keeps running in the daemon; say so instead of claiming an outcome.
		m.message = "Cannot read progress from hive-daemon: " + msg.err.Error() + ". The fold keeps running; this screen retries."
		if m.normalizationStep == normalizationStepProgress {
			return m, normalizationTick(m.normalizationPollSeq)
		}
		return m, nil
	}
	progress := msg.progress
	m.normalizationProgress = &progress
	m.message = ""

	switch progress.State {
	case hiveclient.MigrationRunSucceeded:
		m.normalizationStep = normalizationStepSucceeded
		m.normalizationPollSeq++
		return m, nil
	case hiveclient.MigrationRunFailed:
		m.normalizationStep = normalizationStepFailed
		m.normalizationPollSeq++
		return m, nil
	case hiveclient.MigrationRunRunning:
		if m.normalizationStep != normalizationStepProgress {
			m.normalizationStep = normalizationStepAlreadyRunning
			return m, nil
		}
		return m, normalizationTick(m.normalizationPollSeq)
	}
	if m.normalizationStep == normalizationStepProgress {
		return m, normalizationTick(m.normalizationPollSeq)
	}
	return m, nil
}

func normalizationFailureDetail(err error) string {
	return "Nothing was changed.\nhive-daemon could not be reached:\n" + err.Error()
}

func normalizationRefusalDetail(refusal *hiveclient.MigrationRefusalError) string {
	reason := ""
	switch refusal.State {
	case hiveclient.MigrationStateNotNeeded:
		reason = "The daemon reports there is nothing left to normalize.\n" +
			"Reopen jarvis hive to see the current state."
	case hiveclient.MigrationStatePlanUnsafe:
		reason = "The daemon refused this plan as unsafe to run.\n" +
			"Check jarvis hive → Warnings for diagnostics."
	case hiveclient.MigrationStateInvalidRequest:
		reason = "The daemon rejected the request as malformed.\n" +
			"This is a bug in this client."
	case hiveclient.MigrationStateRequestFailed:
		reason = "The daemon hit an internal error accepting the fold."
	case hiveclient.MigrationStateFoldUnavailable:
		reason = "This daemon build cannot run the fold.\n" +
			"Update hive-daemon and try again."
	default:
		reason = "The daemon refused the fold."
	}
	detail := "Nothing was changed.\n" + reason
	if strings.TrimSpace(refusal.Detail) != "" {
		detail += "\n" + dimTextStyle.Render("daemon detail: "+refusal.Detail)
	}
	detail += "\n" + dimTextStyle.Render("raw state: "+refusal.State)
	return detail
}

// normalizationDisplaySourceText names where the surviving display name came
// from in words an operator can act on.
func normalizationDisplaySourceText(source string) string {
	switch source {
	case "remote":
		return "the team server"
	case "oldest-registration":
		return "the oldest local registration"
	default:
		return source
	}
}

// normalizationConflictText translates a machine conflict kind. The raw kind is
// still rendered next to it so a bug report can carry it.
func normalizationConflictText(kind string) string {
	switch kind {
	case "divergent-global-entity":
		return "the same record exists twice with different content"
	case "divergent-sync-state":
		return "the two names recorded different sync state"
	case "incompatible-session-sentinel":
		return "the two names recorded incompatible session markers"
	case "contradictory-alias":
		return "the two names point at contradictory aliases"
	case "non-monotonic-cursor-protocol":
		return "the sync cursors cannot be ordered safely"
	case "contradictory-governance-head":
		return "the two names recorded contradictory history"
	case "broken-reference":
		return "a link points at a row that is missing"
	default:
		return "an unrecognized disagreement — see the raw kind"
	}
}

func normalizationReasonText(progress hiveclient.MigrationProgress) string {
	switch progress.Reason {
	case "contention":
		return "Another writer held the database while normalization ran."
	case "interrupted":
		return "The daemon stopped before normalization finished."
	case "fault":
		return "Normalization hit an internal error."
	case "":
		return "The daemon did not report a reason."
	default:
		return "The daemon reported: " + progress.Reason
	}
}

// normalizationSurvivingName is the spelling that is left after the fold: the
// canonical variant when one is already stored, and the canonical key otherwise.
// It is read from the Canonical flag, never derived by comparing strings.
func normalizationSurvivingName(group hiveclient.MigrationPlanGroup) (string, bool) {
	for _, variant := range group.Variants {
		if variant.Canonical {
			return variant.Spelling, true
		}
	}
	return group.Key, false
}

func normalizationRewrittenNames(group hiveclient.MigrationPlanGroup) []string {
	names := make([]string, 0, len(group.Variants))
	for _, variant := range group.Variants {
		if !variant.Canonical {
			names = append(names, variant.Spelling)
		}
	}
	return names
}

func normalizationTotalRecords(plan hiveclient.MigrationPlan) int {
	total := 0
	for _, group := range plan.Groups {
		total += group.Records
	}
	return total
}

// --- rendering ---------------------------------------------------------------

func (m Model) projectNormalizationView() string {
	w := max(m.width, 80)
	panelW := terminalui.PanelWidth(w)

	var sb strings.Builder
	crumb := breadcrumbStyle.Render("dashboard / ") + breadcrumbCurrent.Render("project normalization")
	sb.WriteString(terminalui.HeaderRow(crumb, badgeWarning.Render("normalization"), w))
	sb.WriteString("\n")

	switch m.normalizationStep {
	case normalizationStepLoading:
		sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("LOADING", panelW)+
			dimTextStyle.Render("Reading the normalization plan from hive-daemon."), panelW))
	case normalizationStepOverview:
		m.renderNormalizationOverviewPanels(&sb, panelW)
	case normalizationStepConflicts:
		m.renderNormalizationConflictsPanel(&sb, panelW)
	case normalizationStepReview:
		m.renderNormalizationReviewPanels(&sb, panelW)
	case normalizationStepConfirm:
		m.renderNormalizationConfirmPanel(&sb, panelW)
	case normalizationStepProgress:
		m.renderNormalizationProgressPanel(&sb, panelW)
	case normalizationStepSucceeded:
		m.renderNormalizationSucceededPanels(&sb, panelW)
	case normalizationStepFailed:
		m.renderNormalizationFailedPanel(&sb, panelW)
	case normalizationStepStale:
		m.renderNormalizationStalePanel(&sb, panelW)
	case normalizationStepAlreadyRunning:
		m.renderNormalizationAlreadyRunningPanel(&sb, panelW)
	case normalizationStepUnavailable:
		sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("NORMALIZATION NOT STARTED", panelW)+
			m.normalizationDetail, panelW))
	}

	if m.message != "" {
		fmt.Fprintf(&sb, "\n%s\n", m.message)
	}
	sb.WriteString("\n")
	sb.WriteString(helpBar(m.normalizationHelpHints(), "normalization", w))
	return sb.String()
}

func (m Model) normalizationHelpHints() []KeyHint {
	switch m.normalizationStep {
	case normalizationStepOverview, normalizationStepReview:
		return []KeyHint{{"enter", "continue"}, {"esc", "back"}}
	case normalizationStepConfirm:
		return []KeyHint{{"enter", "run normalization"}, {"esc", "back"}}
	case normalizationStepConflicts:
		return []KeyHint{{"esc", "back"}}
	case normalizationStepProgress:
		return []KeyHint{{"esc", "close (keeps running)"}}
	case normalizationStepFailed:
		if m.normalizationRetryable() {
			return []KeyHint{{"r", "retry"}, {"enter", "re-read plan"}, {"esc", "back"}}
		}
		return []KeyHint{{"esc", "back"}}
	case normalizationStepStale:
		return []KeyHint{{"enter", "re-read plan"}, {"esc", "back"}}
	default:
		return []KeyHint{{"enter", "done"}, {"esc", "back"}}
	}
}

func (m Model) renderNormalizationWhyPanel(sb *strings.Builder, panelW int) {
	why := "The same project was recorded under different spellings.\n" +
		"Hive indexes memories by project name, so its data is split\n" +
		"across those names.\n" +
		"Writes stay blocked until this is resolved. That is why\n" +
		"nothing new is written under an ambiguous identity."
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("WHY HIVE IS BLOCKED", panelW)+why, panelW))
	sb.WriteString("\n")
}

func (m Model) renderNormalizationOverviewPanels(sb *strings.Builder, panelW int) {
	m.renderNormalizationWhyPanel(sb, panelW)

	plan := m.normalizationPlan
	if plan == nil {
		return
	}

	var table strings.Builder
	table.WriteString(columnHeaderStyle.Render("PROJECT  SPELLINGS  RECORDS  DISPLAY NAME FROM") + "\n")
	for _, group := range plan.Groups {
		table.WriteString(fmt.Sprintf("%s  %d  %d  %s\n",
			group.Display, len(group.Variants), group.Records,
			normalizationDisplaySourceText(group.DisplaySource)))
		rewritten := normalizationRewrittenNames(group)
		if len(rewritten) == 0 {
			table.WriteString("  " + dimTextStyle.Render("rewritten: none") + "\n")
		} else {
			table.WriteString("  " + dimTextStyle.Render("rewritten: "+strings.Join(rewritten, ", ")) + "\n")
		}
		surviving, stored := normalizationSurvivingName(group)
		table.WriteString("  survives: " + surviving + "\n")
		if !stored {
			table.WriteString("    " + dimTextStyle.Render("new name — no stored row uses it yet") + "\n")
		}
	}
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("PROJECT IDENTITIES", panelW)+table.String(), panelW))
	sb.WriteString("\n")

	decisions := "No decisions needed."
	if count := len(plan.Conflicts); count > 0 {
		noun := "conflicts need"
		if count == 1 {
			noun = "conflict needs"
		}
		decisions = fmt.Sprintf("%d %s a decision before normalization can run.", count, noun)
	}
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("DECISIONS", panelW)+decisions, panelW))
}

func (m Model) renderNormalizationConflictsPanel(sb *strings.Builder, panelW int) {
	plan := m.normalizationPlan
	var content strings.Builder
	content.WriteString(columnHeaderStyle.Render("TABLE  KEY") + "\n")
	if plan != nil {
		for _, conflict := range plan.Conflicts {
			content.WriteString(conflict.Table + "  " + conflict.Key + "\n")
			content.WriteString("  " + normalizationConflictText(conflict.Kind) + "\n")
			content.WriteString("  " + dimTextStyle.Render("raw kind: "+conflict.Kind) + "\n")
			content.WriteString("  " + dimTextStyle.Render("identity: "+conflict.Identity) + "\n")
		}
	}
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("CONFLICTS", panelW)+content.String(), panelW))
	sb.WriteString("\n")

	dead := "Resolving these conflicts is not supported here yet.\n" +
		"Normalization cannot run until they are resolved.\n" +
		"Nothing was changed and your data is intact.\n" +
		"Open jarvis hive → Warnings for diagnostics on these rows."
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("WHAT YOU CAN DO", panelW)+dead, panelW))
}

func (m Model) renderNormalizationReviewPanels(sb *strings.Builder, panelW int) {
	plan := m.normalizationPlan
	if plan == nil {
		return
	}
	var content strings.Builder
	for _, group := range plan.Groups {
		surviving, _ := normalizationSurvivingName(group)
		rewritten := normalizationRewrittenNames(group)
		content.WriteString(fmt.Sprintf("%s → %s  (%d records)\n",
			strings.Join(rewritten, ", "), surviving, group.Records))
	}
	content.WriteString(fmt.Sprintf("\nRecords to be modified: %d\n", normalizationTotalRecords(*plan)))
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("PLAN", panelW)+content.String(), panelW))
	sb.WriteString("\n")

	backup := "A backup is taken before anything changes.\n" +
		"It is the first phase and it is mandatory.\n" +
		"Every phase runs in one transaction: if any phase fails, the\n" +
		"whole transaction is rolled back and nothing is left changed.\n" +
		"The backup id is reported when normalization finishes, so\n" +
		"you can restore it later."
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("BACKUP AND ROLLBACK", panelW)+backup, panelW))
	sb.WriteString("\n")

	team := "This is not a purely local operation.\n" +
		"The rename is reported to the team server, so synced\n" +
		"memories do not stay split under the old names there."
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("TEAM SERVER", panelW)+team, panelW))
}

func (m Model) renderNormalizationConfirmPanel(sb *strings.Builder, panelW int) {
	plan := m.normalizationPlan
	if plan == nil {
		return
	}
	content := fmt.Sprintf("Records to be modified: %d\n\nType exactly to confirm: %s\n\nconfirmation: %s\n",
		normalizationTotalRecords(*plan),
		plan.Confirmation,
		confirmationField(m.normalizationConfirm, plan.Confirmation),
	)
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("CONFIRM", panelW)+content, panelW))
}

// normalizationPhases prefers the live run's phase list and falls back to the
// plan's, so the layout exists before the first progress read lands.
func (m Model) normalizationPhases() []string {
	if m.normalizationProgress != nil && len(m.normalizationProgress.Phases) > 0 {
		return m.normalizationProgress.Phases
	}
	if m.normalizationPlan != nil {
		return m.normalizationPlan.Phases
	}
	return nil
}

func (m Model) renderNormalizationPhaseList(content *strings.Builder) {
	current := ""
	if m.normalizationProgress != nil {
		current = m.normalizationProgress.Phase
	}
	phases := m.normalizationPhases()
	currentIndex := -1
	for i, phase := range phases {
		if phase == current {
			currentIndex = i
			break
		}
	}
	for i, phase := range phases {
		switch {
		case currentIndex >= 0 && i < currentIndex:
			content.WriteString(normalizationPhaseDoneMark + " " + phase + "\n")
		case currentIndex >= 0 && i == currentIndex:
			content.WriteString(cursorStyle.Render(normalizationPhaseCurrentMark+" "+phase) + "\n")
		default:
			content.WriteString(dimTextStyle.Render(normalizationPhasePendingMark+" "+phase) + "\n")
		}
	}
}

func (m Model) renderNormalizationProgressPanel(sb *strings.Builder, panelW int) {
	var content strings.Builder
	m.renderNormalizationPhaseList(&content)
	content.WriteString("\n" + dimTextStyle.Render("Closing this screen is safe.") + "\n")
	content.WriteString(dimTextStyle.Render("hive-daemon tracks progress and keeps running.") + "\n")
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("PROGRESS", panelW)+content.String(), panelW))
}

func (m Model) renderNormalizationSucceededPanels(sb *strings.Builder, panelW int) {
	progress := m.normalizationProgress
	var content strings.Builder
	if progress != nil && progress.Summary != nil {
		summary := progress.Summary
		content.WriteString(fmt.Sprintf("Rows rekeyed: %d\n", summary.RowsRekeyed))
		content.WriteString(fmt.Sprintf("Sessions requeued: %d\n", summary.SessionsRequeued))
		content.WriteString(fmt.Sprintf("Prompts requeued: %d\n", summary.PromptsRequeued))
		content.WriteString(fmt.Sprintf("Reprojections enqueued: %d\n", summary.ReprojectsEnqueued))
	}
	content.WriteString("\nHive is unblocked: writes are accepted again.\n")
	if progress != nil && progress.BackupID != "" {
		content.WriteString("Backup: " + progress.BackupID + "\n")
		content.WriteString(dimTextStyle.Render("To undo this: restore backup "+progress.BackupID) + "\n")
		content.WriteString(dimTextStyle.Render("from jarvis hive → Backup snapshots.") + "\n")
	}
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("NORMALIZATION COMPLETE", panelW)+content.String(), panelW))

	if progress == nil || progress.Summary == nil || len(progress.Summary.SyncPositionsReset) == 0 {
		return
	}
	sb.WriteString("\n")
	notice := "The next sync will re-check the full history of:\n" +
		strings.Join(progress.Summary.SyncPositionsReset, ", ") + "\n" +
		"That is expected and it is the safe path.\n" +
		"Nothing is duplicated — the re-check only confirms what is\n" +
		"already stored, so a large re-download is not a fault."
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("NEXT SYNC", panelW)+notice, panelW))
}

func (m Model) renderNormalizationFailedPanel(sb *strings.Builder, panelW int) {
	progress := m.normalizationProgress
	var content strings.Builder
	content.WriteString("No changes were applied — the transaction was rolled back.\n\n")
	if progress != nil {
		content.WriteString(normalizationReasonText(*progress) + "\n")
		if strings.TrimSpace(progress.Detail) != "" {
			content.WriteString(dimTextStyle.Render("daemon detail: "+progress.Detail) + "\n")
		}
		if progress.BackupID != "" {
			content.WriteString("Backup: " + progress.BackupID + "\n")
		}
		content.WriteString("\n")
		if progress.Retryable {
			content.WriteString("This failure can be retried.\n")
			content.WriteString("Press r to retry — it re-reads the plan first.\n")
		} else {
			content.WriteString("This failure cannot be retried automatically.\n")
			content.WriteString(dimTextStyle.Render("Check jarvis hive → Warnings, then report the") + "\n")
			content.WriteString(dimTextStyle.Render("daemon detail above.") + "\n")
		}
	}
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("NORMALIZATION DID NOT COMPLETE", panelW)+content.String(), panelW))
}

func (m Model) renderNormalizationStalePanel(sb *strings.Builder, panelW int) {
	content := "Nothing was modified.\n\n" +
		"The plan you reviewed is out of date: the database changed\n" +
		"since it was read.\n" +
		"Press enter to re-read the plan and review it again."
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("PLAN OUT OF DATE", panelW)+content, panelW))
}

func (m Model) renderNormalizationAlreadyRunningPanel(sb *strings.Builder, panelW int) {
	progress := m.normalizationProgress
	var content strings.Builder
	content.WriteString("A normalization is already running in hive-daemon.\n\n")
	if progress != nil {
		if progress.Phase != "" {
			content.WriteString("Current phase: " + progress.Phase + "\n")
		}
		if !progress.StartedAt.IsZero() {
			content.WriteString("Elapsed: " + time.Since(progress.StartedAt).Round(time.Second).String() + "\n")
		}
		content.WriteString("\n")
		m.renderNormalizationPhaseList(&content)
	}
	content.WriteString("\n" + dimTextStyle.Render("A second run cannot be started while this one is in flight.") + "\n")
	content.WriteString(dimTextStyle.Render("Reopen this screen to check on it.") + "\n")
	sb.WriteString(terminalui.BorderedPanel(terminalui.SectionHeader("ALREADY RUNNING", panelW)+content.String(), panelW))
}
