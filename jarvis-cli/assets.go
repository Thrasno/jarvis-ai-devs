// Package jarvis exposes embedded filesystem assets used by sub-packages.
// All go:embed declarations must be in a file at the same level as the embed/ directory.
package jarvis

import "embed"

// PersonaFS contains the active V1 and dormant V2 persona YAML catalogs.
//
//go:embed all:embed/personas all:embed/personas-v2
var PersonaFS embed.FS

// SkillsFS contains the embedded skill Markdown files.
//
//go:embed all:embed/skills
var SkillsFS embed.FS

// TemplatesFS contains the embedded template files.
//
//go:embed all:embed/templates
var TemplatesFS embed.FS

// OrchestratorFS contains the embedded orchestrator file.
//
//go:embed all:embed/orchestrator
var OrchestratorFS embed.FS

// HooksFS contains the embedded prompt-capture hook files.
//
//go:embed all:embed/hooks
var HooksFS embed.FS

// AgentsFS contains the embedded named agent definition files.
//
//go:embed all:embed/agents
var AgentsFS embed.FS

// HiveProtocol contains the canonical Hive memory protocol injected into agent instructions.
//
//go:embed embed/hive-protocol.md
var HiveProtocol string

// TechnicalContract contains the canonical technical and educational policy
// composed into Layer1 for every generated agent instruction.
//
//go:embed embed/technical-contract.md
var TechnicalContract string
