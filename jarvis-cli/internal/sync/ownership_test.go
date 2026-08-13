package sync

import (
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
)

// catalogEntry builds a catalog skill whose frontmatter scope is deliberately
// set, so tests can prove the scope never influences any decision.
func catalogEntry(id, scope string) skills.Skill {
	return skills.Skill{ID: id, Scope: scope, IsCore: scope == "core"}
}

func TestOwnership_IdentityIsTheOnlyProof(t *testing.T) {
	catalog := []skills.Skill{
		catalogEntry("sdd-apply", "core"),
		catalogEntry("sdd-spec", "optional"),
	}
	manifest := []string{"sdd-apply", "retired-skill"}
	own := NewOwnership(catalog, manifest)

	tests := []struct {
		name       string
		id         string
		wantOwned  bool
		wantSource IdentitySource
	}{
		{
			name:       "catalog membership alone proves ownership",
			id:         "sdd-spec",
			wantOwned:  true,
			wantSource: IdentitySourceCatalog,
		},
		{
			name:       "manifest membership alone proves ownership",
			id:         "retired-skill",
			wantOwned:  true,
			wantSource: IdentitySourceManifest,
		},
		{
			name:       "membership in both reports the catalog as the source",
			id:         "sdd-apply",
			wantOwned:  true,
			wantSource: IdentitySourceCatalog,
		},
		{
			name:      "an sdd- prefixed skill in neither list is user-authored",
			id:        "sdd-legacy",
			wantOwned: false,
		},
		{
			name:      "an unrelated skill in neither list is user-authored",
			id:        "my-team-conventions",
			wantOwned: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proof, owned := own.Classify(tt.id)
			if owned != tt.wantOwned {
				t.Fatalf("Classify(%q) owned = %v, want %v", tt.id, owned, tt.wantOwned)
			}
			if !owned {
				return
			}
			if proof.Source != tt.wantSource {
				t.Fatalf("Classify(%q) source = %q, want %q", tt.id, proof.Source, tt.wantSource)
			}
		})
	}
}

// A skill declaring scope: optional is still auto-installed, and a skill
// declaring scope: core is still untouchable when no list names it. Ownership
// and lifecycle read identity only.
func TestOwnership_FrontmatterScopeNeverDecides(t *testing.T) {
	own := NewOwnership([]skills.Skill{catalogEntry("sdd-spec", "optional")}, nil)

	if got := own.ResolveSkill("sdd-spec"); got != SkillActionInstall {
		t.Fatalf("optional catalog skill resolved to %q, want %q", got, SkillActionInstall)
	}

	// A user-authored skill declaring scope: core is still untouchable: no
	// list names it, so its frontmatter buys it nothing.
	if got := own.ResolveSkill("user-core-skill"); got != SkillActionUntouched {
		t.Fatalf("unlisted skill resolved to %q, want %q", got, SkillActionUntouched)
	}
}

func TestOwnership_ResolveSkillLifecycle(t *testing.T) {
	// "sdd-spec" is non-interactive; "go-testing" is a member of the
	// interactive selection set.
	const (
		nonInteractiveID = "sdd-spec"
		interactiveID    = "go-testing"
	)

	tests := []struct {
		name     string
		catalog  []string
		manifest []string
		id       string
		want     SkillAction
	}{
		{
			name:     "in manifest and catalog is updated",
			catalog:  []string{nonInteractiveID},
			manifest: []string{nonInteractiveID},
			id:       nonInteractiveID,
			want:     SkillActionUpdate,
		},
		{
			name:     "interactive skill in manifest and catalog is still updated",
			catalog:  []string{interactiveID},
			manifest: []string{interactiveID},
			id:       interactiveID,
			want:     SkillActionUpdate,
		},
		{
			name:     "in manifest but dropped from catalog is deleted",
			catalog:  []string{nonInteractiveID},
			manifest: []string{"retired-skill"},
			id:       "retired-skill",
			want:     SkillActionDelete,
		},
		{
			name:     "interactive skill dropped from catalog is deleted",
			catalog:  nil,
			manifest: []string{interactiveID},
			id:       interactiveID,
			want:     SkillActionDelete,
		},
		{
			name:     "new non-interactive catalog skill is installed",
			catalog:  []string{nonInteractiveID},
			manifest: nil,
			id:       nonInteractiveID,
			want:     SkillActionInstall,
		},
		{
			name:     "new interactive catalog skill is not installed",
			catalog:  []string{interactiveID},
			manifest: nil,
			id:       interactiveID,
			want:     SkillActionSkip,
		},
		{
			name:     "skill in neither list is never touched",
			catalog:  []string{nonInteractiveID},
			manifest: []string{"retired-skill"},
			id:       "hand-written-skill",
			want:     SkillActionUntouched,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := make([]skills.Skill, 0, len(tt.catalog))
			for _, id := range tt.catalog {
				catalog = append(catalog, catalogEntry(id, "optional"))
			}
			own := NewOwnership(catalog, tt.manifest)

			if got := own.ResolveSkill(tt.id); got != tt.want {
				t.Fatalf("ResolveSkill(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

// _shared/ is installed unconditionally and is not a skill, so it must never
// reach the lifecycle resolver as an owned identity.
func TestOwnership_SharedDirectoryIsNotASkill(t *testing.T) {
	own := NewOwnership([]skills.Skill{catalogEntry("sdd-spec", "optional")}, []string{"sdd-spec"})

	if _, owned := own.Classify("_shared"); owned {
		t.Fatal("Classify(\"_shared\") reported Jarvis ownership; _shared is not a skill")
	}
	if got := own.ResolveSkill("_shared"); got != SkillActionUntouched {
		t.Fatalf("ResolveSkill(\"_shared\") = %q, want %q", got, SkillActionUntouched)
	}
}
