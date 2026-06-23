package projectregistry

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/project"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/skills"
)

const (
	ReasonCreated   = project.RegistryReasonCreated
	ReasonUpdated   = project.RegistryReasonUpdated
	ReasonUnchanged = project.RegistryReasonUnchanged
	ReasonForced    = project.RegistryReasonForced

	WarningLegacyRegistryImported = project.RegistryWarningLegacyImported
	SeverityWarning               = project.RegistrySeverityWarning
)

type RefreshOptions struct {
	CWD             string
	Force           bool
	AllowNonGitRoot bool
	SkillsFS        embed.FS
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

// Refresh updates the project-local skill registry and skill copies for the
// explicit cwd's active git worktree.
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

	embeddedSkills, err := skills.ListSkills(opts.SkillsFS)
	if err != nil {
		return Result{Root: root}, fmt.Errorf("list embedded skills: %w", err)
	}
	if len(embeddedSkills) == 0 {
		return Result{Root: root}, fmt.Errorf("list embedded skills: no skills found")
	}

	selected := make([]string, 0, len(embeddedSkills))
	for _, skill := range embeddedSkills {
		selected = append(selected, skill.ID)
	}
	installResult, err := skills.InstallSelectedWithResult(opts.SkillsFS, filepath.Join(root, ".jarvis", "skills"), selected)
	if err != nil {
		return Result{Root: root}, fmt.Errorf("install project skill copies: %w", err)
	}

	registryResult, err := project.WriteRegistryWithResult(
		root,
		project.DetectProject(root),
		toProjectRegistrySkills(skills.RegistryRows(embeddedSkills)),
		project.WriteRegistryOptions{Force: opts.Force},
	)
	if err != nil {
		return Result{Root: root, Path: registryResult.Path}, fmt.Errorf("write skill registry: %w", err)
	}

	changed := registryResult.Changed || installResult.Changed
	reason := registryResult.Reason
	if !registryResult.Changed && installResult.Changed {
		reason = ReasonUpdated
	}

	return Result{
		Root:       root,
		Path:       registryResult.Path,
		Reason:     reason,
		Changed:    changed,
		SkillCount: registryResult.SkillCount,
		Warnings:   toRefreshWarnings(registryResult.Warnings),
	}, nil
}

func toProjectRegistrySkills(rows []skills.RegistryRow) []project.RegistrySkill {
	registrySkills := make([]project.RegistrySkill, 0, len(rows))
	for _, row := range rows {
		registrySkills = append(registrySkills, project.RegistrySkill{
			ID:           row.ID,
			Name:         row.Name,
			Description:  row.Description,
			Trigger:      row.Trigger,
			Scope:        row.Scope,
			Path:         row.Path,
			CompactRules: row.CompactRules,
			IsCore:       row.IsCore,
		})
	}
	return registrySkills
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
