package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"
)

// OpenCodeManagedFS isolates all filesystem and atomic-publication boundaries.
type OpenCodeManagedFS interface {
	ReadFile(string) ([]byte, error)
	AtomicWrite(string, []byte) error
	Remove(string) error
}

type osOpenCodeManagedFS struct{}

func (osOpenCodeManagedFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (osOpenCodeManagedFS) AtomicWrite(path string, data []byte) error {
	return writePrivateFile(path, data)
}
func (osOpenCodeManagedFS) Remove(path string) error { return removePairFile(path) }

// OpenCodeManagedStore owns only /mcp/hive, /mcp/context7 and their v2 sidecar.
// It remains intentionally unwired while the whole-file Store is in production.
type OpenCodeManagedStore struct {
	fs            OpenCodeManagedFS
	configPath    string
	sidecarPath   string
	evidenceStore reconcile.RecoveryEvidenceStore
	lastRecovery  reconcile.RecoveryEvidence
}

type openCodeManagedSidecarV2 struct {
	Version string   `json:"version"`
	Digest  string   `json:"digest"`
	Paths   []string `json:"paths"`
}

type openCodeManagedSnapshot struct {
	managed       map[string]json.RawMessage
	sidecar       []byte
	sidecarExists bool
}

// NewOpenCodeManagedStore creates the dormant production adapter beneath an
// injected home root. Construction validates paths but performs no mutation.
func NewOpenCodeManagedStore(fsys OpenCodeManagedFS, root string, evidenceStore reconcile.RecoveryEvidenceStore) (*OpenCodeManagedStore, error) {
	if fsys == nil {
		return nil, errors.New("OpenCode managed Store filesystem is required")
	}
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("OpenCode managed Store root is required")
	}
	root = filepath.Clean(root)
	if err := validateManagedRoot(root); err != nil {
		return nil, err
	}
	configPath := filepath.Join(root, filepath.FromSlash(openCodeGlobalConfigLocation))
	sidecarPath := filepath.Join(root, filepath.FromSlash(openCodeProvenanceManifestLocation))
	if err := rejectManagedPathSymlinks(root, configPath); err != nil {
		return nil, err
	}
	if err := rejectManagedPathSymlinks(root, sidecarPath); err != nil {
		return nil, err
	}
	return &OpenCodeManagedStore{fs: fsys, configPath: configPath, sidecarPath: sidecarPath, evidenceStore: evidenceStore}, nil
}

// Converge migrates legacy states and converges only Jarvis' managed MCP names.
func (s *OpenCodeManagedStore) Converge(desired OpenCodeManagedMCPs) error {
	canonical, err := canonicalManagedMCPs(desired)
	if err != nil {
		return err
	}
	doc, err := s.readDocument()
	if err != nil {
		return err
	}
	snapshot, err := s.snapshot(doc)
	if err != nil {
		return err
	}
	configMatches := managedMCPsEqual(doc, canonical)
	if !configMatches {
		latest, readErr := s.readDocument()
		if readErr != nil {
			return readErr
		}
		mergeManagedMCPs(latest, canonical)
		if err := s.writeAndVerifyDocument(latest); err != nil {
			return s.failAndCompensate(snapshot, "opencode-managed-config", true, false)
		}
	}

	sidecar, err := json.Marshal(openCodeManagedSidecarV2{
		Version: "v2", Digest: managedOpenCodeDigest(canonical), Paths: managedOpenCodePaths(canonical),
	})
	if err != nil {
		return s.failAndCompensate(snapshot, "opencode-managed-sidecar", !configMatches, false)
	}
	prior, exists, err := s.readOptional(s.sidecarPath)
	if err == nil && exists && bytes.Equal(prior, sidecar) {
		return nil
	}
	if err != nil {
		return s.failAndCompensate(snapshot, "opencode-managed-sidecar", !configMatches, false)
	}
	if s.fs.AtomicWrite(s.sidecarPath, sidecar) != nil || s.verifyExact(s.sidecarPath, sidecar) != nil {
		return s.failAndCompensate(snapshot, "opencode-managed-sidecar", !configMatches, true)
	}
	return nil
}

func (s *OpenCodeManagedStore) readDocument() (map[string]json.RawMessage, error) {
	data, exists, err := s.readOptional(s.configPath)
	if err != nil {
		return nil, errors.New("OpenCode configuration is unavailable; repair it and rerun Install/Reconfigure")
	}
	if !exists {
		return map[string]json.RawMessage{}, nil
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(data, &doc) != nil || doc == nil {
		return nil, errors.New("OpenCode global configuration is malformed; repair it and rerun Install/Reconfigure")
	}
	if raw, found := doc["mcp"]; found {
		var mcp map[string]json.RawMessage
		if json.Unmarshal(raw, &mcp) != nil || mcp == nil {
			return nil, errors.New("OpenCode mcp configuration must be an object; repair it and rerun Install/Reconfigure")
		}
	}
	return doc, nil
}

func (s *OpenCodeManagedStore) snapshot(doc map[string]json.RawMessage) (openCodeManagedSnapshot, error) {
	snapshot := openCodeManagedSnapshot{managed: currentManagedMCPs(doc)}
	var err error
	snapshot.sidecar, snapshot.sidecarExists, err = s.readOptional(s.sidecarPath)
	if err != nil {
		return openCodeManagedSnapshot{}, errors.New("OpenCode managed sidecar is unavailable; repair it and rerun Install/Reconfigure")
	}
	return snapshot, nil
}

func (s *OpenCodeManagedStore) failAndCompensate(snapshot openCodeManagedSnapshot, failedTarget string, restoreConfig, restoreSidecar bool) error {
	failures := make([]string, 0, 2)
	affected := make([]string, 0, 2)
	if restoreConfig {
		affected = append(affected, "opencode-managed-config")
		latest, err := s.readDocument()
		if err != nil {
			failures = append(failures, "opencode-managed-config")
		} else {
			mergeManagedMCPs(latest, snapshot.managed)
			if err := s.writeAndVerifyDocument(latest); err != nil {
				failures = append(failures, "opencode-managed-config")
			}
		}
	}
	if restoreSidecar && snapshot.sidecarExists {
		affected = append(affected, "opencode-managed-sidecar")
		if s.fs.AtomicWrite(s.sidecarPath, snapshot.sidecar) != nil || s.verifyExact(s.sidecarPath, snapshot.sidecar) != nil {
			failures = append(failures, "opencode-managed-sidecar")
		}
	} else if restoreSidecar {
		affected = append(affected, "opencode-managed-sidecar")
		if err := s.fs.Remove(s.sidecarPath); err != nil {
			failures = append(failures, "opencode-managed-sidecar")
		}
	}
	if len(failures) > 0 {
		evidence := reconcile.RecoveryEvidence{
			FailedTarget: failedTarget, AffectedTargets: affected,
			CompensationFailures: failures, RecoveryAction: "repair managed OpenCode state and rerun Install/Reconfigure",
		}
		s.lastRecovery = evidence
		if s.evidenceStore == nil || s.evidenceStore.PersistDegradedRecovery(evidence) != nil {
			return errors.New("OpenCode managed Store reached degraded partial state; recovery evidence persistence failed; repair managed OpenCode state and rerun Install/Reconfigure")
		}
		return errors.New("OpenCode managed Store reached degraded partial state; repair managed OpenCode state and rerun Install/Reconfigure")
	}
	return errors.New("OpenCode managed Store transition failed and was compensated; rerun Install/Reconfigure")
}

func (s *OpenCodeManagedStore) writeAndVerifyDocument(doc map[string]json.RawMessage) error {
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if err := s.fs.AtomicWrite(s.configPath, data); err != nil {
		return err
	}
	return s.verifyExact(s.configPath, data)
}

func (s *OpenCodeManagedStore) verifyExact(path string, want []byte) error {
	got, err := s.fs.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		return errors.New("atomic write verification failed")
	}
	return nil
}

func (s *OpenCodeManagedStore) readOptional(path string) ([]byte, bool, error) {
	data, err := s.fs.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func canonicalManagedMCPs(desired OpenCodeManagedMCPs) (OpenCodeManagedMCPs, error) {
	if err := validateOpenCodeManagedNames(desired); err != nil {
		return nil, err
	}
	canonical := make(OpenCodeManagedMCPs, len(desired))
	for name, value := range desired {
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(value), &object) != nil || object == nil {
			return nil, errors.New("OpenCode desired managed MCP must be a JSON object")
		}
		data, err := json.Marshal(object)
		if err != nil {
			return nil, errors.New("OpenCode desired managed MCP is invalid")
		}
		canonical[name] = string(data)
	}
	return canonical, nil
}

func currentManagedMCPs(doc map[string]json.RawMessage) map[string]json.RawMessage {
	managed := map[string]json.RawMessage{}
	var mcp map[string]json.RawMessage
	_ = json.Unmarshal(doc["mcp"], &mcp)
	for _, name := range []string{"hive", "context7"} {
		if value, found := mcp[name]; found {
			managed[name] = append(json.RawMessage(nil), value...)
		}
	}
	return managed
}

func mergeManagedMCPs(doc map[string]json.RawMessage, managed any) {
	var mcp map[string]json.RawMessage
	_ = json.Unmarshal(doc["mcp"], &mcp)
	if mcp == nil {
		mcp = map[string]json.RawMessage{}
	}
	delete(mcp, "hive")
	delete(mcp, "context7")
	switch values := managed.(type) {
	case OpenCodeManagedMCPs:
		for name, value := range values {
			mcp[name] = json.RawMessage(value)
		}
	case map[string]json.RawMessage:
		for name, value := range values {
			mcp[name] = value
		}
	}
	data, _ := json.Marshal(mcp)
	doc["mcp"] = data
}

func managedMCPsEqual(doc map[string]json.RawMessage, desired OpenCodeManagedMCPs) bool {
	current := currentManagedMCPs(doc)
	if len(current) != len(desired) {
		return false
	}
	for name, value := range desired {
		if !jsonEqual(current[name], []byte(value)) {
			return false
		}
	}
	return true
}

func managedOpenCodePaths(managed OpenCodeManagedMCPs) []string {
	paths := make([]string, 0, len(managed))
	for name := range managed {
		paths = append(paths, "/mcp/"+name)
	}
	sort.Strings(paths)
	return paths
}

func managedOpenCodeDigest(managed OpenCodeManagedMCPs) string {
	canonical, _ := canonicalManagedMCPs(managed)
	data, _ := json.Marshal(canonical)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
