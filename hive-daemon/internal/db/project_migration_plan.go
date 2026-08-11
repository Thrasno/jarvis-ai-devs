package db

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

// ProjectState identifies daemon-local state that carries project identity.
type ProjectState string

const (
	ProjectStateMemories            ProjectState = "memories"
	ProjectStateSessions            ProjectState = "sessions"
	ProjectStateSyncState           ProjectState = "sync_state"
	ProjectStateMemoryMutations     ProjectState = "memory_mutations"
	ProjectStateMutationReceipts    ProjectState = "mutation_receipts"
	ProjectStateMutationCursors     ProjectState = "mutation_cursors"
	ProjectStatePullCursors         ProjectState = "pull_cursors"
	ProjectStatePrompts             ProjectState = "user_prompts"
	ProjectStateAliases             ProjectState = "project_aliases"
	ProjectStateBlocks              ProjectState = "project_blocks"
	ProjectStateQuarantineArchives  ProjectState = "project_quarantine_archives"
	ProjectStateGovernance          ProjectState = "hive_project_governance"
	ProjectStateImportAliases       ProjectState = "import_source_aliases"
	ProjectStatePassiveObservations ProjectState = "passive_observations"
	ProjectStateSyncAttempts        ProjectState = "sync_attempt_logs"
	ProjectStateRecoveryTokens      ProjectState = "recovery_tokens"
	ProjectStateMemoryPromptLinks   ProjectState = "memory_prompt_links"
)

// ProjectKeyedStates returns the complete daemon-local migration inventory.
func ProjectKeyedStates() []ProjectState {
	return []ProjectState{ProjectStateMemories, ProjectStateSessions, ProjectStateSyncState,
		ProjectStateMemoryMutations, ProjectStateMutationReceipts, ProjectStateMutationCursors,
		ProjectStatePullCursors, ProjectStatePrompts, ProjectStateAliases, ProjectStateBlocks,
		ProjectStateQuarantineArchives, ProjectStateGovernance, ProjectStateImportAliases,
		ProjectStatePassiveObservations, ProjectStateSyncAttempts, ProjectStateRecoveryTokens,
		ProjectStateMemoryPromptLinks}
}

// ProjectStateRecord is a read-only preflight observation. Value is a stable
// content fingerprint supplied by the inventory reader; this planner never opens
// a database or mutates a record.
type ProjectStateRecord struct {
	Table         ProjectState
	Project       string
	Identity      string
	Value         string
	RegisteredAt  time.Time
	StableID      string
	RemoteDisplay string
}

type MigrationConflictKind string

const (
	ConflictDivergentGlobalEntity       MigrationConflictKind = "divergent-global-entity"
	ConflictDivergentSyncState          MigrationConflictKind = "divergent-sync-state"
	ConflictIncompatibleSessionSentinel MigrationConflictKind = "incompatible-session-sentinel"
	ConflictContradictoryAlias          MigrationConflictKind = "contradictory-alias"
	ConflictNonMonotonicCursorProtocol  MigrationConflictKind = "non-monotonic-cursor-protocol"
	ConflictContradictoryGovernanceHead MigrationConflictKind = "contradictory-governance-head"
	ConflictBrokenReference             MigrationConflictKind = "broken-reference"
)

type MigrationConflict struct {
	Kind     MigrationConflictKind
	Table    ProjectState
	Key      string
	Identity string
}

type ProjectMigrationGroup struct {
	Key           string
	Display       string
	DisplaySource DisplaySource
	Records       int
	Coalesced     int
	// Variants are the distinct raw spellings this group folds, sorted. Records is
	// a row count and means nothing to a human; these are the names the operator
	// chose, and recognizing them is the only way they can approve the fold.
	Variants []ProjectMigrationVariant
}

// ProjectMigrationVariant is one stored spelling of a project.
//
// Canonical is carried explicitly rather than left for a caller to derive by
// comparing Spelling against the group Key: whether a spelling is already
// canonical is what decides if the fold rewrites it, and a client re-deriving that
// would be reimplementing the rule the daemon just applied. So a wizard can say
// "these three become that one" by reading the flag.
type ProjectMigrationVariant struct {
	Spelling  string
	Canonical bool
}

// DisplaySource makes display-name precedence auditable before execution.
type DisplaySource string

const (
	DisplaySourceRemote             DisplaySource = "remote"
	DisplaySourceOldestRegistration DisplaySource = "oldest-registration"
)

type ProjectMigrationAction struct {
	Key string
}

// ProjectMigrationPlan is a pure preflight result. Actions are intentionally
// empty if any conflict exists, so a later executor cannot select a safe subset.
type ProjectMigrationPlan struct {
	Executable  bool
	Groups      []ProjectMigrationGroup
	Conflicts   []MigrationConflict
	Actions     []ProjectMigrationAction
	Fingerprint string
}

// BuildProjectMigrationPlan classifies case-equivalent records without mutation.
func BuildProjectMigrationPlan(records []ProjectStateRecord) ProjectMigrationPlan {
	sorted := append([]ProjectStateRecord(nil), records...)
	sort.Slice(sorted, func(i, j int) bool {
		left, right := projectidentity.Canonical(sorted[i].Project).String(), projectidentity.Canonical(sorted[j].Project).String()
		if left != right {
			return left < right
		}
		if sorted[i].Table != sorted[j].Table {
			return sorted[i].Table < sorted[j].Table
		}
		if sorted[i].Identity != sorted[j].Identity {
			return sorted[i].Identity < sorted[j].Identity
		}
		return sorted[i].StableID < sorted[j].StableID
	})
	plan := ProjectMigrationPlan{Executable: true}
	for start := 0; start < len(sorted); {
		key := projectidentity.Canonical(sorted[start].Project).String()
		end := start + 1
		for end < len(sorted) && projectidentity.Canonical(sorted[end].Project).String() == key {
			end++
		}
		group := sorted[start:end]
		plan.Groups = append(plan.Groups, makeMigrationGroup(key, group, &plan.Conflicts))
		start = end
	}
	if len(plan.Conflicts) > 0 {
		plan.Executable = false
	} else {
		for _, group := range plan.Groups {
			if group.Records > 1 {
				plan.Actions = append(plan.Actions, ProjectMigrationAction{Key: group.Key})
			}
		}
	}
	plan.Fingerprint = planFingerprint(plan)
	return plan
}

func makeMigrationGroup(key string, records []ProjectStateRecord, conflicts *[]MigrationConflict) ProjectMigrationGroup {
	display, source := displayFor(records)
	group := ProjectMigrationGroup{Key: key, Records: len(records), Display: display, DisplaySource: source,
		Variants: migrationVariants(key, records)}
	seen := make(map[string]string)
	for _, record := range records {
		identity := string(record.Table) + "\x00" + record.Identity
		if value, exists := seen[identity]; exists {
			if value == record.Value || mergeableOnCollision(record.Table) {
				group.Coalesced++
				continue
			}
			*conflicts = append(*conflicts, MigrationConflict{Kind: conflictKind(record.Table), Table: record.Table, Key: key, Identity: record.Identity})
			continue
		}
		seen[identity] = record.Value
	}
	return group
}

// mergeableOnCollision reports state whose colliding rows the executor can merge
// on its own, so a value divergence is coalescible work rather than a conflict
// that blocks the whole plan.
//
// sync_state is the only such state: it is a singleton per project, so two
// spellings always collide on its constant identity, and coalesceProjectSyncState
// folds them through one deterministic per-column policy. Every other table here
// carries a real composite identity, so a divergence there is a genuine
// contradiction no rule can settle.
func mergeableOnCollision(table ProjectState) bool {
	return table == ProjectStateSyncState
}

// migrationVariants reduces a group's rows to its distinct spellings.
//
// Distinct because a project with 148 rows across two spellings is two variants,
// not 148 — the operator is deciding about names, not rows. Sorted because this
// feeds planFingerprint, and a variant order that followed the inventory reader's
// row order would give the same database a different fingerprint on every read,
// so every approval would come back stale.
//
// A group may legitimately have no canonical variant: if every stored spelling
// needs rewriting, the canonical key is a name that exists nowhere yet.
func migrationVariants(key string, records []ProjectStateRecord) []ProjectMigrationVariant {
	seen := make(map[string]bool, len(records))
	spellings := make([]string, 0, len(records))
	for _, record := range records {
		if seen[record.Project] {
			continue
		}
		seen[record.Project] = true
		spellings = append(spellings, record.Project)
	}
	sort.Strings(spellings)
	variants := make([]ProjectMigrationVariant, 0, len(spellings))
	for _, spelling := range spellings {
		variants = append(variants, ProjectMigrationVariant{Spelling: spelling, Canonical: spelling == key})
	}
	return variants
}

func displayFor(records []ProjectStateRecord) (string, DisplaySource) {
	oldest := records[0]
	for _, record := range records {
		if record.RemoteDisplay != "" {
			return record.RemoteDisplay, DisplaySourceRemote
		}
		if record.RegisteredAt.Before(oldest.RegisteredAt) || (record.RegisteredAt.Equal(oldest.RegisteredAt) && record.StableID < oldest.StableID) {
			oldest = record
		}
	}
	return oldest.Project, DisplaySourceOldestRegistration
}

func conflictKind(table ProjectState) MigrationConflictKind {
	switch table {
	// Unreachable from the planner: mergeableOnCollision coalesces a divergent
	// sync_state row instead of reporting it. The kind stays mapped so a
	// persisted or replayed conflict from before the merge existed still
	// classifies, and so a future non-mergeable sync_state collision would not
	// silently fall through to divergent-global-entity.
	case ProjectStateSyncState:
		return ConflictDivergentSyncState
	case ProjectStateSessions:
		return ConflictIncompatibleSessionSentinel
	case ProjectStateAliases:
		return ConflictContradictoryAlias
	case ProjectStateMutationCursors, ProjectStatePullCursors:
		return ConflictNonMonotonicCursorProtocol
	case ProjectStateGovernance, ProjectStateBlocks, ProjectStateQuarantineArchives:
		return ConflictContradictoryGovernanceHead
	case ProjectStateMemoryPromptLinks:
		return ConflictBrokenReference
	default:
		return ConflictDivergentGlobalEntity
	}
}

func planFingerprint(plan ProjectMigrationPlan) string {
	plan.Fingerprint = ""
	encoded, _ := json.Marshal(plan)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
