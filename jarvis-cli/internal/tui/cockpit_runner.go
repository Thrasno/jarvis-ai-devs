package tui

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"strings"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/apiclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

type cockpitRunner interface {
	ConfigSummary(context.Context) (string, error)
	ApplyPersonaPreset(context.Context, personaApplyRequest) (string, error)
	LoginHiveCloud(context.Context, string, string) (string, error)
	Doctor(context.Context, string) (lifecycle.DoctorPlan, error)
	Verify(context.Context, string) (lifecycle.VerifyResult, error)
	Backup(context.Context, string) (string, error)
	Restore(context.Context, string, string) (lifecycle.RestoreResult, error)
	ReconcileDryRun(context.Context, string) (lifecycle.DoctorPlan, error)
	Reconcile(context.Context, string) (lifecycle.ReconcileResult, error)
	UninstallDryRun(context.Context, string) (lifecycle.DoctorPlan, error)
	Uninstall(context.Context, string) (lifecycle.UninstallResult, error)
}

type personaApplyRequest struct {
	PresetName           string
	PersonaFS            fs.FS
	Agents               []agent.Agent
	Skills               []config.SkillInfo
	PreviousPresetSlug   string
	PreviousPresetSource persona.PresetSource
}

type defaultCockpitRunner struct{}

type cockpitLifecycleService interface {
	Doctor(provider string) (lifecycle.DoctorPlan, error)
	Verify(provider string) (lifecycle.VerifyResult, error)
	Backup(provider, sourceOperation string) (string, error)
	Restore(provider, snapshotID string) (lifecycle.RestoreResult, error)
	ReconcileDryRun(provider string) (lifecycle.DoctorPlan, error)
	Reconcile(provider string) (lifecycle.ReconcileResult, error)
	Uninstall(provider, mode string) (lifecycle.UninstallResult, error)
}

func (defaultCockpitRunner) ConfigSummary(context.Context) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	agents := strings.Join(cfg.ConfiguredAgents, ", ")
	if agents == "" {
		agents = "(none)"
	}
	version := cfg.Version
	if version == "" {
		version = "(unset)"
	}
	return fmt.Sprintf("preset=%s\napi_url=%s\nemail=%s\nconfigured_agents=%s\nversion=%s", cfg.PersonaPreset, cfg.APIURL, cfg.Email, agents, version), nil
}

func (defaultCockpitRunner) ApplyPersonaPreset(_ context.Context, req personaApplyRequest) (string, error) {
	resolved, err := resolveWizardPresetSelection(req.PersonaFS, req.PresetName, nil)
	if err != nil {
		return "", fmt.Errorf("resolve preset: %w", err)
	}
	pipelineAgents := make([]persona.PresetAgent, 0, len(req.Agents))
	for _, a := range req.Agents {
		pipelineAgents = append(pipelineAgents, a)
	}
	if err := persona.ApplyPresetPipeline(pipelineAgents, resolved, persona.ApplyOptions{
		Layer1:               config.Layer1Content(),
		Skills:               req.Skills,
		PreviousPresetSlug:   req.PreviousPresetSlug,
		PreviousPresetSource: req.PreviousPresetSource,
		PersistConfig:        true,
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("preset=%s source=%s agents=%d", resolved.Slug, resolved.Source, len(req.Agents)), nil
}

func (defaultCockpitRunner) LoginHiveCloud(_ context.Context, email, password string) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	apiURL := strings.TrimSpace(cfg.APIURL)
	if apiURL == "" {
		apiURL = config.DefaultAPIURL
	}
	resp, err := apiclient.New(apiURL).Login(email, password)
	if err != nil {
		return "", err
	}
	resolvedEmail := strings.TrimSpace(resp.User.Email)
	if resolvedEmail == "" {
		resolvedEmail = strings.TrimSpace(email)
	}
	enable := true
	if err := config.WriteSyncCredentials(apiURL, resolvedEmail, password, &enable); err != nil {
		return "", fmt.Errorf("write sync credentials: %w", err)
	}
	if cfg.Cloud == nil {
		cfg.Cloud = &config.CloudConfig{}
	}
	cfg.Cloud.Email = resolvedEmail
	cfg.Cloud.SyncConfigured = true
	cfg.Email = resolvedEmail
	cfg.Scope = config.ScopeLocalCloud
	if err := config.Save(cfg); err != nil {
		return "", fmt.Errorf("save config: %w", err)
	}
	return resolvedEmail, nil
}

func (r defaultCockpitRunner) Doctor(_ context.Context, provider string) (lifecycle.DoctorPlan, error) {
	engine := newCockpitLifecycleEngine()
	plans := []lifecycle.DoctorPlan{}
	for _, target := range cockpitProviderTargets(provider) {
		plan, err := engine.Doctor(target)
		if err != nil {
			return lifecycle.DoctorPlan{}, err
		}
		plans = append(plans, plan)
	}
	return mergeCockpitDoctorPlans(provider, plans), nil
}

func (r defaultCockpitRunner) Verify(_ context.Context, provider string) (lifecycle.VerifyResult, error) {
	engine := newCockpitLifecycleEngine()
	result := lifecycle.VerifyResult{Status: sddruntime.StatusPass}
	for _, target := range cockpitProviderTargets(provider) {
		providerResult, err := engine.Verify(target)
		if err != nil {
			return lifecycle.VerifyResult{}, err
		}
		if providerResult.Status == sddruntime.StatusFail {
			result.Status = sddruntime.StatusFail
		}
		result.Report.Checks = append(result.Report.Checks, providerResult.Report.Checks...)
	}
	return result, nil
}

func (r defaultCockpitRunner) Backup(_ context.Context, provider string) (string, error) {
	engine := newCockpitLifecycleEngine()
	snapshots := []string{}
	for _, target := range cockpitProviderTargets(provider) {
		snapshot, err := engine.Backup(target, "backup")
		if err != nil {
			return "", err
		}
		snapshots = append(snapshots, target+"="+snapshot)
	}
	return strings.Join(snapshots, ","), nil
}

func (r defaultCockpitRunner) Restore(_ context.Context, provider, snapshotID string) (lifecycle.RestoreResult, error) {
	engine := newCockpitLifecycleEngine()
	var result lifecycle.RestoreResult
	for _, target := range cockpitProviderTargets(provider) {
		providerResult, err := engine.Restore(target, snapshotID)
		if err != nil {
			return lifecycle.RestoreResult{}, err
		}
		result.Restored += providerResult.Restored
	}
	return result, nil
}

func (r defaultCockpitRunner) ReconcileDryRun(_ context.Context, provider string) (lifecycle.DoctorPlan, error) {
	engine := newCockpitLifecycleEngine()
	plans := []lifecycle.DoctorPlan{}
	for _, target := range cockpitProviderTargets(provider) {
		plan, err := engine.ReconcileDryRun(target)
		if err != nil {
			return lifecycle.DoctorPlan{}, err
		}
		plans = append(plans, plan)
	}
	return mergeCockpitDoctorPlans(provider, plans), nil
}

func (r defaultCockpitRunner) Reconcile(_ context.Context, provider string) (lifecycle.ReconcileResult, error) {
	engine := newCockpitLifecycleEngine()
	var result lifecycle.ReconcileResult
	for _, target := range cockpitProviderTargets(provider) {
		providerResult, err := engine.Reconcile(target)
		if err != nil {
			return lifecycle.ReconcileResult{}, err
		}
		result.Applied += providerResult.Applied
		result.ManualRequired += providerResult.ManualRequired
		result.SkippedNonOwned = append(result.SkippedNonOwned, providerResult.SkippedNonOwned...)
	}
	return result, nil
}

func (r defaultCockpitRunner) UninstallDryRun(_ context.Context, provider string) (lifecycle.DoctorPlan, error) {
	return r.Doctor(context.Background(), provider)
}

func (r defaultCockpitRunner) Uninstall(_ context.Context, provider string) (lifecycle.UninstallResult, error) {
	engine := newCockpitLifecycleEngine()
	if provider == "all" {
		return engine.Uninstall("all", "all")
	}
	var result lifecycle.UninstallResult
	for _, target := range cockpitProviderTargets(provider) {
		providerResult, err := engine.Uninstall(target, "provider")
		if err != nil {
			return lifecycle.UninstallResult{}, err
		}
		result.Applied += providerResult.Applied
		if providerResult.VerifyStatus == sddruntime.StatusFail || result.VerifyStatus == "" {
			result.VerifyStatus = providerResult.VerifyStatus
		}
		result.LedgerRemoved = result.LedgerRemoved || providerResult.LedgerRemoved
	}
	return result, nil
}

var newCockpitLifecycleEngine = func() cockpitLifecycleService {
	adapters := map[string]lifecycle.ProviderAdapter{}
	for _, a := range agent.Detect(jarvis.TemplatesFS) {
		adapters[a.Name()] = agent.NewLifecycleAdapter(a)
	}
	return lifecycle.NewEngine(cockpitLifecycleEngineDeps(adapters))
}

func cockpitLifecycleEngineDeps(adapters map[string]lifecycle.ProviderAdapter) lifecycle.EngineDeps {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return lifecycle.EngineDeps{Adapters: adapters, HomeDir: home}
}

func cockpitProviderTargets(provider string) []string {
	if provider == "all" {
		return []string{"claude", "opencode"}
	}
	return []string{provider}
}

func mergeCockpitDoctorPlans(provider string, plans []lifecycle.DoctorPlan) lifecycle.DoctorPlan {
	merged := lifecycle.DoctorPlan{Provider: provider, Status: sddruntime.StatusPass, ReadOnly: true}
	for _, plan := range plans {
		if plan.Status == sddruntime.StatusFail {
			merged.Status = sddruntime.StatusFail
		}
		merged.Steps = append(merged.Steps, plan.Steps...)
	}
	return merged
}
