package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateHome sets HOME to a fresh temp dir and registers cleanup.
// This is mandatory to prevent tests from touching the real ~/.jarvis.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	setHomeEnv(t, home)
	return home
}

func setHomeEnv(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
}

// completeManifest is the manifest half of a machine that finished an install.
// ~/.jarvis/state.yaml owns the persona, the skills and the agents, so readiness
// is a joint question and this stands in for that store.
var completeManifest = RecordedInstall{Complete: true, Populated: true}

func TestReconfigureReadiness_FalseWhenNoFile(t *testing.T) {
	isolateHome(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IsReadyForReconfigure(completeManifest) {
		t.Fatal("expected not ready for a fresh home dir with no config file")
	}
	if got := cfg.ConfigStatus(RecordedInstall{}); got != ConfigStatusSetup {
		t.Fatalf("expected setup for a fresh home dir, got %q", got)
	}
}

func TestReconfigureReadiness_FalseWhenEmpty(t *testing.T) {
	home := isolateHome(t)

	// Create the directory and an empty config file.
	jarvisDir := filepath.Join(home, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatalf("create .jarvis dir: %v", err)
	}
	emptyPath := filepath.Join(jarvisDir, "config.yaml")
	if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
		t.Fatalf("write empty config: %v", err)
	}

	// An empty file records no completed install, so the machine is not ready
	// however complete the manifest is.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IsReadyForReconfigure(completeManifest) {
		t.Fatal("expected not ready when config file is empty")
	}
}

func TestReconfigureReadiness_TrueWhenValid(t *testing.T) {
	isolateHome(t)

	cfg := &AppConfig{
		SchemaVersion: currentSchemaVersion,
		APIURL:        DefaultAPIURL,
		Install:       InstallState{Completed: true},
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.IsReadyForReconfigure(completeManifest) {
		t.Fatal("expected ready after saving a completed install with a complete manifest")
	}
}

func TestSave_CreatesDirectoryIfMissing(t *testing.T) {
	home := isolateHome(t)

	// ~/.jarvis does not exist yet.
	jarvisDir := filepath.Join(home, ".jarvis")
	if _, err := os.Stat(jarvisDir); !os.IsNotExist(err) {
		t.Fatal("expected .jarvis dir to NOT exist before Save")
	}

	cfg := &AppConfig{
		SchemaVersion: currentSchemaVersion,
		APIURL:        DefaultAPIURL,
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Directory must now exist.
	if _, err := os.Stat(jarvisDir); err != nil {
		t.Fatalf("expected .jarvis dir to exist after Save, got: %v", err)
	}
	// Config file must exist inside it.
	cfgPath := filepath.Join(jarvisDir, "config.yaml")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("expected config.yaml to exist after Save, got: %v", err)
	}
}

func TestSave_RoundTrip(t *testing.T) {
	isolateHome(t)

	original := &AppConfig{
		SchemaVersion: currentSchemaVersion,
		APIURL:        "https://custom.api.example.com",
		Cloud:         &CloudConfig{Email: "rhodey@war.machine", SyncConfigured: true},
		Install: InstallState{
			Mode:      "reconfigure",
			Completed: true,
		},
		Version: "2.0.0",
	}

	if err := Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}

	if loaded.Cloud == nil || loaded.Cloud.Email != original.Cloud.Email {
		t.Errorf("Cloud.Email: got %#v, want %q", loaded.Cloud, original.Cloud.Email)
	}
	if loaded.APIURL != original.APIURL {
		t.Errorf("APIURL: got %q, want %q", loaded.APIURL, original.APIURL)
	}
	if loaded.Version != original.Version {
		t.Errorf("Version: got %q, want %q", loaded.Version, original.Version)
	}
	if loaded.Install.Mode != original.Install.Mode || !loaded.Install.Completed {
		t.Errorf("Install: got %+v, want %+v", loaded.Install, original.Install)
	}
	if !loaded.Cloud.SyncConfigured {
		t.Error("Cloud.SyncConfigured did not survive the round trip")
	}
}

func TestLoad_MigratesLegacyV1ConfigToTheCurrentSchema(t *testing.T) {
	home := isolateHome(t)
	legacy := strings.Join([]string{
		"api_url: https://hivemem.dev",
		"email: legacy@example.com",
		"preset: argentino",
		"configured_agents:",
		"  - claude",
	}, "\n")
	if err := os.MkdirAll(filepath.Join(home, ".jarvis"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".jarvis", "config.yaml"), []byte(legacy), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load legacy config: %v", err)
	}

	if cfg.SchemaVersion != currentSchemaVersion {
		t.Fatalf("expected schema_version=%d after migration, got %d", currentSchemaVersion, cfg.SchemaVersion)
	}
	if cfg.Cloud == nil || cfg.Cloud.Email != "legacy@example.com" {
		t.Fatalf("expected migrated cloud email, got %#v", cfg.Cloud)
	}
	// The legacy preset and configured_agents keys belong to the manifest now.
	// state.Migrate carries them across; this store must not decode them, and it
	// must not drop them from the file either, which
	// TestSaveThenMigrate_DoesNotStrandReplayFieldsOnAnUnmigratedMachine covers.
}

func TestConfigStatus_ReadyWithoutCloudEmail(t *testing.T) {
	cfg := &AppConfig{
		SchemaVersion: currentSchemaVersion,
		APIURL:        DefaultAPIURL,
		Install:       InstallState{Completed: true},
	}

	if !cfg.IsReadyForReconfigure(completeManifest) {
		t.Fatal("expected ready for a complete local install without a cloud email")
	}
	if got := cfg.ConfigStatus(completeManifest); got != ConfigStatusReconfigure {
		t.Fatalf("expected ConfigStatusReconfigure, got %q", got)
	}
}

// TestConfigStatus_RecoverWhenPartiallyConfigured covers the half-installed
// machine: config.yaml records a completed install but the manifest records no
// persona, skills or agents, so the installation is damaged rather than absent.
func TestConfigStatus_RecoverWhenPartiallyConfigured(t *testing.T) {
	cfg := &AppConfig{
		SchemaVersion: currentSchemaVersion,
		APIURL:        DefaultAPIURL,
		Install:       InstallState{Completed: true},
	}

	partial := RecordedInstall{Complete: false, Populated: false}
	if cfg.IsReadyForReconfigure(partial) {
		t.Fatal("expected not ready when the manifest records no installation")
	}
	if got := cfg.ConfigStatus(partial); got != ConfigStatusRecover {
		t.Fatalf("expected ConfigStatusRecover for partial state, got %q", got)
	}
}

// TestConfigStatus_RecoverWhenOnlyTheManifestCarriesState covers the mirror
// case: config.yaml looks untouched but the manifest records real choices, which
// is a damaged installation and not a fresh machine.
func TestConfigStatus_RecoverWhenOnlyTheManifestCarriesState(t *testing.T) {
	cfg := &AppConfig{SchemaVersion: currentSchemaVersion, APIURL: DefaultAPIURL}

	got := cfg.ConfigStatus(RecordedInstall{Populated: true})
	if got != ConfigStatusRecover {
		t.Fatalf("config status = %q, want recover when only the manifest carries state", got)
	}
	if fresh := cfg.ConfigStatus(RecordedInstall{}); fresh != ConfigStatusSetup {
		t.Fatalf("config status = %q, want setup when neither store carries state", fresh)
	}
}

// TestHasStoredCloudLink_IsTheSeamTheScopeDefaultReadsFrom covers what used to
// be an unexported helper behind the scope default. ~/.jarvis/state.yaml owns
// the scope and decides what an unrecorded one means, but the evidence lives
// here, so this store has to expose it and get it right for a legacy config
// whose only cloud trace is the flat email key.
func TestHasStoredCloudLink_IsTheSeamTheScopeDefaultReadsFrom(t *testing.T) {
	t.Run("legacy flat email counts", func(t *testing.T) {
		home := isolateHome(t)
		writeRawConfig(t, home, "api_url: https://hivemem.dev\nemail: legacy@example.com\n")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !cfg.HasStoredCloudLink() {
			t.Fatalf("expected a stored cloud link for a legacy email config, got %+v", cfg.Cloud)
		}
	})

	t.Run("no cloud trace", func(t *testing.T) {
		home := isolateHome(t)
		writeRawConfig(t, home, "api_url: https://hivemem.dev\n")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.HasStoredCloudLink() {
			t.Fatal("expected no stored cloud link without any cloud trace")
		}
	})

	t.Run("sync configured without an email counts", func(t *testing.T) {
		cfg := &AppConfig{Cloud: &CloudConfig{SyncConfigured: true}}
		if !cfg.HasStoredCloudLink() {
			t.Fatal("a configured sync is a stored cloud link even without an email")
		}
	})

	t.Run("nil config", func(t *testing.T) {
		var cfg *AppConfig
		if cfg.HasStoredCloudLink() {
			t.Fatal("a nil config has no stored cloud link")
		}
	})
}

func TestLoad_ReturnsErrorWhenFileCorrupt(t *testing.T) {
	home := isolateHome(t)

	// Write invalid YAML to the config path.
	jarvisDir := filepath.Join(home, ".jarvis")
	if err := os.MkdirAll(jarvisDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgPath := filepath.Join(jarvisDir, "config.yaml")
	corruptYAML := []byte("email: [\nbad yaml: {unclosed")
	if err := os.WriteFile(cfgPath, corruptYAML, 0644); err != nil {
		t.Fatalf("write corrupt config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected Load() to return an error for corrupt YAML, got nil")
	}
}

func TestSave_ReturnsErrorOnNilConfig(t *testing.T) {
	isolateHome(t)
	err := Save(nil)
	if err == nil || !strings.Contains(err.Error(), "config is nil") {
		t.Fatalf("expected nil-config error, got %v", err)
	}
}

func TestLoad_DefaultConfigRespectsEnvOverride(t *testing.T) {
	isolateHome(t)
	t.Setenv("JARVIS_API_URL", "https://override.example")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIURL != "https://override.example" {
		t.Fatalf("expected APIURL override, got %q", cfg.APIURL)
	}
}

func TestConfigStatusSetupWhenConfigNil(t *testing.T) {
	var cfg *AppConfig
	if got := cfg.ConfigStatus(completeManifest); got != ConfigStatusSetup {
		t.Fatalf("expected ConfigStatusSetup for nil cfg, got %q", got)
	}
}

// TestIsReadyForReconfigure_FailsWhenTheManifestRecordsNoInstall keeps the
// guarantee the old per-agent check carried: a machine whose recorded agents are
// missing is not ready. Which parts make the manifest half complete is
// State.RecordsCompleteInstall's business; this asserts the joint answer honours
// it even when config.yaml looks finished.
func TestIsReadyForReconfigure_FailsWhenTheManifestRecordsNoInstall(t *testing.T) {
	cfg := &AppConfig{
		SchemaVersion: currentSchemaVersion,
		APIURL:        DefaultAPIURL,
		Install:       InstallState{Completed: true},
	}

	if cfg.IsReadyForReconfigure(RecordedInstall{Complete: false, Populated: true}) {
		t.Fatal("expected not ready when the manifest records no complete install")
	}
}

// TestIsReadyForReconfigure_FailsOnAnOlderSchema keeps the schema gate.
func TestIsReadyForReconfigure_FailsOnAnOlderSchema(t *testing.T) {
	cfg := &AppConfig{
		SchemaVersion: currentSchemaVersion - 1,
		APIURL:        DefaultAPIURL,
		Install:       InstallState{Completed: true},
	}

	if cfg.IsReadyForReconfigure(completeManifest) {
		t.Fatal("expected not ready before the config has been migrated to the current schema")
	}
}

// TestLayer1Content_ContainsAllRequiredSections verifies that Layer1Content includes
// behavior-only runtime guardrails while deferring protocol details to protocol.hive.
func TestLayer1Content_ContainsAllRequiredSections(t *testing.T) {
	content := Layer1Content()

	required := []string{
		// PROJECT CONTEXT
		"PROJECT CONTEXT",
		"git remote get-url origin",
		"basename",
		`"default"`,
		// Canonical Hive protocol boundary
		"Hive Protocol Source Boundary",
		"protocol.hive",
		"jarvis-cli/embed/hive-protocol.md",
		"Layer1 MUST NOT duplicate the Hive protocol body",
		// Contextual skill loading guardrail
		"Contextual Skill Loading Self-Check",
		"Before every response",
		"matches an installed skill",
		"load that skill before task-specific work",
		// Persona/artifact language guardrail
		"Persona Scope and Artifact Language",
		"Persona voice applies only to direct user replies",
		"Generated technical artifacts default to English",
		"Hive",
		"jarvis CLI",
		".jarvis/skill-registry.md",
		".jarvis/skills/<skill>/SKILL.md",
		// Hive-specific behavior summary
		"scope",
		// SDD DAG without retired QA gate
		"SDD DAG: `proposal → specs → tasks → apply → verify → archive`",
		"Apply-progress continuity",
		// Hive-specific
		"project",
	}

	for _, want := range required {
		if !strings.Contains(content, want) {
			t.Errorf("Layer1Content missing required string %q", want)
		}
	}

	for _, protocolBodyMarker := range []string{"PROACTIVE SAVE TRIGGERS", "SESSION CLOSE PROTOCOL", "FORMAT FOR mem_save"} {
		if strings.Contains(content, protocolBodyMarker) {
			t.Errorf("Layer1Content must not duplicate protocol.hive body marker %q", protocolBodyMarker)
		}
	}
}

// TestLayer1Content_NoEngramReferences verifies that Layer1Content contains no
// references to "Engram" (the old memory system) in any casing.
func TestLayer1Content_NoEngramReferences(t *testing.T) {
	content := Layer1Content()

	if strings.Contains(content, "Engram") {
		t.Error("Layer1Content must not contain 'Engram' (old memory system reference)")
	}
	if strings.Contains(content, "engram") {
		t.Error("Layer1Content must not contain 'engram' (old memory system reference)")
	}
}

func TestLayer1Content_NoRetiredSDDQAReferences(t *testing.T) {
	content := Layer1Content()

	for _, forbidden := range []string{"sdd-qa", "qa-signoff", "qa-checklist"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("Layer1Content must not contain retired QA gate reference %q", forbidden)
		}
	}
}
