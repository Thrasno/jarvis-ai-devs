// Package project provides stack detection and project scaffolding for jarvis init.
package project

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/hivederive"
)

// Stack represents the detected technology stack of a project.
type Stack string

const (
	StackGo      Stack = "Go"
	StackLaravel Stack = "Laravel"
	StackPHP     Stack = "PHP"
	StackAngular Stack = "Angular"
	StackReact   Stack = "React"
	StackNode    Stack = "Node"
	StackRust    Stack = "Rust"
	StackPython  Stack = "Python"
	StackZoho    Stack = "Zoho"
	StackUnknown Stack = "Unknown"
)

// DetectStack returns the primary technology stack by probing dir for known files.
// Probes are evaluated in the order listed below; first match wins.
func DetectStack(dir string) Stack {
	probe := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	readLower := func(name string) string {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return ""
		}
		return strings.ToLower(string(data))
	}

	if probe("go.mod") {
		return StackGo
	}
	if probe("composer.json") {
		content := readLower("composer.json")
		if strings.Contains(content, "zoho") || strings.Contains(content, "deluge") {
			return StackZoho
		}
		if strings.Contains(content, "laravel/framework") {
			return StackLaravel
		}
		return StackPHP
	}
	if probe("package.json") {
		content := readLower("package.json")
		if strings.Contains(content, "zoho") || strings.Contains(content, "deluge") {
			return StackZoho
		}
		if strings.Contains(content, `"@angular/core"`) {
			return StackAngular
		}
		if strings.Contains(content, `"react"`) {
			return StackReact
		}
		return StackNode
	}
	if probe("Cargo.toml") {
		return StackRust
	}
	if probe("pyproject.toml") || probe("requirements.txt") {
		return StackPython
	}
	return StackUnknown
}

// DetectProject returns the canonical project name for dir, delegating to the
// shared hivederive.Derive source of truth (git remote → basename). On any
// derivation error — empty directory, unresolvable path, or no derivable name —
// it returns "" so hook callers treat it as "no pin / skip register" rather
// than leaking an ambient-cwd repo name or the silent "default" sentinel.
func DetectProject(dir string) string {
	name, err := hivederive.Derive(dir)
	if err != nil {
		return ""
	}
	return name
}

// SkillsForStack returns the skill list for a given stack.
// Core skills are always first.
func SkillsForStack(stack Stack) []string {
	skills := []string{"hive"}
	switch stack {
	case StackGo:
		skills = append(skills, "go-testing")
	case StackLaravel:
		skills = append(skills, "laravel-architecture", "phpunit-testing")
	case StackZoho:
		skills = append(skills, "zoho-deluge")
	}
	return skills
}
