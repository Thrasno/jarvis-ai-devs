package agent

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/persona"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

// TestAgentInstaller_InterfaceSatisfiedByClaudeAgent verifies that ClaudeAgent
// implements the AgentInstaller optional capability interface.
func TestAgentInstaller_InterfaceSatisfiedByClaudeAgent(t *testing.T) {
	a := newClaudeAgent(emptyFS)
	if _, ok := any(a).(AgentInstaller); !ok {
		t.Fatal("ClaudeAgent must implement AgentInstaller")
	}
}

// TestInstallAgentsIfSupported_NoopsForNonImplementors verifies that
// InstallAgentsIfSupported returns nil for an agent that does not implement
// AgentInstaller, without touching the filesystem.
func TestInstallAgentsIfSupported_NoopsForNonImplementors(t *testing.T) {
	a := &unsupportedAgentInstaller{home: t.TempDir()}

	if err := InstallAgentsIfSupported(a, fstest.MapFS{}); err != nil {
		t.Fatalf("InstallAgentsIfSupported returned unexpected error: %v", err)
	}
}

// TestInstallAgentsIfSupported_CallsInstallAgentsWhenSupported verifies that
// InstallAgentsIfSupported delegates to InstallAgents when the agent implements
// AgentInstaller.
func TestInstallAgentsIfSupported_CallsInstallAgentsWhenSupported(t *testing.T) {
	stub := &stubAgentInstaller{}
	testFS := fstest.MapFS{
		"agent-a.md": {Data: []byte("# Agent A")},
	}

	if err := InstallAgentsIfSupported(stub, testFS); err != nil {
		t.Fatalf("InstallAgentsIfSupported: %v", err)
	}

	if stub.callCount != 1 {
		t.Fatalf("expected InstallAgents to be called once, got %d", stub.callCount)
	}
}

// stubAgentInstaller is a minimal Agent+AgentInstaller stub for testing.
type stubAgentInstaller struct {
	callCount int
	lastFS    fs.FS
}

func (s *stubAgentInstaller) Name() string      { return "stub" }
func (s *stubAgentInstaller) IsInstalled() bool { return true }
func (s *stubAgentInstaller) ConfigDir() string { return "" }
func (s *stubAgentInstaller) MergeConfig(MCPEntry) error {
	return nil
}
func (s *stubAgentInstaller) WriteInstructions(string, string, []config.SkillInfo) error {
	return nil
}
func (s *stubAgentInstaller) InstallSkills(fs.FS, []string) error    { return nil }
func (s *stubAgentInstaller) InstallOrchestrator([]byte) error       { return nil }
func (s *stubAgentInstaller) SupportsOutputStyles() bool             { return false }
func (s *stubAgentInstaller) WriteOutputStyle(*persona.Preset) error { return nil }
func (s *stubAgentInstaller) ClearOutputStyle(string) error          { return nil }
func (s *stubAgentInstaller) RuntimePlan() (sddruntime.RuntimePlan, error) {
	return sddruntime.Build("claude")
}
func (s *stubAgentInstaller) ObserveRuntime() (sddruntime.ObservedRuntime, error) {
	return sddruntime.ObservedRuntime{}, nil
}
func (s *stubAgentInstaller) InstallPromptHook(fs.FS) error  { return nil }
func (s *stubAgentInstaller) InstallSessionHooks(fs.FS) error { return nil }
func (s *stubAgentInstaller) InstallAgents(agentsFS fs.FS) error {
	s.callCount++
	s.lastFS = agentsFS
	return nil
}

// unsupportedAgentInstaller is an agent stub that does NOT implement AgentInstaller.
type unsupportedAgentInstaller struct{ home string }

func (a *unsupportedAgentInstaller) Name() string      { return "unsupported-installer" }
func (a *unsupportedAgentInstaller) IsInstalled() bool { return true }
func (a *unsupportedAgentInstaller) ConfigDir() string {
	return a.home + "/.unsupported-installer"
}
func (a *unsupportedAgentInstaller) MergeConfig(MCPEntry) error               { return nil }
func (a *unsupportedAgentInstaller) WriteInstructions(string, string, []config.SkillInfo) error {
	return nil
}
func (a *unsupportedAgentInstaller) InstallSkills(fs.FS, []string) error    { return nil }
func (a *unsupportedAgentInstaller) InstallOrchestrator([]byte) error       { return nil }
func (a *unsupportedAgentInstaller) SupportsOutputStyles() bool             { return false }
func (a *unsupportedAgentInstaller) WriteOutputStyle(*persona.Preset) error { return nil }
func (a *unsupportedAgentInstaller) ClearOutputStyle(string) error          { return nil }
func (a *unsupportedAgentInstaller) RuntimePlan() (sddruntime.RuntimePlan, error) {
	return sddruntime.Build("claude")
}
func (a *unsupportedAgentInstaller) ObserveRuntime() (sddruntime.ObservedRuntime, error) {
	return sddruntime.ObservedRuntime{}, nil
}
func (a *unsupportedAgentInstaller) InstallPromptHook(fs.FS) error   { return nil }
func (a *unsupportedAgentInstaller) InstallSessionHooks(fs.FS) error { return nil }
