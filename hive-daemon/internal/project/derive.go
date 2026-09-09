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
func DeriveFromDirectory(dir string) string {
	name, ok := DeriveProjectIdentity(dir)
	if !ok {
		return "default"
	}
	return name
}

// DeriveProjectIdentity returns a concrete project identity backed by directory
// evidence. It preserves hivederive's Git-origin-first and basename-fallback
// precedence, while refusing the reserved "default" sentinel as an identity.
func DeriveProjectIdentity(dir string) (string, bool) {
	name, ok, _ := deriveProjectIdentity(dir)
	return name, ok
}

func deriveProjectIdentity(dir string) (string, bool, error) {
	name, err := hivederive.Derive(dir)
	if err != nil {
		return "", false, err
	}
	if strings.TrimSpace(name) == "" || name == "default" {
		return "", false, nil
	}
	return name, true, nil
}

// ResolveEffectiveProject returns the caller project or a directory-derived
// fallback, together with whether the fallback was derived. Write paths use
// ValidateWriteProject for shared identity validation.
//
// Rules:
//   - Non-empty project (even whitespace-trimmed empty is treated as empty):
//     return (project, false) — caller-supplied name, no derivation.
//   - Empty project + non-empty directory: derive via DeriveFromDirectory,
//     return (derived_name, true).
//   - Both empty: return ("", false).
//
// The derived bool is an explicit return value — never inferred.
func ResolveEffectiveProject(projectName, directory string) (string, bool) {
	if strings.TrimSpace(projectName) != "" {
		return projectName, false
	}
	if strings.TrimSpace(directory) != "" {
		return DeriveFromDirectory(directory), true
	}
	return "", false
}
