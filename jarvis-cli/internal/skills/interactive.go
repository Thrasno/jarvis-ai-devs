package skills

// interactiveSkillIDs lists the stack-specific skills a human explicitly opts
// into during the wizard. Every other skill is auto-installed.
//
// This is the single source of truth for that set. It lives here, in the
// catalog package, so both the wizard (internal/tui) and the machine-scoped
// replay planner (internal/sync) read the same list instead of keeping two
// copies that can silently drift apart.
//
// Membership is decided by skill ID only. A skill's frontmatter scope never
// takes part: sdd-spec declares scope: optional and is still auto-installed.
var interactiveSkillIDs = map[string]bool{
	"zoho-deluge":          true,
	"phpunit-testing":      true,
	"laravel-architecture": true,
	"go-testing":           true,
}

// IsInteractive reports whether the skill with this ID is only installed after
// an explicit human selection.
func IsInteractive(id string) bool {
	return interactiveSkillIDs[id]
}
