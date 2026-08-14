// Package sync plans and replays the desired state recorded by the last
// installation. This file holds the ownership half of the planner: deciding
// which on-disk artifacts Jarvis is allowed to touch, and what each skill's
// lifecycle action is.
package sync

import (
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// IdentitySource names the membership list that proved ownership.
type IdentitySource string

const (
	// IdentitySourceCatalog means the current embedded catalog offers this ID.
	IdentitySourceCatalog IdentitySource = "catalog"
	// IdentitySourceManifest means only the manifest's skills list names this
	// ID: the installation created it and a later catalog dropped it.
	IdentitySourceManifest IdentitySource = "manifest"
)

// IdentityProof records that an artifact is Jarvis-owned because its identity
// appears in the embedded catalog or in the manifest's skills list. It is the
// proof used where no provenance marker exists, such as skill directories.
//
// It is one of the two members of the planner's closed Proof sum; the other,
// MarkerProof, covers artifacts a provenance marker binds.
type IdentityProof struct {
	Source IdentitySource
}

// InstructionOwnership answers a single question: may replay write this
// instruction-file path for this agent?
//
// It is the same kind of evidence Ownership uses for skills — membership in a
// recorded list, never a path convention — applied to the one file each agent
// keeps its managed instructions in. The manifest is the whole list: a path it
// does not record belongs to nobody, and replay never touches it.
//
// It deliberately says nothing about the file's contents. Inside an owned path,
// the installer's own rules apply: a sentinel-bearing file is patched in place
// and everything outside the managed sections survives, while a file that lost
// its sentinels is rendered fresh and its previous content is discarded.
type InstructionOwnership struct {
	// agentByPath maps a canonical instruction path to the agent that owns it,
	// so a path recorded for one agent cannot be written on another's behalf.
	agentByPath map[string]string
}

// NewInstructionOwnership captures the instruction paths the manifest records.
// Agents with no recorded path contribute nothing: an agent whose instruction
// file was never recorded has no owned path to replay.
func NewInstructionOwnership(agents []state.Agent) InstructionOwnership {
	own := InstructionOwnership{agentByPath: make(map[string]string, len(agents))}
	for _, configured := range agents {
		id := strings.TrimSpace(configured.ID)
		path := canonicalInstructionPath(configured.InstructionsPath)
		if id == "" || path == "" {
			continue
		}
		own.agentByPath[path] = id
	}
	return own
}

// OwnsInstructions reports whether replay may write path on agentID's behalf.
// Both must match a single manifest record: an unrecorded path is refused, and
// so is a recorded path claimed for a different agent.
func (o InstructionOwnership) OwnsInstructions(agentID, path string) bool {
	canonical := canonicalInstructionPath(path)
	if canonical == "" {
		return false
	}
	owner, recorded := o.agentByPath[canonical]
	return recorded && owner == strings.TrimSpace(agentID)
}

// canonicalInstructionPath normalizes a path for comparison. It never resolves
// symlinks and never touches the filesystem: deciding ownership must not read
// anything, least of all a path Jarvis may turn out not to own.
func canonicalInstructionPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.Clean(trimmed)
}

// SkillAction is the lifecycle action the planner resolves for one skill ID.
type SkillAction string

const (
	// SkillActionUpdate rewrites a skill both lists name.
	SkillActionUpdate SkillAction = "update"
	// SkillActionDelete removes a skill the manifest owns and the catalog
	// dropped. The unfiltered manifest list is the only proof authorizing it.
	SkillActionDelete SkillAction = "delete"
	// SkillActionInstall adds a catalog skill this machine does not own yet.
	SkillActionInstall SkillAction = "install"
	// SkillActionSkip leaves a stack-specific catalog skill uninstalled: it is
	// only ever installed after an explicit human selection.
	SkillActionSkip SkillAction = "skip"
	// SkillActionUntouched leaves a user-authored skill alone. A skill neither
	// list names is never read, never counted as drift, and never written.
	SkillActionUntouched SkillAction = "untouched"
)

// sharedDirName is installed alongside the skills but is not a skill, so it
// never participates in ownership or lifecycle decisions.
const sharedDirName = "_shared"

// Ownership answers ownership and lifecycle questions from exactly two
// membership sets: the currently embedded catalog and the manifest's skills
// list.
//
// Nothing else is evidence. Ownership is never decided by a file path, by a
// naming convention such as an sdd- prefix, or by a frontmatter scope value.
type Ownership struct {
	catalog  map[string]bool
	manifest map[string]bool
}

// NewOwnership captures the two membership sets. catalog is the currently
// embedded skill catalog; manifestSkills is the manifest's unfiltered skills
// list, which still names skills a newer catalog dropped.
func NewOwnership(catalog []skills.Skill, manifestSkills []string) Ownership {
	own := Ownership{
		catalog:  make(map[string]bool, len(catalog)),
		manifest: make(map[string]bool, len(manifestSkills)),
	}
	for _, s := range catalog {
		if s.ID == sharedDirName {
			continue
		}
		own.catalog[s.ID] = true
	}
	for _, id := range manifestSkills {
		if id == sharedDirName {
			continue
		}
		own.manifest[id] = true
	}
	return own
}

// Classify reports whether Jarvis owns this identity and which list proved it.
// A false second return means the artifact is user-authored and untouchable.
// When both lists name the identity, the catalog is reported as the source
// because it is the current, authoritative offer.
func (o Ownership) Classify(id string) (IdentityProof, bool) {
	switch {
	case o.catalog[id]:
		return IdentityProof{Source: IdentitySourceCatalog}, true
	case o.manifest[id]:
		return IdentityProof{Source: IdentitySourceManifest}, true
	default:
		return IdentityProof{}, false
	}
}

// ResolveSkill returns the lifecycle action for one skill ID.
//
//	in manifest + in catalog                  -> update
//	in manifest + dropped from catalog        -> delete
//	new catalog skill, auto-installed         -> install
//	new catalog skill, stack-specific         -> skip
//	in neither list                           -> untouched
func (o Ownership) ResolveSkill(id string) SkillAction {
	inManifest := o.manifest[id]
	inCatalog := o.catalog[id]

	switch {
	case inManifest && inCatalog:
		return SkillActionUpdate
	case inManifest:
		return SkillActionDelete
	case inCatalog && skills.IsInteractive(id):
		return SkillActionSkip
	case inCatalog:
		return SkillActionInstall
	default:
		return SkillActionUntouched
	}
}
