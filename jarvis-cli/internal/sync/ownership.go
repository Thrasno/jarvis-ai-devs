// Package sync plans and replays the desired state recorded by the last
// installation. This file holds the ownership half of the planner: deciding
// which on-disk artifacts Jarvis is allowed to touch, and what each skill's
// lifecycle action is.
package sync

import (
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
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
// It becomes one member of the planner's closed Proof sum once MarkerProof
// lands with the target-rendering slice.
type IdentityProof struct {
	Source IdentitySource
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
