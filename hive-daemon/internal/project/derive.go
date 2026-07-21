package project

import (
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/hivederive"
)

// DeriveFromDirectory returns the canonical project name for dir, delegating to
// the shared hivederive.Derive source of truth (git remote → basename, with
// WSL/Windows path normalization). On any derivation error it returns the
// internal "default" sentinel so the existing `!= "default"` provenance guards
// keep working unchanged; hivederive itself never returns the literal
// "default".
//
// parity anchor: jarvis-cli/internal/project/detector.go:DetectProject
func DeriveFromDirectory(dir string) string {
	name, err := hivederive.Derive(dir)
	if err != nil {
		return "default"
	}
	return name
}

// ResolveEffectiveProject returns the project name to use for persistence and
// validation, together with a provenance flag indicating whether the name was
// derived from the filesystem (derived=true) or supplied by the caller
// (derived=false).
//
// Rules:
//   - Non-empty project (even whitespace-trimmed empty is treated as empty):
//     return (project, false) — caller-supplied name, no derivation.
//   - Empty project + non-empty directory: derive via DeriveFromDirectory,
//     return (derived_name, true).
//   - Both empty: return ("", false).
//
// The derived bool is an explicit return value — never inferred — so that
// callers can gate provenance-specific behaviour (e.g. the project_unknown
// bypass in memSaveHandler) without ambiguity.
func ResolveEffectiveProject(projectName, directory string) (string, bool) {
	if strings.TrimSpace(projectName) != "" {
		return projectName, false
	}
	if strings.TrimSpace(directory) != "" {
		return DeriveFromDirectory(directory), true
	}
	return "", false
}
