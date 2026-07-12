package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"
)

func TestReconcileInstallPersistsAndReloadsDegradedStoreEvidence(t *testing.T) {
	store := &compositionStore{failWrite: true, failRestore: true}
	evidencePath := filepath.Join(t.TempDir(), "recovery.json")

	_, err := ReconcileInstall(ReconcileInstallRequest{
		Store:        store,
		StorePlan:    compositionPlan(),
		EvidencePath: evidencePath,
	}, nil)

	if err == nil {
		t.Fatal("ReconcileInstall() error = nil, want degraded Store failure")
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("ReconcileInstall() error leaked secret: %v", err)
	}
	reloaded, loadErr := reconcile.NewFileRecoveryEvidenceStore(evidencePath)
	if loadErr != nil {
		t.Fatalf("NewFileRecoveryEvidenceStore() error = %v", loadErr)
	}
	evidence, loadErr := reloaded.LoadDegradedRecovery()
	if loadErr != nil {
		t.Fatalf("LoadDegradedRecovery() error = %v", loadErr)
	}
	if evidence.FailedTarget != "jarvis/store.json" || len(evidence.CompensationFailures) != 1 || evidence.RecoveryAction == "" {
		t.Fatalf("reloaded evidence = %#v, want sanitized degraded recovery evidence", evidence)
	}
}

func TestReconcileInstallSkipsNativeMCPWhenNoDesiredEntriesAndAppliesStore(t *testing.T) {
	store := &compositionStore{}
	native := &compositionNativeMCP{}

	result, err := ReconcileInstall(ReconcileInstallRequest{
		Store:        store,
		StorePlan:    compositionPlan(),
		EvidencePath: filepath.Join(t.TempDir(), "recovery.json"),
	}, native)

	if err != nil {
		t.Fatalf("ReconcileInstall() error = %v", err)
	}
	if result.Native.Phase != NativeMCPSkipped || native.calls != 0 {
		t.Fatalf("native result/calls = (%#v, %d), want deterministic skip without native work", result.Native, native.calls)
	}
	if store.writes != 1 {
		t.Fatalf("Store writes = %d, want Jarvis/Hive Store reconciliation to continue", store.writes)
	}
}

func TestReconcileInstallFailsStopWhenNativeAdapterFails(t *testing.T) {
	native := &compositionNativeMCP{err: errors.New("native output token=super-secret")}

	_, err := ReconcileInstall(ReconcileInstallRequest{
		Store:        &compositionStore{},
		EvidencePath: filepath.Join(t.TempDir(), "recovery.json"),
		DesiredMCPs:  []NativeMCPDefinition{nativeMCPDefinition("jarvis-hive", "desired-secret")},
	}, native)

	if err == nil || !strings.Contains(err.Error(), "correct the native MCP error and rerun Install/Reconfigure") {
		t.Fatalf("ReconcileInstall() error = %v, want actionable native fail-stop", err)
	}
	if strings.Contains(err.Error(), "super-secret") || strings.Contains(err.Error(), "desired-secret") {
		t.Fatalf("ReconcileInstall() error leaked secret: %v", err)
	}
}

type compositionNativeMCP struct {
	calls int
	err   error
}

func (n *compositionNativeMCP) Replace([]NativeMCPDefinition) (*NativeMCPResult, error) {
	n.calls++
	return &NativeMCPResult{Phase: NativeMCPAdded, Guidance: nativeMCPFixForwardGuidance}, n.err
}

type compositionStore struct {
	writes      int
	failWrite   bool
	failRestore bool
}

func (s *compositionStore) Snapshot(string) (reconcile.Snapshot, error) {
	return reconcile.Snapshot{Exists: true, Bytes: []byte("prior")}, nil
}

func (s *compositionStore) Write(string, []byte, reconcile.Provenance) error {
	s.writes++
	if s.failWrite && s.writes == 1 {
		return errors.New("write token=super-secret")
	}
	if s.failRestore && s.writes > 1 {
		return errors.New("restore token=super-secret")
	}
	return nil
}

func (s *compositionStore) Delete(string) error { return nil }

func compositionPlan() reconcile.Plan {
	content := []byte("managed")
	return reconcile.BuildPlan(reconcile.Inventory{}, reconcile.DesiredState{
		Manifest: reconcile.Manifest{Version: "v1", Artifacts: map[string]reconcile.ManifestEntry{
			"jarvis-store": {Location: "jarvis/store.json", Digest: compositionDigest(content)},
		}},
		Artifacts: []reconcile.DesiredArtifact{{Identity: "jarvis-store", Location: "jarvis/store.json", Bytes: content}},
	})
}

func compositionDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}
