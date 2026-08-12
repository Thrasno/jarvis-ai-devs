package projectregistry

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/project"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills/diskscan"
)

const (
	ReasonCreated   = project.RegistryReasonCreated
	ReasonUpdated   = project.RegistryReasonUpdated
	ReasonUnchanged = project.RegistryReasonUnchanged
	ReasonForced    = project.RegistryReasonForced

	SeverityWarning = project.RegistrySeverityWarning
)

// RefreshOptions configures a Refresh run.
// ScanDirs overrides the default directory list produced by diskscan.ResolveScanDirs.
// When ScanDirs is nil or empty, ResolveScanDirs(root) is used.
// Refresh never installs skill copies — that is cmd_init's responsibility.
type RefreshOptions struct {
	CWD             string
	Force           bool
	AllowNonGitRoot bool
	ScanDirs        []string
	// NoGitignore skips .gitignore mutation when true.
	NoGitignore bool
}

type Warning struct {
	Code     string
	Severity string
	Path     string
	Message  string
}

type Result struct {
	Root       string
	Path       string
	Reason     string
	Changed    bool
	SkillCount int
	Warnings   []Warning
}

// Refresh updates the project-local skill registry by scanning disk skill directories.
// It does NOT install skill copies — that is the responsibility of cmd_init.
func Refresh(ctx context.Context, opts RefreshOptions) (Result, error) {
	root, err := ResolveRoot(ctx, opts.CWD)
	if err != nil {
		if !opts.AllowNonGitRoot || !errors.Is(err, ErrNotGitWorktree) {
			return Result{}, err
		}
		root, err = resolveExplicitProjectRoot(opts.CWD)
		if err != nil {
			return Result{}, err
		}
	}

	dirs := opts.ScanDirs
	if len(dirs) == 0 {
		dirs = diskscan.ResolveScanDirs(root)
	}

	rows, scanWarns, err := diskscan.Scan(dirs)
	if err != nil {
		return Result{Root: root}, fmt.Errorf("scan skills: %w", err)
	}

	registrySkills := scanRowsToRegistrySkills(root, rows)
	registryWarnings := toRegistryWarnings(scanWarns)

	registryResult, err := project.WriteRegistryWithResult(
		root,
		project.DetectProject(root),
		registrySkills,
		project.WriteRegistryOptions{Force: opts.Force, Warnings: registryWarnings},
	)
	if err != nil {
		return Result{Root: root, Path: registryResult.Path}, fmt.Errorf("write skill registry: %w", err)
	}

	refreshWarnings := toRefreshWarnings(registryResult.Warnings)
	refreshWarnings = append(refreshWarnings, unsafeRootWarnings(root)...)

	// After writing the registry, ensure .gitignore contains the per-machine cache entries.
	// Filesystem errors (read/write .gitignore) are hard errors that abort Refresh.
	// Only git rm failures are demoted to non-fatal warnings surfaced in Result.Warnings.
	if gitWarn, gitErr := EnsureGitignore(ctx, root, EnsureGitignoreOptions{NoGitignore: opts.NoGitignore}); gitErr != nil {
		return Result{Root: root, Path: registryResult.Path}, fmt.Errorf("ensure gitignore: %w", gitErr)
	} else if gitWarn != "" {
		refreshWarnings = append(refreshWarnings, Warning{
			Code:     "gitignore-untrack",
			Severity: SeverityWarning,
			Path:     ".gitignore",
			Message:  gitWarn,
		})
	}

	return Result{
		Root:       root,
		Path:       registryResult.Path,
		Reason:     registryResult.Reason,
		Changed:    registryResult.Changed,
		SkillCount: registryResult.SkillCount,
		Warnings:   refreshWarnings,
	}, nil
}

// scanRowRelPath converts an absolute OS-native path to a project-relative,
// forward-slash path. This is the cross-platform path normalization gate required
// for portable registry markdown on all operating systems (including Windows).
//
// When the path is outside the project root (e.g. a user-global skill under
// ~/.claude/skills/...), filepath.Rel returns a ".." -prefixed relative path
// that is meaningless outside the original machine layout. In that case the
// function falls back to the absolute path with forward slashes so the registry
// entry remains well-defined regardless of the caller's directory.
func scanRowRelPath(root, absPath string) string {
	rel, err := filepath.Rel(root, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		// Path is outside the project root — use the absolute path with
		// forward slashes so the registry entry is always a valid, portable path.
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}

// scanRowsToRegistrySkills maps diskscan.ScanRow values to project.RegistrySkill
// values, normalizing the absolute OS-native path to a project-relative,
// forward-slash path suitable for the registry markdown.
func scanRowsToRegistrySkills(root string, rows []diskscan.ScanRow) []project.RegistrySkill {
	skills := make([]project.RegistrySkill, 0, len(rows))
	for _, row := range rows {
		skills = append(skills, project.RegistrySkill{
			ID:      row.ID,
			Name:    row.Name,
			Trigger: row.Trigger,
			Scope:   row.Scope,
			Path:    scanRowRelPath(root, row.Path),
		})
	}
	return skills
}

func toRegistryWarnings(warns []diskscan.ScanWarning) []project.RegistryWarning {
	result := make([]project.RegistryWarning, 0, len(warns))
	for _, w := range warns {
		result = append(result, project.RegistryWarning{
			Code:     w.Code,
			Severity: SeverityWarning,
			Path:     w.Path,
		})
	}
	return result
}

func toRefreshWarnings(warnings []project.RegistryWarning) []Warning {
	result := make([]Warning, 0, len(warnings))
	for _, warning := range warnings {
		result = append(result, Warning{
			Code:     warning.Code,
			Severity: warning.Severity,
			Path:     warning.Path,
			Message:  warning.Message,
		})
	}
	return result
}
