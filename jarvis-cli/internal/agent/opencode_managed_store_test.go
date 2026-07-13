package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"
)

func TestOpenCodeManagedStoreMigratesLegacyStateWithoutReplacingUserValues(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, filepath.FromSlash(openCodeGlobalConfigLocation))
	sidecarPath := filepath.Join(root, filepath.FromSlash(openCodeProvenanceManifestLocation))
	writeTestFile(t, configPath, []byte(`{"theme":"night","mcp":{"hive":{"old":true},"user":{"url":"keep"}}}`))
	writeTestFile(t, sidecarPath, []byte(`{"version":"v1","stale":true}`))

	store := newTestOpenCodeManagedStore(t, root, osOpenCodeManagedFS{}, nil)
	desired := OpenCodeManagedMCPs{
		"hive":     `{"type":"local","command":["hive"]}`,
		"context7": `{"type":"remote","url":"https://example.invalid"}`,
	}
	if err := store.Converge(desired); err != nil {
		t.Fatalf("Converge() error = %v", err)
	}

	doc := readJSONDocument(t, configPath)
	if string(doc["theme"]) != `"night"` {
		t.Fatalf("theme = %s, want preserved", doc["theme"])
	}
	var mcp map[string]json.RawMessage
	if err := json.Unmarshal(doc["mcp"], &mcp); err != nil {
		t.Fatalf("decode mcp: %v", err)
	}
	if string(mcp["user"]) != `{"url":"keep"}` || !jsonEqual(mcp["hive"], []byte(desired["hive"])) || !jsonEqual(mcp["context7"], []byte(desired["context7"])) {
		t.Fatalf("mcp = %s, want managed replacement and user preservation", doc["mcp"])
	}
	assertV2Sidecar(t, sidecarPath, desired)
}

func TestOpenCodeManagedStoreNoOpsMatchingConfigButUpgradesSidecar(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, filepath.FromSlash(openCodeGlobalConfigLocation))
	desired := OpenCodeManagedMCPs{"hive": `{"type":"local"}`}
	original := []byte("{\n  \"mcp\": {\"hive\": {\"type\": \"local\"}},\n  \"theme\": \"keep-format\"\n}\n")
	writeTestFile(t, configPath, original)
	fsys := &recordingOpenCodeFS{delegate: osOpenCodeManagedFS{}}
	store := newTestOpenCodeManagedStore(t, root, fsys, nil)

	if err := store.Converge(desired); err != nil {
		t.Fatalf("Converge() error = %v", err)
	}
	if fsys.writes[configPath] != 0 {
		t.Fatalf("config writes = %d, want no-op", fsys.writes[configPath])
	}
	got, err := os.ReadFile(configPath)
	if err != nil || !reflect.DeepEqual(got, original) {
		t.Fatalf("config = %q, %v; want exact original", got, err)
	}
	assertV2Sidecar(t, filepath.Join(root, filepath.FromSlash(openCodeProvenanceManifestLocation)), desired)
}

func TestOpenCodeManagedStoreAcceptsEveryLegacySidecarInput(t *testing.T) {
	legacy := [][]byte{
		nil,
		[]byte(`{"version":"v1","identity":"opencode-global-config"}`),
		[]byte(`{"version":"v1","digest":"stale"}`),
		[]byte(`{malformed`),
	}
	for i, sidecar := range legacy {
		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, filepath.FromSlash(openCodeGlobalConfigLocation)), []byte(`{"mcp":{"hive":{"old":true}}}`))
		if sidecar != nil {
			writeTestFile(t, filepath.Join(root, filepath.FromSlash(openCodeProvenanceManifestLocation)), sidecar)
		}
		store := newTestOpenCodeManagedStore(t, root, osOpenCodeManagedFS{}, nil)
		desired := OpenCodeManagedMCPs{"hive": `{"current":true}`}
		if err := store.Converge(desired); err != nil {
			t.Fatalf("legacy case %d Converge() error = %v", i, err)
		}
		assertV2Sidecar(t, filepath.Join(root, filepath.FromSlash(openCodeProvenanceManifestLocation)), desired)
	}
}

func TestOpenCodeManagedStoreRejectsInvalidInputsBeforeMutation(t *testing.T) {
	tests := []struct {
		name    string
		config  []byte
		desired OpenCodeManagedMCPs
	}{
		{name: "malformed root", config: []byte(`[`), desired: OpenCodeManagedMCPs{"hive": `{}`}},
		{name: "non-object mcp", config: []byte(`{"mcp":[]}`), desired: OpenCodeManagedMCPs{"hive": `{}`}},
		{name: "foreign desired name", config: []byte(`{}`), desired: OpenCodeManagedMCPs{"user": `{}`}},
		{name: "invalid desired value", config: []byte(`{}`), desired: OpenCodeManagedMCPs{"hive": `[]`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			configPath := filepath.Join(root, filepath.FromSlash(openCodeGlobalConfigLocation))
			writeTestFile(t, configPath, tt.config)
			fsys := &recordingOpenCodeFS{delegate: osOpenCodeManagedFS{}}
			store := newTestOpenCodeManagedStore(t, root, fsys, nil)
			if err := store.Converge(tt.desired); err == nil {
				t.Fatal("Converge() error = nil, want rejection")
			}
			if len(fsys.writes) != 0 {
				t.Fatalf("writes = %#v, want none", fsys.writes)
			}
		})
	}
}

func TestOpenCodeManagedStoreCompensatesOnlyManagedPathsAndPriorSidecar(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, filepath.FromSlash(openCodeGlobalConfigLocation))
	sidecarPath := filepath.Join(root, filepath.FromSlash(openCodeProvenanceManifestLocation))
	priorSidecar := []byte(`{"version":"v1","exact":"bytes"}`)
	writeTestFile(t, configPath, []byte(`{"theme":"before","mcp":{"hive":{"old":true},"user":{"value":1}}}`))
	writeTestFile(t, sidecarPath, priorSidecar)
	fsys := &recordingOpenCodeFS{delegate: osOpenCodeManagedFS{}, failWritePath: sidecarPath, beforeFailure: func() {
		writeTestFile(t, configPath, []byte(`{"theme":"concurrent","mcp":{"hive":{"new":true},"user":{"value":2}}}`))
	}}
	store := newTestOpenCodeManagedStore(t, root, fsys, nil)

	if err := store.Converge(OpenCodeManagedMCPs{"hive": `{"new":true}`}); err == nil {
		t.Fatal("Converge() error = nil, want sidecar failure")
	}
	doc := readJSONDocument(t, configPath)
	if string(doc["theme"]) != `"concurrent"` {
		t.Fatalf("theme = %s, want concurrent value", doc["theme"])
	}
	var mcp map[string]json.RawMessage
	_ = json.Unmarshal(doc["mcp"], &mcp)
	if string(mcp["user"]) != `{"value":2}` || string(mcp["hive"]) != `{"old":true}` {
		t.Fatalf("mcp = %s, want scoped restoration", doc["mcp"])
	}
	gotSidecar, _ := os.ReadFile(sidecarPath)
	if !reflect.DeepEqual(gotSidecar, priorSidecar) {
		t.Fatalf("sidecar = %q, want exact prior bytes", gotSidecar)
	}
}

func TestOpenCodeManagedStorePersistsSanitizedEvidenceWhenCompensationFails(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, filepath.FromSlash(openCodeGlobalConfigLocation))
	sidecarPath := filepath.Join(root, filepath.FromSlash(openCodeProvenanceManifestLocation))
	writeTestFile(t, configPath, []byte(`{"mcp":{"hive":{"token":"secret"}}}`))
	evidence := &capturingEvidenceStore{}
	fsys := &recordingOpenCodeFS{delegate: osOpenCodeManagedFS{}, failWritePath: sidecarPath, failAfter: 0, failConfigAfter: 1}
	store := newTestOpenCodeManagedStore(t, root, fsys, evidence)

	if err := store.Converge(OpenCodeManagedMCPs{"hive": `{"token":"replacement"}`}); err == nil {
		t.Fatal("Converge() error = nil, want degraded failure")
	}
	if len(evidence.items) != 1 || evidence.items[0].FailedTarget != "opencode-managed-sidecar" || len(evidence.items[0].CompensationFailures) == 0 {
		t.Fatalf("evidence = %#v, want sanitized degraded record", evidence.items)
	}
	if !reflect.DeepEqual(evidence.items[0].AffectedTargets, []string{"opencode-managed-config", "opencode-managed-sidecar"}) || fsys.writes[sidecarPath] == 0 {
		t.Fatalf("evidence = %#v, sidecar writes = %d; want attempted sidecar tracked", evidence.items[0], fsys.writes[sidecarPath])
	}
}

func TestOpenCodeManagedStoreLeavesSidecarUntouchedBeforeSidecarMutation(t *testing.T) {
	tests := []struct {
		name          string
		corruptConfig bool
	}{
		{name: "config write failure"},
		{name: "config verification failure", corruptConfig: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			configPath := filepath.Join(root, filepath.FromSlash(openCodeGlobalConfigLocation))
			sidecarPath := filepath.Join(root, filepath.FromSlash(openCodeProvenanceManifestLocation))
			priorSidecar := []byte(`{malformed legacy bytes`)
			writeTestFile(t, configPath, []byte(`{"mcp":{"hive":{"old":true}}}`))
			writeTestFile(t, sidecarPath, priorSidecar)
			fsys := &recordingOpenCodeFS{delegate: osOpenCodeManagedFS{}, failWritePath: configPath, corruptWritePath: map[bool]string{true: configPath}[tt.corruptConfig]}
			if tt.corruptConfig {
				fsys.failWritePath = ""
			}
			store := newTestOpenCodeManagedStore(t, root, fsys, nil)

			if err := store.Converge(OpenCodeManagedMCPs{"hive": `{"new":true}`}); err == nil {
				t.Fatal("Converge() error = nil, want config failure")
			}
			got, err := os.ReadFile(sidecarPath)
			if err != nil || !bytes.Equal(got, priorSidecar) || fsys.writes[sidecarPath] != 0 || fsys.removes[sidecarPath] != 0 {
				t.Fatalf("sidecar = %q, err %v, writes %d, removes %d; want byte-identical and untouched", got, err, fsys.writes[sidecarPath], fsys.removes[sidecarPath])
			}
			if tt.corruptConfig && !reflect.DeepEqual(store.lastRecovery.AffectedTargets, []string{"opencode-managed-config"}) {
				t.Fatalf("affected targets = %#v, want config only before sidecar attempt", store.lastRecovery.AffectedTargets)
			}
		})
	}
}

func TestOpenCodeManagedStoreReportsUnavailableDegradedEvidencePersistence(t *testing.T) {
	tests := []struct {
		name     string
		evidence reconcile.RecoveryEvidenceStore
	}{
		{name: "nil store"},
		{name: "store failure", evidence: &capturingEvidenceStore{err: errors.New("secret persistence detail")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			configPath := filepath.Join(root, filepath.FromSlash(openCodeGlobalConfigLocation))
			sidecarPath := filepath.Join(root, filepath.FromSlash(openCodeProvenanceManifestLocation))
			writeTestFile(t, configPath, []byte(`{"mcp":{"hive":{"old":true}}}`))
			fsys := &recordingOpenCodeFS{delegate: osOpenCodeManagedFS{}, failWritePath: sidecarPath, failConfigAfter: 1}
			store := newTestOpenCodeManagedStore(t, root, fsys, tt.evidence)

			err := store.Converge(OpenCodeManagedMCPs{"hive": `{"new":true}`})
			if err == nil || !strings.Contains(err.Error(), "recovery evidence persistence failed") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("Converge() error = %v, want explicit sanitized persistence failure", err)
			}
			if store.lastRecovery.CompensationFailures == nil || store.lastRecovery.FailedTarget != "opencode-managed-sidecar" {
				t.Fatalf("in-memory recovery = %#v, want retained degraded evidence", store.lastRecovery)
			}
		})
	}
}

type recordingOpenCodeFS struct {
	delegate         OpenCodeManagedFS
	writes           map[string]int
	failWritePath    string
	failAfter        int
	failConfigAfter  int
	corruptWritePath string
	removes          map[string]int
	beforeFailure    func()
}

func (f *recordingOpenCodeFS) ReadFile(path string) ([]byte, error) { return f.delegate.ReadFile(path) }
func (f *recordingOpenCodeFS) Remove(path string) error {
	if f.removes == nil {
		f.removes = map[string]int{}
	}
	f.removes[path]++
	return f.delegate.Remove(path)
}
func (f *recordingOpenCodeFS) AtomicWrite(path string, data []byte) error {
	if f.writes == nil {
		f.writes = map[string]int{}
	}
	f.writes[path]++
	shouldFail := path == f.failWritePath && f.writes[path] == f.failAfter+1
	if filepath.Base(path) == "opencode.json" && f.failConfigAfter > 0 && f.writes[path] > f.failConfigAfter {
		shouldFail = true
	}
	if shouldFail {
		if f.beforeFailure != nil {
			f.beforeFailure()
			f.beforeFailure = nil
		}
		return errors.New("injected secret failure")
	}
	if err := f.delegate.AtomicWrite(path, data); err != nil {
		return err
	}
	if path == f.corruptWritePath {
		return f.delegate.AtomicWrite(path, []byte(`{"corrupt":true}`))
	}
	return nil
}

type capturingEvidenceStore struct {
	items []reconcile.RecoveryEvidence
	err   error
}

func (s *capturingEvidenceStore) PersistDegradedRecovery(e reconcile.RecoveryEvidence) error {
	s.items = append(s.items, e)
	return s.err
}

func newTestOpenCodeManagedStore(t *testing.T, root string, fsys OpenCodeManagedFS, evidence reconcile.RecoveryEvidenceStore) *OpenCodeManagedStore {
	t.Helper()
	store, err := NewOpenCodeManagedStore(fsys, root, evidence)
	if err != nil {
		t.Fatalf("NewOpenCodeManagedStore() error = %v", err)
	}
	return store
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func readJSONDocument(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return doc
}

func assertV2Sidecar(t *testing.T, path string, desired OpenCodeManagedMCPs) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(sidecar) error = %v", err)
	}
	var got openCodeManagedSidecarV2
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal(sidecar) error = %v", err)
	}
	if got.Version != "v2" || !reflect.DeepEqual(got.Paths, managedOpenCodePaths(desired)) || got.Digest != managedOpenCodeDigest(desired) {
		t.Fatalf("sidecar = %#v, want v2 exact managed binding", got)
	}
}
