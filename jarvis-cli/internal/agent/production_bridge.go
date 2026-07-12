package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/reconcile"
)

// RenderedManagedOutput is a rendered Jarvis-owned file/config target. Location
// is relative to Root; it is never inferred from a user-owned path.
type RenderedManagedOutput struct {
	Identity string
	Location string
	Bytes    []byte
	Existing *reconcile.Artifact
}

// ProductionReconcileInput is the non-UI bridge between wizard selections,
// rendered managed outputs, and the injectable reconciliation composition.
type ProductionReconcileInput struct {
	SelectedAgents  []string
	Root            string
	EvidencePath    string
	RenderedOutputs []RenderedManagedOutput
	ClaudeMCPs      []NativeMCPDefinition
}

// FileCompensationStore writes only paths declared by rendered managed output.
// It is deliberately not a general purpose filesystem adapter.
type FileCompensationStore struct {
	root    string
	allowed map[string]struct{}
}

// NewFileCompensationStore creates a managed-only Store rooted in root.
func NewFileCompensationStore(root string, outputs []RenderedManagedOutput) (*FileCompensationStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("managed Store root is required")
	}
	store := &FileCompensationStore{root: filepath.Clean(root), allowed: make(map[string]struct{}, len(outputs))}
	if err := validateManagedRoot(store.root); err != nil {
		return nil, err
	}
	for _, output := range outputs {
		if strings.TrimSpace(output.Identity) == "" {
			return nil, errors.New("managed output identity is required")
		}
		location, err := safeManagedLocation(output.Location)
		if err != nil {
			return nil, err
		}
		if _, exists := store.allowed[location]; exists {
			return nil, fmt.Errorf("duplicate managed output location %q", location)
		}
		store.allowed[location] = struct{}{}
	}
	return store, nil
}

func (s *FileCompensationStore) Snapshot(location string) (reconcile.Snapshot, error) {
	path, err := s.pathFor(location)
	if err != nil {
		return reconcile.Snapshot{}, err
	}
	bytes, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return reconcile.Snapshot{}, nil
	}
	if err != nil {
		return reconcile.Snapshot{}, errors.New("managed Store snapshot failed; repair the managed artifact and rerun Install/Reconfigure")
	}
	return reconcile.Snapshot{Exists: true, Bytes: bytes}, nil
}

func (s *FileCompensationStore) Write(location string, content []byte, _ reconcile.Provenance) error {
	path, err := s.pathFor(location)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("managed Store directory is unavailable; repair it and rerun Install/Reconfigure")
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return errors.New("managed Store write failed; repair the managed artifact and rerun Install/Reconfigure")
	}
	return nil
}

func (s *FileCompensationStore) Delete(location string) error {
	path, err := s.pathFor(location)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("managed Store removal failed; repair the managed artifact and rerun Install/Reconfigure")
	}
	return nil
}

func (s *FileCompensationStore) pathFor(location string) (string, error) {
	if s == nil {
		return "", errors.New("managed Store adapter is unavailable")
	}
	location, err := safeManagedLocation(location)
	if err != nil {
		return "", err
	}
	if _, allowed := s.allowed[location]; !allowed {
		return "", errors.New("managed Store rejects a path outside rendered Jarvis artifacts")
	}
	path := filepath.Join(s.root, filepath.FromSlash(location))
	if err := rejectManagedPathSymlinks(s.root, path); err != nil {
		return "", err
	}
	return path, nil
}

func safeManagedLocation(location string) (string, error) {
	location = filepath.ToSlash(filepath.Clean(strings.TrimSpace(location)))
	if location == "." || filepath.IsAbs(location) || location == ".." || strings.HasPrefix(location, "../") {
		return "", errors.New("managed artifact path is unsafe")
	}
	return location, nil
}

func rejectManagedPathSymlinks(root, path string) error {
	if err := validateManagedRoot(root); err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("managed artifact path is unsafe")
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return errors.New("managed artifact path is unavailable; repair it and rerun Install/Reconfigure")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("managed artifact path contains a symlink")
		}
	}
	return nil
}

// validateManagedRoot rejects a root link before any allowlisted Store
// operation. It is intentionally repeated for each path operation because the
// filesystem can change after construction; full filesystem locking is outside
// this MVP boundary.
func validateManagedRoot(root string) error {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("managed Store root is unavailable; repair it and rerun Install/Reconfigure")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("managed Store root is unsafe")
	}
	if !info.IsDir() {
		return errors.New("managed Store root is unavailable; repair it and rerun Install/Reconfigure")
	}
	return nil
}

// BuildProductionReconcileRequest derives the Store plan solely from wizard
// selections and already-rendered managed outputs. OpenCode is represented only
// by its global JSON output; it never enters the Claude native manager.
func BuildProductionReconcileRequest(input ProductionReconcileInput) (ReconcileInstallRequest, error) {
	store, err := NewFileCompensationStore(input.Root, input.RenderedOutputs)
	if err != nil {
		return ReconcileInstallRequest{}, err
	}
	manifest := reconcile.Manifest{Version: "v1", Artifacts: make(map[string]reconcile.ManifestEntry, len(input.RenderedOutputs))}
	desired := reconcile.DesiredState{Manifest: manifest, Artifacts: make([]reconcile.DesiredArtifact, 0, len(input.RenderedOutputs))}
	inventory := reconcile.Inventory{}
	for _, output := range input.RenderedOutputs {
		location, err := safeManagedLocation(output.Location)
		if err != nil {
			return ReconcileInstallRequest{}, err
		}
		if _, exists := desired.Manifest.Artifacts[output.Identity]; exists {
			return ReconcileInstallRequest{}, fmt.Errorf("duplicate managed output identity %q", output.Identity)
		}
		digest := managedOutputDigest(output.Bytes)
		desired.Manifest.Artifacts[output.Identity] = reconcile.ManifestEntry{Location: location, Digest: digest}
		desired.Artifacts = append(desired.Artifacts, reconcile.DesiredArtifact{Identity: output.Identity, Location: location, Bytes: append([]byte(nil), output.Bytes...)})
		if output.Existing != nil {
			inventory.Artifacts = append(inventory.Artifacts, *output.Existing)
		}
	}

	request := ReconcileInstallRequest{Store: store, StorePlan: reconcile.BuildPlan(inventory, desired), EvidencePath: input.EvidencePath}
	if selectedAgent(input.SelectedAgents, "claude") {
		for _, definition := range input.ClaudeMCPs {
			if definition.Scope != nativeMCPUserScope {
				return ReconcileInstallRequest{}, errors.New("Claude native MCP definitions must use user scope")
			}
		}
		request.DesiredMCPs = append([]NativeMCPDefinition(nil), input.ClaudeMCPs...)
	}
	return request, nil
}

// ProductionExecutor is the shared, non-UI execution seam consumed by PR5B.
type productionReconciler func(ReconcileInstallRequest, NativeMCPReplacer) (ReconcileInstallResult, error)

type ProductionExecutor struct {
	Native    NativeMCPReplacer
	reconcile productionReconciler
}

// NewProductionExecutor wires the Claude-only native adapter. OpenCode remains
// represented by rendered global JSON Store output rather than this manager.
func NewProductionExecutor() ProductionExecutor {
	return ProductionExecutor{Native: NativeMCPManager{}, reconcile: ReconcileInstall}
}

func (e ProductionExecutor) Execute(input ProductionReconcileInput) (ReconcileInstallResult, error) {
	if strings.TrimSpace(input.EvidencePath) == "" {
		return ReconcileInstallResult{}, errors.New("recovery evidence path is required")
	}
	if err := os.MkdirAll(filepath.Dir(input.EvidencePath), 0o700); err != nil {
		return ReconcileInstallResult{}, errors.New("recovery evidence directory is unavailable; repair it and rerun Install/Reconfigure")
	}
	request, err := BuildProductionReconcileRequest(input)
	if err != nil {
		return ReconcileInstallResult{}, err
	}
	reconcileInstall := e.reconcile
	if reconcileInstall == nil {
		reconcileInstall = ReconcileInstall
	}
	result, err := reconcileInstall(request, e.Native)
	if err != nil {
		return result, errors.New("reconciliation failed; repair managed artifacts and rerun Install/Reconfigure")
	}
	return result, nil
}

func selectedAgent(selected []string, want string) bool {
	for _, name := range selected {
		if strings.EqualFold(strings.TrimSpace(name), want) {
			return true
		}
	}
	return false
}

func managedOutputDigest(content []byte) string {
	// BuildPlan validates this value against the rendered bytes before mutation.
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}
