// Package project provides stack detection and project scaffolding for jarvis init.
package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// DetectProject returns the project name. Resolution order:
//  1. git remote get-url origin → last path/colon-segment, .git stripped
//  2. basename of dir
//  3. "default"
func DetectProject(dir string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	if out, err := cmd.Output(); err == nil {
		if name := extractRepoName(strings.TrimSpace(string(out))); name != "" {
			return name
		}
	}
	if base := filepath.Base(dir); base != "" && base != "." && base != "/" {
		return base
	}
	return "default"
}

// SkillsForStack returns the skill list for a given stack.
// Core skills (sdd-workflow, hive) are always first.
func SkillsForStack(stack Stack) []string {
	skills := []string{"sdd-workflow", "hive"}
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

// extractRepoName parses a git remote URL and returns the repository name.
// Handles both HTTPS (https://github.com/org/repo.git) and SSH (git@github.com:org/repo.git).
func extractRepoName(remoteURL string) string {
	remoteURL = strings.TrimSuffix(remoteURL, ".git")
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return ""
	}
	// Find last segment separator — whichever of '/' or ':' comes last.
	lastSlash := strings.LastIndex(remoteURL, "/")
	lastColon := strings.LastIndex(remoteURL, ":")
	sep := lastSlash
	if lastColon > sep {
		sep = lastColon
	}
	if sep < 0 || sep == len(remoteURL)-1 {
		return remoteURL
	}
	return remoteURL[sep+1:]
}
