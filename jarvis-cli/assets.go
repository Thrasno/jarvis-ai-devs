// Package jarvis exposes embedded filesystem assets used by sub-packages.
// All go:embed declarations must be in a file at the same level as the embed/ directory.
package jarvis

import "embed"

// PersonaFS contains the embedded persona YAML preset files.
//
//go:embed all:embed/personas
var PersonaFS embed.FS

// SkillsFS contains the embedded skill Markdown files.
//
//go:embed all:embed/skills
var SkillsFS embed.FS

// TemplatesFS contains the embedded template files.
//
//go:embed all:embed/templates
var TemplatesFS embed.FS
