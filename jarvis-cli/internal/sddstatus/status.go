package sddstatus

import (
	"regexp"
	"strings"
)

const StatusSchema = "jarvis.sdd-status"

// ArtifactState represents how complete an SDD artifact is.
type ArtifactState string

const (
	ArtifactMissing ArtifactState = "missing"
	ArtifactPartial ArtifactState = "partial"
	ArtifactDone    ArtifactState = "done"
)

type ActionMode string

const (
	ActionModeWorkspaceEdit     ActionMode = "workspace-edit"
	ActionModeWorkspacePlanning ActionMode = "workspace-planning"
)

// DependencyState represents whether a phase's required inputs are satisfied.
type DependencyState string

const (
	DepBlocked DependencyState = "blocked"
	DepReady   DependencyState = "ready"
	DepAllDone DependencyState = "all_done"
)

// Artifact name constants — short keys used in maps and Hive topic paths.
// Hive topic key: "sdd/{change}/{artifact}".
const (
	ArtifactExplore       = "explore"
	ArtifactProposal      = "proposal"
	ArtifactSpec          = "spec"
	ArtifactDesign        = "design"
	ArtifactTasks         = "tasks"
	ArtifactApplyProgress = "apply-progress"
	ArtifactVerifyReport  = "verify-report"
	ArtifactArchiveReport = "archive-report"
)

// Phase name constants — must match the canonical names in sddruntime.DefaultContract.
const (
	PhaseExplore = "sdd-explore"
	PhasePropose = "sdd-propose"
	PhaseSpec    = "sdd-spec"
	PhaseDesign  = "sdd-design"
	PhaseTasks   = "sdd-tasks"
	PhaseApply   = "sdd-apply"
	PhaseVerify  = "sdd-verify"
	PhaseArchive = "sdd-archive"
)

// PhaseOrder is the canonical phase execution order used for routing decisions.
var PhaseOrder = []string{
	PhaseExplore,
	PhasePropose,
	PhaseSpec,
	PhaseDesign,
	PhaseTasks,
	PhaseApply,
	PhaseVerify,
	PhaseArchive,
}

// PhaseOutput maps each phase to the artifact it produces.
var PhaseOutput = map[string]string{
	PhaseExplore: ArtifactExplore,
	PhasePropose: ArtifactProposal,
	PhaseSpec:    ArtifactSpec,
	PhaseDesign:  ArtifactDesign,
	PhaseTasks:   ArtifactTasks,
	PhaseApply:   ArtifactApplyProgress,
	PhaseVerify:  ArtifactVerifyReport,
	PhaseArchive: ArtifactArchiveReport,
}

// PhaseRequiredDeps maps each phase to the artifacts it strictly requires before starting.
// Explore and propose have no hard requirements. Optional inputs are handled in special logic.
var PhaseRequiredDeps = map[string][]string{
	PhaseExplore: {},
	PhasePropose: {},
	PhaseSpec:    {ArtifactProposal},
	PhaseDesign:  {ArtifactProposal},
	PhaseTasks:   {ArtifactSpec, ArtifactDesign},
	PhaseApply:   {ArtifactTasks, ArtifactSpec, ArtifactDesign},
	PhaseVerify:  {ArtifactSpec, ArtifactTasks},
	PhaseArchive: {ArtifactSpec, ArtifactDesign, ArtifactTasks, ArtifactApplyProgress, ArtifactVerifyReport},
}

// verifyBlockPatterns are regex patterns that indicate a failing or unclear verify report.
// Archive is blocked if any pattern matches the verify-report content after negated forms are stripped.
// The "failed" pattern requires a non-zero leading digit so "0 failed" does not match.
// "blockers?" and "failures?" use word boundaries; zero-count forms are handled by verifyNegatedForms.
var verifyBlockPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bcritical\b`),
	regexp.MustCompile(`(?i)\bblockers?\b`),
	regexp.MustCompile(`(?i)\bfailures?\b`),
	regexp.MustCompile(`(?i)[1-9]\d*\s+failed\b`),
	regexp.MustCompile(`(?i)\bpending\b`),
	regexp.MustCompile(`(?i)\buntested\b`),
}

// verifyNegatedForms matches phrases that indicate a passing state despite containing
// blocking keywords. Two forms are handled:
//   - Zero/none quantified terms: "0 failures", "no blockers", "no pending items", "zero untested"
//   - Hyphenated negations: "non-critical"
//
// These are stripped from the verify-report content before verifyBlockPatterns are applied,
// preventing false-positive archive blocks on common summary phrases.
var verifyNegatedForms = regexp.MustCompile(`(?i)(?:\b(?:0|no|zero)\s+(?:failures?|blockers?|pending|untested)\b|\bnon-critical\b)`)

var (
	rxCheckedBox   = regexp.MustCompile(`(?i)\[x\]`)
	rxUncheckedBox = regexp.MustCompile(`\[ \]`)

	// rxApplyDecisionRequired matches "Decision needed before apply: Yes" with word boundary
	// to avoid prefix collisions like "Yesterday". Case-insensitive for robustness.
	rxApplyDecisionRequired = regexp.MustCompile(`(?i)Decision needed before apply:\s*Yes\b`)
	// rxApplyDecisionNo matches the explicit "No" resolution token. Word boundary prevents
	// matching "None" or other words that start with "No".
	rxApplyDecisionNo = regexp.MustCompile(`(?i)Decision needed before apply:\s*No\b`)
)

// ApplyDecision reports whether an unresolved delivery decision blocks apply.
// Required is true when the tasks artifact declares "Decision needed before apply: Yes".
// Resolved is true when a resolution token is present (explicit No, a non-pending chain
// strategy, or an accepted size exception). Apply is blocked only when Required && !Resolved.
type ApplyDecision struct {
	Required bool `json:"required"`
	Resolved bool `json:"resolved"`
}

// TaskProgress holds parsed task completion counts from a tasks artifact.
type TaskProgress struct {
	Total     int  `json:"total"`
	Completed int  `json:"completed"`
	AllDone   bool `json:"allDone"`
}

// ApplyState describes the state of iterative apply execution.
type ApplyState struct {
	HasProgress bool `json:"hasProgress"`
	// Complete is true when a verify-report exists, implying apply was far enough along to verify.
	Complete bool `json:"complete"`
}

// ActionContext describes how phase agents may use the current workspace.
type ActionContext struct {
	Mode             ActionMode `json:"mode"`
	AllowedEditRoots []string   `json:"allowedEditRoots,omitempty"`
}

// PhaseRelationship describes the artifact dependency contract for a phase.
type PhaseRelationship struct {
	Phase          string   `json:"phase"`
	OutputArtifact string   `json:"outputArtifact"`
	Requires       []string `json:"requires,omitempty"`
}

// ChangeStatus is the stable JSON contract for SDD phase routing.
// Schema: "jarvis.sdd-status".
type ChangeStatus struct {
	Schema            string                     `json:"schema"`
	ChangeName        string                     `json:"changeName"`
	ArtifactStore     string                     `json:"artifactStore"`
	PlanningHome      string                     `json:"planningHome"`
	ChangeRoot        string                     `json:"changeRoot"`
	ArtifactPaths     map[string]string          `json:"artifactPaths"`
	ContextFiles      map[string]string          `json:"contextFiles"`
	Artifacts         map[string]ArtifactState   `json:"artifacts"`
	Dependencies      map[string]DependencyState `json:"dependencies"`
	ActionContext     ActionContext              `json:"actionContext"`
	AllowedEditRoots  []string                   `json:"allowedEditRoots,omitempty"`
	Relationships     []PhaseRelationship        `json:"relationships"`
	PhaseInstructions map[string]string          `json:"phaseInstructions"`
	TaskProgress      *TaskProgress              `json:"taskProgress,omitempty"`
	ApplyState        *ApplyState                `json:"applyState,omitempty"`
	ApplyDecision     *ApplyDecision             `json:"applyDecision,omitempty"`
	NextRecommended   string                     `json:"nextRecommended"`
	BlockedReasons    []string                   `json:"blockedReasons"`
}

// Input is the observable artifact state for a change, used as input to ComputeStatus.
type Input struct {
	// Artifacts maps artifact name to state. Missing keys are treated as ArtifactMissing.
	Artifacts map[string]ArtifactState
	// Contents maps artifact name to full text, used for content-based checks.
	// Absent entries disable content-based checks for that artifact.
	Contents map[string]string
	// PlanningHome identifies the planning artifact home. Defaults to sdd/{changeName}.
	PlanningHome string
	// ChangeRoot identifies the root for this change's artifacts. Defaults to PlanningHome.
	ChangeRoot string
	// ContextFiles maps logical artifact names to paths/topics a phase should read.
	// Defaults to ArtifactPaths.
	ContextFiles map[string]string
	// ActionMode describes whether agents may edit the workspace or should only plan.
	// Defaults to workspace-planning unless allowed edit roots are provided.
	ActionMode ActionMode
	// AllowedEditRoots lists roots that phase agents may edit in workspace-edit mode.
	AllowedEditRoots []string
	// PhaseInstructions maps phases to the slash command used to invoke them.
	// Defaults to /{phase} {changeName}.
	PhaseInstructions map[string]string
}

// ComputeStatus derives a ChangeStatus from the observed artifact states and contents.
// It is pure and deterministic — no I/O.
func ComputeStatus(changeName, artifactStore string, in Input) *ChangeStatus {
	if in.Artifacts == nil {
		in.Artifacts = map[string]ArtifactState{}
	}
	if in.Contents == nil {
		in.Contents = map[string]string{}
	}

	artifactPaths := buildArtifactPaths(changeName)
	planningHome := defaultString(in.PlanningHome, buildPlanningHome(changeName))
	changeRoot := defaultString(in.ChangeRoot, planningHome)
	contextFiles := copyStringMap(in.ContextFiles)
	if len(contextFiles) == 0 {
		contextFiles = copyStringMap(artifactPaths)
	}
	allowedEditRoots := compactStringSlice(in.AllowedEditRoots)
	actionMode := in.ActionMode
	if len(allowedEditRoots) == 0 {
		actionMode = ActionModeWorkspacePlanning
	} else if actionMode == "" {
		actionMode = ActionModeWorkspaceEdit
	} else if actionMode != ActionModeWorkspaceEdit {
		allowedEditRoots = nil
	}
	phaseInstructions := copyStringMap(in.PhaseInstructions)
	if len(phaseInstructions) == 0 {
		phaseInstructions = buildPhaseInstructions(changeName)
	}
	taskProgress := parseTaskProgress(in.Contents[ArtifactTasks])
	applyDecision := parseApplyDecision(in.Contents[ArtifactTasks])
	applyState := buildApplyState(in.Artifacts)
	dependencies := computeDependencies(in.Artifacts, taskProgress, applyDecision, in.Contents[ArtifactVerifyReport])
	nextRecommended, blockedReasons := computeNextAndBlockers(in.Artifacts, dependencies, applyDecision, in.Contents[ArtifactVerifyReport])

	return &ChangeStatus{
		Schema:        StatusSchema,
		ChangeName:    changeName,
		ArtifactStore: artifactStore,
		PlanningHome:  planningHome,
		ChangeRoot:    changeRoot,
		ArtifactPaths: artifactPaths,
		ContextFiles:  contextFiles,
		Artifacts:     normalizeArtifacts(in.Artifacts),
		Dependencies:  dependencies,
		ActionContext: ActionContext{
			Mode:             actionMode,
			AllowedEditRoots: copyStringSlice(allowedEditRoots),
		},
		AllowedEditRoots:  allowedEditRoots,
		Relationships:     buildPhaseRelationships(),
		PhaseInstructions: phaseInstructions,
		TaskProgress:      taskProgress,
		ApplyState:        applyState,
		ApplyDecision:     applyDecision,
		NextRecommended:   nextRecommended,
		BlockedReasons:    blockedReasons,
	}
}

func buildPlanningHome(changeName string) string {
	return "sdd/" + changeName
}

func buildArtifactPaths(changeName string) map[string]string {
	paths := make(map[string]string, len(PhaseOutput))
	for _, artifact := range PhaseOutput {
		paths[artifact] = "sdd/" + changeName + "/" + artifact
	}
	return paths
}

func buildPhaseInstructions(changeName string) map[string]string {
	instructions := make(map[string]string, len(PhaseOrder))
	for _, phase := range PhaseOrder {
		instructions[phase] = "/" + phase + " " + changeName
	}
	return instructions
}

func buildPhaseRelationships() []PhaseRelationship {
	relationships := make([]PhaseRelationship, 0, len(PhaseOrder))
	for _, phase := range PhaseOrder {
		relationships = append(relationships, PhaseRelationship{
			Phase:          phase,
			OutputArtifact: PhaseOutput[phase],
			Requires:       copyStringSlice(PhaseRequiredDeps[phase]),
		})
	}
	return relationships
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func compactStringSlice(in []string) []string {
	out := make([]string, 0, len(in))
	for _, value := range in {
		if value != "" {
			out = append(out, value)
		}
	}
	return copyStringSlice(out)
}

// normalizeArtifacts ensures every known artifact has an explicit entry.
func normalizeArtifacts(raw map[string]ArtifactState) map[string]ArtifactState {
	out := make(map[string]ArtifactState, len(PhaseOutput))
	for _, artifact := range PhaseOutput {
		if state, ok := raw[artifact]; ok {
			out[artifact] = state
		} else {
			out[artifact] = ArtifactMissing
		}
	}
	return out
}

func parseTaskProgress(content string) *TaskProgress {
	if content == "" {
		return nil
	}
	checked := len(rxCheckedBox.FindAllString(content, -1))
	unchecked := len(rxUncheckedBox.FindAllString(content, -1))
	total := checked + unchecked
	if total == 0 {
		return nil
	}
	return &TaskProgress{
		Total:     total,
		Completed: checked,
		AllDone:   unchecked == 0,
	}
}

// parseApplyDecision inspects the tasks artifact content to determine whether an
// unresolved delivery decision is blocking apply. It mirrors the parseTaskProgress
// pattern: pure content inspection, no I/O.
//
// Required is set when the content contains the literal phrase
// "Decision needed before apply: Yes".
//
// Resolved is set when any of the following resolution tokens are present:
//   - "Decision needed before apply: No"
//   - "Chain strategy: stacked-to-main"
//   - "Chain strategy: feature-branch-chain"
//   - "size:exception"
//
// Returns nil when tasksContent is empty (gate inactive).
func parseApplyDecision(tasksContent string) *ApplyDecision {
	if tasksContent == "" {
		return nil
	}
	required := rxApplyDecisionRequired.MatchString(tasksContent)
	if !required {
		// Gate inactive — not required, therefore not blocking.
		return &ApplyDecision{Required: false, Resolved: true}
	}
	resolved := rxApplyDecisionNo.MatchString(tasksContent) ||
		strings.Contains(tasksContent, "Chain strategy: stacked-to-main") ||
		strings.Contains(tasksContent, "Chain strategy: feature-branch-chain") ||
		strings.Contains(tasksContent, "size:exception")
	return &ApplyDecision{Required: true, Resolved: resolved}
}

func buildApplyState(artifacts map[string]ArtifactState) *ApplyState {
	state := artifacts[ArtifactApplyProgress]
	if state == ArtifactMissing || state == "" {
		return nil
	}
	return &ApplyState{
		HasProgress: true,
		Complete:    artifacts[ArtifactVerifyReport] != ArtifactMissing && artifacts[ArtifactVerifyReport] != "",
	}
}

func computeDependencies(artifacts map[string]ArtifactState, tp *TaskProgress, ad *ApplyDecision, verifyContent string) map[string]DependencyState {
	deps := make(map[string]DependencyState, len(PhaseOrder))
	for _, phase := range PhaseOrder {
		deps[phase] = computePhaseDep(phase, artifacts, tp, ad, verifyContent)
	}
	return deps
}

func computePhaseDep(phase string, artifacts map[string]ArtifactState, tp *TaskProgress, ad *ApplyDecision, verifyContent string) DependencyState {
	output := PhaseOutput[phase]
	// When apply-progress is already done the delivery-decision gate is moot —
	// the phase completed in a prior session and the gate was resolved then.
	if artifacts[output] == ArtifactDone {
		return DepAllDone
	}

	// Explore is an optional discovery phase. Once the team has produced a proposal,
	// explore is considered implicitly complete — routing should not suggest going back.
	if phase == PhaseExplore && artifacts[ArtifactProposal] == ArtifactDone {
		return DepAllDone
	}

	for _, dep := range PhaseRequiredDeps[phase] {
		if artifacts[dep] != ArtifactDone {
			return DepBlocked
		}
	}

	switch phase {
	case PhaseApply:
		// Apply is blocked when a delivery decision is required but unresolved.
		if ad != nil && ad.Required && !ad.Resolved {
			return DepBlocked
		}
		if artifacts[ArtifactApplyProgress] == ArtifactPartial {
			return DepReady
		}
		// Apply is complete (not just done artifact) when tasks are all checked off.
		if tp != nil && tp.AllDone {
			return DepAllDone
		}
	case PhaseVerify:
		// Verify also requires apply-progress to exist (or tasks all done).
		applyProgress := artifacts[ArtifactApplyProgress]
		if applyProgress == ArtifactPartial {
			return DepBlocked
		}
		hasProgress := applyProgress == ArtifactDone
		tasksAllDone := tp != nil && tp.AllDone
		if !hasProgress && !tasksAllDone {
			return DepBlocked
		}
	case PhaseArchive:
		// Archive requires verify-report to be clearly passing.
		if !isVerifyPassing(verifyContent) {
			return DepBlocked
		}
	}

	return DepReady
}

// isVerifyPassing returns true when the verify report content does not contain
// any keyword that indicates a failing or unclear state.
// Negated zero-count forms ("0 failures", "no blockers") are stripped first so that
// common CI summary phrases do not cause false-positive archive blocks.
func isVerifyPassing(content string) bool {
	if content == "" {
		return false
	}
	cleaned := verifyNegatedForms.ReplaceAllString(content, "")
	for _, rx := range verifyBlockPatterns {
		if rx.MatchString(cleaned) {
			return false
		}
	}
	return true
}

func computeNextAndBlockers(artifacts map[string]ArtifactState, deps map[string]DependencyState, ad *ApplyDecision, verifyContent string) (next string, reasons []string) {
	var blocked []string
	for _, phase := range PhaseOrder {
		state := deps[phase]
		switch state {
		case DepAllDone:
			continue
		case DepReady:
			if next == "" {
				next = phase
			}
		case DepBlocked:
			missingDeps := missingDepsFor(phase, artifacts)
			for _, d := range missingDeps {
				blocked = append(blocked, "phase "+phase+" requires artifact "+d)
			}
			// Soft blockers: phase is DepBlocked but all hard-dep artifacts are present.
			// The block comes from a secondary condition in computePhaseDep's switch block.
			if len(missingDeps) == 0 {
				switch phase {
				case PhaseApply:
					blocked = append(blocked, "phase sdd-apply blocked — delivery decision required (tasks declare 'Decision needed before apply: Yes' and no resolved chain strategy/size:exception)")
				case PhaseVerify:
					if artifacts[ArtifactApplyProgress] == ArtifactPartial {
						blocked = append(blocked, "phase sdd-verify blocked — apply-progress is partial; complete or reconcile sdd-apply before verification")
					} else {
						blocked = append(blocked, "phase sdd-verify blocked — apply-progress required or all tasks must be done")
					}
				case PhaseArchive:
					// verifyContent is always "" here only when the verify-report artifact
					// exists (it is a hard dep and passed the dep check above) but was stored
					// with no body. In that case the message must distinguish "empty" from
					// "failing" so the user knows to re-run sdd-verify, not fix failures.
					if verifyContent == "" {
						blocked = append(blocked, "phase sdd-archive blocked — verify report is empty (re-run sdd-verify to generate content)")
					} else {
						blocked = append(blocked, "phase sdd-archive blocked — verify report must pass before archiving")
					}
				default:
					blocked = append(blocked, "phase "+phase+" blocked — secondary condition not met")
				}
			}
		}
	}

	if next == "" && len(blocked) == 0 {
		next = "none"
	}

	return next, blocked
}

func missingDepsFor(phase string, artifacts map[string]ArtifactState) []string {
	var missing []string
	for _, dep := range PhaseRequiredDeps[phase] {
		if artifacts[dep] != ArtifactDone {
			missing = append(missing, dep)
		}
	}
	return missing
}
