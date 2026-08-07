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
	group := ProjectMigrationGroup{Key: key, Records: len(records), Display: display, DisplaySource: source}
	seen := make(map[string]string)
	for _, record := range records {
		identity := string(record.Table) + "\x00" + record.Identity
		if value, exists := seen[identity]; exists {
			if value == record.Value {
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
