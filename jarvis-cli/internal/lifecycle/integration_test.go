package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/sddruntime"
)

func TestLifecycleMatrix_VerifyDoctorReconcileBackupRestoreUninstall(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, settingsPath, `{"mcp":true}`)

	adapter := &fakeProviderAdapter{
		name: "claude",
		observed: ObservedProviderState{Artifacts: map[string]sddruntime.ObservedArtifact{
			"instructions": {Exists: true, MarkersValid: true},
			"orchestrator": {Exists: false},
			"skills":       {Exists: true},
		}},
	}
	engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: home})

	if verifyResult, err := engine.Verify("claude"); err != nil || verifyResult.Status != sddruntime.StatusFail {
		t.Fatalf("Verify() status=%q err=%v, want fail and nil error", verifyResult.Status, err)
	}
	plan, err := engine.Doctor("claude")
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("expected doctor plan with owned drift operations")
	}
	if _, err := engine.Reconcile("claude"); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if snapshot, err := engine.Backup("claude", "manual"); err != nil || snapshot == "" {
		t.Fatalf("Backup() snapshot=%q err=%v, want snapshot and nil error", snapshot, err)
	}

	manifest, err := engine.backups.CreateSnapshot("restore", []BackupTarget{{Path: settingsPath}})
	if err != nil {
		t.Fatalf("CreateSnapshot for restore setup: %v", err)
	}
	if result, err := engine.Restore("claude", manifest.SnapshotID); err != nil || result.Restored != 1 {
		t.Fatalf("Restore() restored=%d err=%v, want restored=1 and nil error", result.Restored, err)
	}

	if result, err := engine.Uninstall("claude", "provider"); err != nil || result.Applied == 0 {
		t.Fatalf("Uninstall() applied=%d err=%v, want applied>0 and nil error", result.Applied, err)
	}
}

func TestLifecycleMatrix_FailurePathsExposeStructuredEnvelope(t *testing.T) {
	tests := []struct {
		name      string
		engine    func(t *testing.T) *Engine
		run       func(*Engine) error
		wantCode  string
		wantStage string
	}{
		{
			name: "reconcile apply failure",
			engine: func(t *testing.T) *Engine {
				home := t.TempDir()
				adapter := &fakeProviderAdapter{
					name:     "claude",
					applyErr: errors.New("apply boom"),
					observed: ObservedProviderState{Artifacts: map[string]sddruntime.ObservedArtifact{"orchestrator": {Exists: false}}},
				}
				return NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": adapter}, HomeDir: home})
			},
			run: func(e *Engine) error {
				_, err := e.Reconcile("claude")
				return err
			},
			wantCode:  "apply_failed",
			wantStage: "apply",
		},
		{
			name: "restore checksum mismatch",
			engine: func(t *testing.T) *Engine {
				home := t.TempDir()
				path := filepath.Join(home, ".claude", "settings.json")
				writeFile(t, path, `{"before":true}`)
				engine := NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": &fakeProviderAdapter{name: "claude"}}, HomeDir: home})
				manifest, err := engine.backups.CreateSnapshot("restore", []BackupTarget{{Path: path}})
				if err != nil {
					t.Fatalf("CreateSnapshot setup: %v", err)
				}
				writeFile(t, manifest.ArchivePath, "not-a-valid-archive")
				return engine
			},
			run: func(e *Engine) error {
				_, err := e.Restore("claude", lastSnapshotID(t, e))
				return err
			},
			wantCode:  "restore_checksum_mismatch",
			wantStage: "restore",
		},
		{
			name: "uninstall unsupported mode",
			engine: func(t *testing.T) *Engine {
				return NewEngine(EngineDeps{Adapters: map[string]ProviderAdapter{"claude": &fakeProviderAdapter{name: "claude"}}, HomeDir: t.TempDir()})
			},
			run: func(e *Engine) error {
				_, err := e.Uninstall("claude", "soft")
				return err
			},
			wantCode:  "unsupported_uninstall_mode",
			wantStage: "validate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := tt.engine(t)
			err := tt.run(engine)
			if err == nil {
				t.Fatal("expected lifecycle error")
			}
			var lerr *LifecycleError
			if !errors.As(err, &lerr) {
				t.Fatalf("expected LifecycleError, got %v", err)
			}
			if lerr.Code != tt.wantCode || lerr.Stage != tt.wantStage || lerr.NextAction == "" {
				t.Fatalf("unexpected lifecycle envelope: %#v", lerr)
			}
		})
	}
}

func lastSnapshotID(t *testing.T, e *Engine) string {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(e.backups.backupDir(), "*.manifest.json"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("manifest lookup failed: err=%v entries=%d", err, len(entries))
	}
	name := filepath.Base(entries[0])
	return name[:len(name)-len(".manifest.json")]
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
