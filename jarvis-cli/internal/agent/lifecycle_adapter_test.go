package agent

import (
	"archive/tar"
	"compress/gzip"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/lifecycle"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

func TestLifecycleAdapter_BackupTargetsResolveProviderPaths(t *testing.T) {
	a := &ClaudeAgent{home: t.TempDir()}
	adapter := NewLifecycleAdapter(a)

	targets, err := adapter.BackupTargets([]lifecycle.DoctorStep{{AssetID: "orchestrator"}, {AssetID: "skills"}})
	if err != nil {
		t.Fatalf("BackupTargets returned error: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Path == "" || targets[1].Path == "" {
		t.Fatal("backup targets must contain resolved absolute paths")
	}
}

func TestLifecycleAdapter_ApplyCreatesManagedArtifacts(t *testing.T) {
	a := &ClaudeAgent{home: t.TempDir()}
	adapter := NewLifecycleAdapter(a)

	if err := adapter.Apply([]lifecycle.DoctorStep{{AssetID: "orchestrator"}, {AssetID: "skills"}}); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(a.ConfigDir(), "sdd-orchestrator.md")); err != nil {
		t.Fatalf("expected orchestrator artifact to exist after apply: %v", err)
	}
	if stat, err := os.Stat(filepath.Join(a.ConfigDir(), "skills")); err != nil || !stat.IsDir() {
		t.Fatalf("expected skills directory after apply, statErr=%v isDir=%v", err, err == nil && stat.IsDir())
	}
}

func TestLifecycleAdapter_RestoreWritesFilesFromSnapshotArchive(t *testing.T) {
	a := &ClaudeAgent{home: t.TempDir()}
	adapter := NewLifecycleAdapter(a)

	target := filepath.Join(a.ConfigDir(), "sdd-orchestrator.md")
	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := writeSingleFileArchive(archive, target, []byte("restored-content")); err != nil {
		t.Fatalf("writeSingleFileArchive: %v", err)
	}

	manifest := lifecycle.BackupManifest{
		SnapshotID:  "snap-1",
		ArchivePath: archive,
		Entries:     []lifecycle.BackupEntry{{Path: target}},
	}

	restored, err := adapter.Restore(manifest)
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if restored != 1 {
		t.Fatalf("expected 1 restored entry, got %d", restored)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(raw) != "restored-content" {
		t.Fatalf("unexpected restored content: %q", string(raw))
	}
}

func TestLifecycleAdapter_RestoreRejectsPathOutsideAllowedRoots(t *testing.T) {
	a := &ClaudeAgent{home: t.TempDir()}
	adapter := NewLifecycleAdapter(a)

	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := writeSingleFileArchive(archive, "/etc/passwd", []byte("nope")); err != nil {
		t.Fatalf("writeSingleFileArchive: %v", err)
	}

	manifest := lifecycle.BackupManifest{
		SnapshotID:  "snap-escape",
		ArchivePath: archive,
		Entries:     []lifecycle.BackupEntry{{Path: "/etc/passwd"}},
	}

	_, err := adapter.Restore(manifest)
	if err == nil {
		t.Fatal("expected restore to reject path outside allowed roots")
	}
}

func TestLifecycleAdapter_BackupTargetsRejectPrefixEscapingManagedAsset(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), ".claude")
	plan := sddruntime.RuntimePlan{
		Agent: "claude",
		Contract: sddruntime.Contract{ManagedArtifacts: []sddruntime.ManagedArtifact{
			{ID: "escape", RelativePath: "../.claude-evil/file.md"},
		}},
	}
	adapter := NewLifecycleAdapter(&stubLifecycleAgent{name: "claude", configDir: configDir, plan: plan})

	_, err := adapter.BackupTargets([]lifecycle.DoctorStep{{AssetID: "escape"}})
	if err == nil {
		t.Fatal("expected backup target resolution to reject managed asset path escaping provider config dir")
	}
}

func TestLifecycleAdapter_ObserveForwardsProviderSpecificVerifierEvidence(t *testing.T) {
	openCode := sddruntime.ObservedOpenCodeConfig{
		ParseSucceeded: true,
		MCPHivePresent: false,
		SDDSubagentHiveGrantEvidence: map[string][]sddruntime.OpenCodePermissionEvidence{
			"sdd-apply": {{Key: "hive_mem_*", Action: "deny"}},
		},
	}
	claudeTools := map[string][]string{
		"sdd-apply": {"mcp__hive__mem_search"},
	}
	agent := &stubLifecycleAgent{
		name:      "opencode",
		configDir: filepath.Join(t.TempDir(), ".config", "opencode"),
		observed: sddruntime.ObservedRuntime{
			OpenCode:                   openCode,
			ClaudeSDDSubagentHiveTools: claudeTools,
		},
	}
	adapter := NewLifecycleAdapter(agent)

	got, err := adapter.Observe()
	if err != nil {
		t.Fatalf("Observe returned error: %v", err)
	}
	if !got.OpenCode.ParseSucceeded || got.OpenCode.MCPHivePresent {
		t.Fatalf("OpenCode verifier evidence was not forwarded: %+v", got.OpenCode)
	}
	if evidence := got.OpenCode.SDDSubagentHiveGrantEvidence["sdd-apply"]; len(evidence) != 1 || evidence[0].Key != "hive_mem_*" || evidence[0].Action != "deny" {
		t.Fatalf("OpenCode SDD Hive grant evidence was not forwarded: %+v", got.OpenCode.SDDSubagentHiveGrantEvidence)
	}
	if tools := got.ClaudeSDDSubagentHiveTools["sdd-apply"]; len(tools) != 1 || tools[0] != "mcp__hive__mem_search" {
		t.Fatalf("Claude SDD Hive tool evidence was not forwarded: %+v", got.ClaudeSDDSubagentHiveTools)
	}
}

func TestLifecycleAdapter_DoctorDiagnosesStaleClaudeSDDSubagentHiveTools(t *testing.T) {
	home := t.TempDir()
	agentsDir := filepath.Join(home, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("create agents dir: %v", err)
	}
	staleAgent := "---\nname: sdd-apply\ntools: mcp__hive__mem_search\n---\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "sdd-apply.md"), []byte(staleAgent), 0o644); err != nil {
		t.Fatalf("write stale agent: %v", err)
	}
	agent := &ClaudeAgent{home: home}
	adapter := NewLifecycleAdapter(agent)
	engine := lifecycle.NewEngine(lifecycle.EngineDeps{Adapters: map[string]lifecycle.ProviderAdapter{"claude": adapter}, HomeDir: home})

	plan, err := engine.Doctor("claude")
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	step := findLifecycleStep(plan.Steps, "invariant.claude.sdd_hive_tools")
	if step == nil {
		t.Fatalf("expected real adapter doctor path to diagnose stale Claude SDD Hive tools, steps=%+v", plan.Steps)
	}
	if step.SafeToAutoApply || step.ReasonCode != "generated_claude_sdd_hive_tools_outdated" {
		t.Fatalf("unexpected stale Claude SDD Hive tool diagnosis: %+v", *step)
	}
}

func TestLifecycleAdapter_DoctorDiagnosesMalformedOpenCodeConfig(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create opencode dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "opencode.json"), []byte(`{"agent":`), 0o644); err != nil {
		t.Fatalf("write malformed opencode config: %v", err)
	}
	agent := &OpenCodeAgent{home: home}
	adapter := NewLifecycleAdapter(agent)
	engine := lifecycle.NewEngine(lifecycle.EngineDeps{Adapters: map[string]lifecycle.ProviderAdapter{"opencode": adapter}, HomeDir: home})

	plan, err := engine.Doctor("opencode")
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	step := findLifecycleStep(plan.Steps, "invariant.opencode.structure_valid")
	if step == nil {
		t.Fatalf("expected real adapter doctor path to diagnose malformed OpenCode config, steps=%+v", plan.Steps)
	}
	if step.SafeToAutoApply || step.ReasonCode != "generated_opencode_artifact_outdated" {
		t.Fatalf("unexpected malformed OpenCode diagnosis: %+v", *step)
	}
}

func findLifecycleStep(steps []lifecycle.DoctorStep, key string) *lifecycle.DoctorStep {
	for i := range steps {
		if steps[i].CheckKey == key {
			return &steps[i]
		}
	}
	return nil
}

type stubLifecycleAgent struct {
	name      string
	configDir string
	plan      sddruntime.RuntimePlan
	observed  sddruntime.ObservedRuntime
}

func (a *stubLifecycleAgent) Name() string                                               { return a.name }
func (a *stubLifecycleAgent) IsInstalled() bool                                          { return true }
func (a *stubLifecycleAgent) ConfigDir() string                                          { return a.configDir }
func (a *stubLifecycleAgent) MergeConfig(MCPEntry) error                                 { return nil }
func (a *stubLifecycleAgent) WriteInstructions(string, string, []config.SkillInfo) error { return nil }
func (a *stubLifecycleAgent) InstallSkills(fs.FS, []string) error                        { return nil }
func (a *stubLifecycleAgent) InstallOrchestrator([]byte) error                           { return nil }
func (a *stubLifecycleAgent) SupportsOutputStyles() bool                                 { return false }
func (a *stubLifecycleAgent) WriteOutputStyle(*persona.Preset) error                     { return nil }
func (a *stubLifecycleAgent) ClearOutputStyle(string) error                              { return nil }
func (a *stubLifecycleAgent) RuntimePlan() (sddruntime.RuntimePlan, error)               { return a.plan, nil }
func (a *stubLifecycleAgent) ObserveRuntime() (sddruntime.ObservedRuntime, error) {
	return a.observed, nil
}
func (a *stubLifecycleAgent) InstallPromptHook(fs.FS) error   { return nil }
func (a *stubLifecycleAgent) InstallSessionHooks(fs.FS) error { return nil }

func writeSingleFileArchive(archivePath, absolutePath string, content []byte) (err error) {
	f, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	gz := gzip.NewWriter(f)
	defer func() {
		if cerr := gz.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	tw := tar.NewWriter(gz)
	defer func() {
		if cerr := tw.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	name := absolutePath
	if len(name) > 0 && name[0] == filepath.Separator {
		name = name[1:]
	}
	hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	return nil
}
