package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	OpenCodeMCPs    OpenCodeManagedMCPs
	ClaudeMCPs      []NativeMCPDefinition
}

const (
	openCodeGlobalConfigIdentity = "opencode-global-config"
	openCodeGlobalConfigLocation = ".config/opencode/opencode.json"
)

// WizardReconcileInput is the UI-neutral, already-rendered input to managed
// reconciliation. Callers must supply rather than discover ownership evidence.
type WizardReconcileInput struct {
	SelectedAgents  []string
	Root            string
	EvidencePath    string
	RenderedOutputs []RenderedManagedOutput
	OpenCodeMCPs    OpenCodeManagedMCPs
	ClaudeHive      NativeMCPDefinition
	ClaudeContext7  NativeMCPDefinition
}

// FileCompensationStore writes only paths declared by rendered managed output.
// It is deliberately not a general purpose filesystem adapter.
type FileCompensationStore struct {
	root    string
	allowed map[string]string
}

const openCodeProvenanceManifestLocation = ".jarvis/metadata/reconcile/opencode-global-config.json"

var (
	readOpenCodePairFile   = readPairFile
	writeOpenCodePairFile  = writePrivateFile
	removeOpenCodePairFile = removePairFile
)

type openCodeProvenanceManifest struct {
	Version    string               `json:"version"`
	Identity   string               `json:"identity"`
	Location   string               `json:"location"`
	Digest     string               `json:"digest"`
	Provenance reconcile.Provenance `json:"provenance"`
}

// NewFileCompensationStore creates a managed-only Store rooted in root.
func NewFileCompensationStore(root string, outputs []RenderedManagedOutput) (*FileCompensationStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("managed Store root is required")
	}
	store := &FileCompensationStore{root: filepath.Clean(root), allowed: make(map[string]string, len(outputs))}
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
		if (output.Identity == openCodeGlobalConfigIdentity) != (location == openCodeGlobalConfigLocation) {
			return nil, errors.New("OpenCode managed artifact binding is invalid")
		}
		store.allowed[location] = output.Identity
	}
	return store, nil
}

func (s *FileCompensationStore) Snapshot(location string) (reconcile.Snapshot, error) {
	path, err := s.pathFor(location)
	if err != nil {
		return reconcile.Snapshot{}, err
	}
	if s.identityFor(location) == openCodeGlobalConfigIdentity {
		return s.openCodeSnapshot(path)
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

func (s *FileCompensationStore) openCodeSnapshot(path string) (reconcile.Snapshot, error) {
	manifestPath, err := s.openCodeManifestPath()
	if err != nil {
		return reconcile.Snapshot{}, err
	}
	content, artifactExists, err := readOpenCodePairFile(path)
	if err != nil {
		return reconcile.Snapshot{}, errors.New("OpenCode managed artifact is unavailable; repair it and rerun Install/Reconfigure")
	}
	manifest, manifestExists, err := readOpenCodePairFile(manifestPath)
	if err != nil {
		return reconcile.Snapshot{}, errors.New("OpenCode managed provenance is unavailable; repair it and rerun Install/Reconfigure")
	}
	if !artifactExists {
		if manifestExists {
			return reconcile.Snapshot{}, errors.New("OpenCode managed artifact/provenance pair is incomplete; repair it and rerun Install/Reconfigure")
		}
		return reconcile.Snapshot{}, nil
	}
	snapshot := reconcile.Snapshot{Exists: true, Bytes: content}
	provenance, err := openCodeProvenance(content, manifest, manifestExists)
	if err != nil {
		return reconcile.Snapshot{}, err
	}
	snapshot.Provenance = provenance
	return snapshot, nil
}

func (s *FileCompensationStore) Write(location string, content []byte, provenance reconcile.Provenance) error {
	path, err := s.pathFor(location)
	if err != nil {
		return err
	}
	if s.identityFor(location) == openCodeGlobalConfigIdentity {
		if err := validateOpenCodeProvenance(content, provenance); err != nil {
			return err
		}
		return s.writeOpenCodePair(path, content, provenance)
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
	if s.identityFor(location) == openCodeGlobalConfigIdentity {
		return s.deleteOpenCodePair(path)
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

func (s *FileCompensationStore) identityFor(location string) string {
	location, err := safeManagedLocation(location)
	if err != nil || s == nil {
		return ""
	}
	return s.allowed[location]
}

func (s *FileCompensationStore) openCodeManifestPath() (string, error) {
	path := filepath.Join(s.root, filepath.FromSlash(openCodeProvenanceManifestLocation))
	if err := rejectManagedPathSymlinks(s.root, path); err != nil {
		return "", err
	}
	return path, nil
}

func openCodeProvenance(content, manifestData []byte, manifestExists bool) (reconcile.Provenance, error) {
	if !manifestExists {
		if openCodeContainsManagedMCP(content) {
			return reconcile.Provenance{}, errors.New("OpenCode managed ownership is ambiguous; repair it and rerun Install/Reconfigure")
		}
		return reconcile.Provenance{}, nil
	}
	var manifest openCodeProvenanceManifest
	if json.Unmarshal(manifestData, &manifest) != nil || !validOpenCodeManifest(manifest, content) {
		return reconcile.Provenance{}, errors.New("OpenCode managed provenance does not match; repair it and rerun Install/Reconfigure")
	}
	return manifest.Provenance, nil
}

func openCodeContainsManagedMCP(content []byte) bool {
	var document map[string]json.RawMessage
	return json.Unmarshal(content, &document) == nil && hasJarvisManagedMCP(document)
}

func validateOpenCodeProvenance(content []byte, provenance reconcile.Provenance) error {
	if provenance.Version != "v1" || provenance.ManagedIdentity != openCodeGlobalConfigIdentity ||
		provenance.Location != openCodeGlobalConfigLocation || provenance.ManifestDigest != managedOutputDigest(content) {
		return errors.New("OpenCode managed provenance is invalid; repair it and rerun Install/Reconfigure")
	}
	return nil
}

func validOpenCodeManifest(manifest openCodeProvenanceManifest, content []byte) bool {
	return manifest.Version == "v1" && manifest.Identity == openCodeGlobalConfigIdentity &&
		manifest.Location == openCodeGlobalConfigLocation && manifest.Digest == managedOutputDigest(content) &&
		manifest.Provenance.Version == "v1" && manifest.Provenance.ManagedIdentity == manifest.Identity &&
		manifest.Provenance.Location == manifest.Location && manifest.Provenance.ManifestDigest == manifest.Digest
}

func (s *FileCompensationStore) writeOpenCodePair(path string, content []byte, provenance reconcile.Provenance) error {
	manifestPath, err := s.openCodeManifestPath()
	if err != nil {
		return err
	}
	manifest, err := json.Marshal(openCodeProvenanceManifest{Version: "v1", Identity: openCodeGlobalConfigIdentity, Location: openCodeGlobalConfigLocation, Digest: managedOutputDigest(content), Provenance: provenance})
	if err != nil {
		return errors.New("OpenCode managed provenance cannot be recorded; rerun Install/Reconfigure")
	}
	priorArtifact, artifactExists, err := readOpenCodePairFile(path)
	if err != nil {
		return openCodePairReadFailure("artifact")
	}
	priorManifest, manifestExists, err := readOpenCodePairFile(manifestPath)
	if err != nil {
		return openCodePairReadFailure("manifest")
	}
	if err := writeOpenCodePairFile(path, content); err != nil || writeOpenCodePairFile(manifestPath, manifest) != nil {
		if restoreErr := s.restoreOpenCodePair(path, priorArtifact, artifactExists, manifestPath, priorManifest, manifestExists); restoreErr != nil {
			return openCodePairRestoreFailure("write", restoreErr)
		}
		return errors.New("OpenCode managed Store write failed; repair it and rerun Install/Reconfigure")
	}
	return nil
}

func (s *FileCompensationStore) deleteOpenCodePair(path string) error {
	manifestPath, err := s.openCodeManifestPath()
	if err != nil {
		return err
	}
	priorArtifact, artifactExists, err := readOpenCodePairFile(path)
	if err != nil {
		return openCodePairReadFailure("artifact")
	}
	priorManifest, manifestExists, err := readOpenCodePairFile(manifestPath)
	if err != nil {
		return openCodePairReadFailure("manifest")
	}
	if err := removeOpenCodePairFile(path); err != nil || removeOpenCodePairFile(manifestPath) != nil {
		if restoreErr := s.restoreOpenCodePair(path, priorArtifact, artifactExists, manifestPath, priorManifest, manifestExists); restoreErr != nil {
			return openCodePairRestoreFailure("removal", restoreErr)
		}
		return errors.New("OpenCode managed Store removal failed; repair it and rerun Install/Reconfigure")
	}
	return nil
}

func (s *FileCompensationStore) restoreOpenCodePair(path string, artifact []byte, artifactExists bool, manifestPath string, manifest []byte, manifestExists bool) error {
	failed := make([]string, 0, 2)
	if artifactExists {
		if err := writeOpenCodePairFile(path, artifact); err != nil {
			failed = append(failed, "artifact")
		}
	} else {
		if err := removeOpenCodePairFile(path); err != nil {
			failed = append(failed, "artifact")
		}
	}
	if manifestExists {
		if err := writeOpenCodePairFile(manifestPath, manifest); err != nil {
			failed = append(failed, "manifest")
		}
	} else {
		if err := removeOpenCodePairFile(manifestPath); err != nil {
			failed = append(failed, "manifest")
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return errors.New(strings.Join(failed, " and "))
}

func readPairFile(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return nil, false, err
}

func openCodePairReadFailure(component string) error {
	return fmt.Errorf("OpenCode managed Store prior %s is unavailable; repair the OpenCode %s and rerun Install/Reconfigure", component, component)
}

func openCodePairRestoreFailure(operation string, restoreErr error) error {
	return fmt.Errorf("OpenCode managed Store %s failed; paired restoration is incomplete for %s; repair the OpenCode %s and rerun Install/Reconfigure", operation, restoreErr.Error(), restoreErr.Error())
}

func writePrivateFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".jarvis-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func removePairFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
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
		if output.Identity == openCodeGlobalConfigIdentity || location == openCodeGlobalConfigLocation {
			return ReconcileInstallRequest{}, errors.New("OpenCode must use the managed subdocument Store")
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

// BuildWizardReconcileRequest validates wizard-derived data before it reaches
// the executor. It does not read, merge, write, or infer configuration state.
func BuildWizardReconcileRequest(input WizardReconcileInput) (ReconcileInstallRequest, error) {
	if err := validateWizardInput(input); err != nil {
		return ReconcileInstallRequest{}, err
	}
	production := ProductionReconcileInput{
		SelectedAgents:  append([]string(nil), input.SelectedAgents...),
		Root:            input.Root,
		EvidencePath:    input.EvidencePath,
		RenderedOutputs: append([]RenderedManagedOutput(nil), input.RenderedOutputs...),
		OpenCodeMCPs:    input.OpenCodeMCPs,
	}
	if selectedAgent(input.SelectedAgents, "claude") {
		production.ClaudeMCPs = []NativeMCPDefinition{input.ClaudeHive, input.ClaudeContext7}
	}
	request, err := BuildProductionReconcileRequest(production)
	if err != nil {
		return ReconcileInstallRequest{}, err
	}
	if request.StorePlan.Blocked() {
		return ReconcileInstallRequest{}, errors.New("wizard rendered provenance or inventory does not authorize reconciliation")
	}
	return request, nil
}

func validateWizardInput(input WizardReconcileInput) error {
	if strings.TrimSpace(input.Root) == "" {
		return errors.New("wizard reconciliation root is required")
	}
	if err := validateWizardEvidencePath(input.Root, input.EvidencePath); err != nil {
		return err
	}
	selected := make(map[string]struct{}, len(input.SelectedAgents))
	for _, agentName := range input.SelectedAgents {
		name := strings.ToLower(strings.TrimSpace(agentName))
		if name != "claude" && name != "opencode" {
			return errors.New("wizard selected agent is unsupported")
		}
		if _, duplicate := selected[name]; duplicate {
			return errors.New("wizard selected agents are duplicated")
		}
		selected[name] = struct{}{}
	}

	_, selectedOpenCode := selected["opencode"]
	if selectedOpenCode != (len(input.OpenCodeMCPs) > 0) {
		return errors.New("wizard OpenCode desired state does not match selected agents")
	}
	if selectedOpenCode {
		if _, err := canonicalManagedMCPs(input.OpenCodeMCPs); err != nil {
			return err
		}
	}
	if _, selectedClaude := selected["claude"]; selectedClaude {
		if input.ClaudeHive.Identity != "hive" || input.ClaudeContext7.Identity != "context7" ||
			validateNativeMCPDefinition(input.ClaudeHive) != nil || validateNativeMCPMutationDefinition(input.ClaudeHive) != nil ||
			validateNativeMCPDefinition(input.ClaudeContext7) != nil || validateNativeMCPMutationDefinition(input.ClaudeContext7) != nil {
			return errors.New("wizard Claude MCP definitions are incomplete or invalid")
		}
	}
	return nil
}

func validateWizardEvidencePath(root, evidencePath string) error {
	if strings.TrimSpace(evidencePath) == "" {
		return errors.New("recovery evidence path is required")
	}
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(evidencePath))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("recovery evidence path must be inside the wizard reconciliation root")
	}
	return nil
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

// NewProductionExecutorWithNative preserves the production reconciliation
// composition while allowing callers to supply only the native command boundary.
// Production construction must continue to use NewProductionExecutor.
func NewProductionExecutorWithNative(native NativeMCPReplacer) ProductionExecutor {
	return ProductionExecutor{Native: native, reconcile: ReconcileInstall}
}

func (e ProductionExecutor) Execute(input ProductionReconcileInput) (ReconcileInstallResult, error) {
	request, err := BuildProductionReconcileRequest(input)
	if err != nil {
		return ReconcileInstallResult{}, err
	}
	if err := e.convergeOpenCode(input.Root, input.EvidencePath, input.OpenCodeMCPs); err != nil {
		return ReconcileInstallResult{}, err
	}
	return e.executeRequest(request)
}

// ExecuteWizard is the future TUI/no-TUI handoff. It validates all rendered
// inputs before creating recovery directories or invoking reconciliation.
func (e ProductionExecutor) ExecuteWizard(input WizardReconcileInput) (ReconcileInstallResult, error) {
	request, err := BuildWizardReconcileRequest(input)
	if err != nil {
		return ReconcileInstallResult{}, err
	}
	if err := e.convergeOpenCode(input.Root, input.EvidencePath, input.OpenCodeMCPs); err != nil {
		return ReconcileInstallResult{}, err
	}
	return e.executeRequest(request)
}

func (e ProductionExecutor) convergeOpenCode(root, evidencePath string, desired OpenCodeManagedMCPs) error {
	if len(desired) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(evidencePath), 0o700); err != nil {
		return errors.New("recovery evidence directory is unavailable; repair it and rerun Install/Reconfigure")
	}
	evidence, err := reconcile.NewFileRecoveryEvidenceStore(evidencePath)
	if err != nil {
		return errors.New("OpenCode recovery evidence adapter is unavailable; repair its location and rerun Install/Reconfigure")
	}
	store, err := NewOpenCodeManagedStore(osOpenCodeManagedFS{}, root, evidence)
	if err != nil {
		return err
	}
	return store.Converge(desired)
}

func (e ProductionExecutor) executeRequest(request ReconcileInstallRequest) (ReconcileInstallResult, error) {
	if err := os.MkdirAll(filepath.Dir(request.EvidencePath), 0o700); err != nil {
		return ReconcileInstallResult{}, errors.New("recovery evidence directory is unavailable; repair it and rerun Install/Reconfigure")
	}
	reconcileInstall := e.reconcile
	if reconcileInstall == nil {
		reconcileInstall = ReconcileInstall
	}
	native := e.Native
	if len(request.DesiredMCPs) == 0 {
		native = nil
	}
	result, err := reconcileInstall(request, native)
	if err != nil {
		if result.Native.ErrorCategory != "" {
			return result, fmt.Errorf("native MCP reconciliation failed (%s): %s", result.Native.Diagnostics(), nativeMCPFixForwardGuidance)
		}
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
