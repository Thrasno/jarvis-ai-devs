package sddruntime

import "testing"

func TestDefaultContract_HasCanonicalInvariants(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, c Contract)
	}{
		{
			name: "version and registry path are deterministic",
			fn: func(t *testing.T, c Contract) {
				t.Helper()
				if c.Version != "2026.05" {
					t.Fatalf("expected contract version 2026.05, got %q", c.Version)
				}
				if c.RegistryPath != ".jarvis/skill-registry.md" {
					t.Fatalf("expected registry path .jarvis/skill-registry.md, got %q", c.RegistryPath)
				}
			},
		},
		{
			name: "model assignments include required SDD phases",
			fn: func(t *testing.T, c Contract) {
				t.Helper()
				expected := map[string]string{
					"orchestrator": "opus",
					"sdd-apply":    "sonnet",
					"sdd-verify":   "sonnet",
					"sdd-archive":  "haiku",
					"default":      "sonnet",
				}
				for phase, want := range expected {
					got, ok := c.ModelAssignments[phase]
					if !ok {
						t.Fatalf("missing model assignment for phase %q", phase)
					}
					if got != want {
						t.Fatalf("model assignment mismatch for %q: got %q want %q", phase, got, want)
					}
				}
			},
		},
		{
			name: "managed artifact catalog includes instructions orchestrator and skills",
			fn: func(t *testing.T, c Contract) {
				t.Helper()
				if len(c.ManagedArtifacts) == 0 {
					t.Fatal("managed artifact catalog should not be empty")
				}
				mustContain := []string{"instructions", "orchestrator", "skills"}
				for _, id := range mustContain {
					if !hasArtifactID(c.ManagedArtifacts, id) {
						t.Fatalf("managed artifacts must contain id %q", id)
					}
				}
			},
		},
	}

	contract := DefaultContract()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t, contract)
		})
	}
}

func TestBuild_DerivesAgentSpecificPlanFromCanonicalContract(t *testing.T) {
	tests := []struct {
		name                 string
		agent                string
		wantInstructionPath  string
		wantSettingsPath     string
		wantOrchestratorPath string
	}{
		{
			name:                 "claude plan",
			agent:                "claude",
			wantInstructionPath:  ".claude/CLAUDE.md",
			wantSettingsPath:     ".claude/settings.json",
			wantOrchestratorPath: ".claude/sdd-orchestrator.md",
		},
		{
			name:                 "opencode plan",
			agent:                "opencode",
			wantInstructionPath:  ".config/opencode/AGENTS.md",
			wantSettingsPath:     ".config/opencode/opencode.json",
			wantOrchestratorPath: ".config/opencode/sdd-orchestrator.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := Build(tt.agent)
			if err != nil {
				t.Fatalf("Build(%q) returned error: %v", tt.agent, err)
			}

			if plan.Agent != tt.agent {
				t.Fatalf("expected agent %q, got %q", tt.agent, plan.Agent)
			}
			if plan.Contract.Version != "2026.05" {
				t.Fatalf("expected contract version 2026.05, got %q", plan.Contract.Version)
			}
			if plan.Paths.Instructions != tt.wantInstructionPath {
				t.Fatalf("instructions path mismatch: got %q want %q", plan.Paths.Instructions, tt.wantInstructionPath)
			}
			if plan.Paths.Settings != tt.wantSettingsPath {
				t.Fatalf("settings path mismatch: got %q want %q", plan.Paths.Settings, tt.wantSettingsPath)
			}
			if plan.Paths.Orchestrator != tt.wantOrchestratorPath {
				t.Fatalf("orchestrator path mismatch: got %q want %q", plan.Paths.Orchestrator, tt.wantOrchestratorPath)
			}
			if plan.Paths.Registry != ".jarvis/skill-registry.md" {
				t.Fatalf("registry path mismatch: got %q", plan.Paths.Registry)
			}
		})
	}
}

func TestBuild_RejectsUnknownAgent(t *testing.T) {
	_, err := Build("cursor")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestNewIntegrityReport_DefaultsToPassAndContractVersion(t *testing.T) {
	report := NewIntegrityReport("claude", DefaultContract())

	if report.Status != StatusPass {
		t.Fatalf("expected initial status %q, got %q", StatusPass, report.Status)
	}
	if report.Agent != "claude" {
		t.Fatalf("agent mismatch: got %q", report.Agent)
	}
	if report.ContractVersion != "2026.05" {
		t.Fatalf("contract version mismatch: got %q", report.ContractVersion)
	}
}

func TestIntegrityReport_AddCheckEscalatesStatusBySeverity(t *testing.T) {
	report := NewIntegrityReport("opencode", DefaultContract())

	report.AddCheck(CheckResult{Key: "non_owned_change", Status: StatusWarn, DriftClass: DriftNonOwned})
	if report.Status != StatusWarn {
		t.Fatalf("expected status %q after warn check, got %q", StatusWarn, report.Status)
	}

	report.AddCheck(CheckResult{Key: "missing_orchestrator", Status: StatusFail, DriftClass: DriftOwned})
	if report.Status != StatusFail {
		t.Fatalf("expected status %q after fail check, got %q", StatusFail, report.Status)
	}
}

func hasArtifactID(items []ManagedArtifact, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
