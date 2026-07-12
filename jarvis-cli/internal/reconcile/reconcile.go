// Package reconcile plans and applies ownership-safe Jarvis configuration changes.
package reconcile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Ownership is the result of evidence-based artifact classification.
type Ownership string

const (
	OwnershipProvenJarvis    Ownership = "proven-jarvis"
	OwnershipForeign         Ownership = "foreign"
	OwnershipAmbiguousLegacy Ownership = "ambiguous-legacy"
)

// Provenance is the versioned marker stored with a Jarvis-managed artifact.
// All fields must match the durable manifest before mutation is permitted.
type Provenance struct {
	Version         string `json:"version"`
	ManagedIdentity string `json:"managed_identity"`
	Location        string `json:"location"`
	ManifestDigest  string `json:"manifest_digest"`
}

// Manifest records the expected durable ownership binding for each artifact.
type Manifest struct {
	Version   string                   `json:"version"`
	Artifacts map[string]ManifestEntry `json:"artifacts"`
}

// ManifestEntry identifies an artifact without treating its path as ownership proof.
type ManifestEntry struct {
	Location string `json:"location"`
	Digest   string `json:"digest"`
}

// Artifact is an adapter-observed artifact. References are observation-only and
// are deliberately not treated as provenance evidence.
type Artifact struct {
	Identity   string
	Location   string
	Bytes      []byte
	References []string
	Provenance *Provenance
}

// Inventory is the immutable observation passed to the planner.
type Inventory struct{ Artifacts []Artifact }

// DesiredArtifact is a fully rendered managed target.
type DesiredArtifact struct {
	Identity string
	Location string
	Bytes    []byte
}

// DesiredState combines the versioned manifest and rendered target bytes.
type DesiredState struct {
	Manifest  Manifest
	Artifacts []DesiredArtifact
}

// OperationKind describes a safe mutation.
type OperationKind string

const OperationWrite OperationKind = "write"

// Operation contains the minimum data required to apply a managed change.
// content stays private so journals and reports cannot leak configuration data.
type Operation struct {
	Kind       OperationKind
	Identity   string
	Location   string
	Digest     string
	Provenance Provenance
	content    []byte
}

// Blocker records a non-destructive collision and deterministic user guidance.
type Blocker struct {
	Identity        string
	Location        string
	Ownership       Ownership
	RecoveryCommand string
}

// Plan is either safe to apply or blocked without any partial mutation.
type Plan struct {
	Operations []Operation
	Blockers   []Blocker
}

func (p Plan) Blocked() bool { return len(p.Blockers) > 0 }

// Classify proves ownership only when a versioned marker binds the artifact to
// the current manifest. Names, paths, JSON keys, values, and references are not proof.
func Classify(artifact Artifact, manifest Manifest) Ownership {
	entry, reserved := manifest.Artifacts[artifact.Identity]
	marker := artifact.Provenance
	if marker != nil && reserved && marker.Version == manifest.Version &&
		marker.ManagedIdentity == artifact.Identity && marker.Location == artifact.Location &&
		artifact.Location == entry.Location && marker.Location == entry.Location &&
		marker.ManifestDigest == entry.Digest && digest(artifact.Bytes) == entry.Digest {
		return OwnershipProvenJarvis
	}
	if reserved {
		return OwnershipAmbiguousLegacy
	}
	return OwnershipForeign
}

// BuildPlan creates mutations only for empty targets or proven Jarvis artifacts.
func BuildPlan(inventory Inventory, desired DesiredState) Plan {
	if blocker, duplicate := duplicateDesiredLocationBlocker(desired.Artifacts); duplicate {
		return Plan{Blockers: []Blocker{blocker}}
	}

	byLocation := make(map[string]Artifact, len(inventory.Artifacts))
	duplicateLocations := make(map[string]bool)
	for _, artifact := range inventory.Artifacts {
		if _, exists := byLocation[artifact.Location]; exists {
			duplicateLocations[artifact.Location] = true
		}
		byLocation[artifact.Location] = artifact
	}

	plan := Plan{}
	for _, target := range desired.Artifacts {
		entry, registered := desired.Manifest.Artifacts[target.Identity]
		if !registered || entry.Location != target.Location || entry.Digest != digest(target.Bytes) {
			plan.Blockers = append(plan.Blockers, blockerFor(target, OwnershipAmbiguousLegacy))
			continue
		}
		if hasStaleManagedArtifact(inventory, target) {
			plan.Blockers = append(plan.Blockers, blockerFor(target, OwnershipAmbiguousLegacy))
			continue
		}
		if duplicateLocations[target.Location] {
			plan.Blockers = append(plan.Blockers, blockerFor(target, OwnershipAmbiguousLegacy))
			continue
		}
		existing, found := byLocation[target.Location]
		if !found {
			plan.Operations = append(plan.Operations, writeOperation(target, desired.Manifest))
			continue
		}

		ownership := Classify(existing, desired.Manifest)
		if ownership != OwnershipProvenJarvis || existing.Identity != target.Identity {
			plan.Blockers = append(plan.Blockers, blockerFor(target, ownership))
			continue
		}
		if string(existing.Bytes) != string(target.Bytes) {
			plan.Operations = append(plan.Operations, writeOperation(target, desired.Manifest))
		}
	}
	return plan
}

func duplicateDesiredLocationBlocker(artifacts []DesiredArtifact) (Blocker, bool) {
	identitiesByLocation := make(map[string]string, len(artifacts))
	var blocker Blocker
	for _, artifact := range artifacts {
		firstIdentity, seen := identitiesByLocation[artifact.Location]
		if !seen {
			identitiesByLocation[artifact.Location] = artifact.Identity
			continue
		}
		if artifact.Identity < firstIdentity {
			identitiesByLocation[artifact.Location] = artifact.Identity
			firstIdentity = artifact.Identity
		}
		candidate := blockerFor(DesiredArtifact{Identity: firstIdentity, Location: artifact.Location}, OwnershipAmbiguousLegacy)
		if blocker.Identity == "" || candidate.Location < blocker.Location || candidate.Location == blocker.Location && candidate.Identity < blocker.Identity {
			blocker = candidate
		}
	}
	return blocker, blocker.Identity != ""
}

func blockerFor(target DesiredArtifact, ownership Ownership) Blocker {
	return Blocker{
		Identity:        target.Identity,
		Location:        target.Location,
		Ownership:       ownership,
		RecoveryCommand: "jarvis reconcile recover --artifact " + target.Identity,
	}
}

func hasStaleManagedArtifact(inventory Inventory, target DesiredArtifact) bool {
	for _, artifact := range inventory.Artifacts {
		if artifact.Location == target.Location {
			continue
		}
		if artifact.Identity == target.Identity ||
			artifact.Provenance != nil && artifact.Provenance.ManagedIdentity == target.Identity {
			return true
		}
	}
	return false
}

func writeOperation(target DesiredArtifact, manifest Manifest) Operation {
	entry := manifest.Artifacts[target.Identity]
	return Operation{
		Kind:     OperationWrite,
		Identity: target.Identity,
		Location: target.Location,
		Digest:   digest(target.Bytes),
		content:  append([]byte(nil), target.Bytes...),
		Provenance: Provenance{
			Version:         manifest.Version,
			ManagedIdentity: target.Identity,
			Location:        target.Location,
			ManifestDigest:  entry.Digest,
		},
	}
}

func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Store is the file/JSON boundary used by the core. Adapters own real paths;
// tests and callers can use fakes without touching a user home directory.
type Store interface {
	Write(path string, content []byte, provenance Provenance) error
}

// Snapshot is the prior Store representation for one managed target.
// Exists distinguishes a new target from a target whose content must be restored.
type Snapshot struct {
	Exists     bool
	Bytes      []byte
	Provenance Provenance
}

// CompensationStore provides the controlled Store operations needed to reverse
// a failed file/config transaction. Native MCP mutations deliberately do not use
// this boundary because they have no lossless restoration guarantee.
type CompensationStore interface {
	Store
	Snapshot(path string) (Snapshot, error)
	Delete(path string) error
}

// RecoveryEvidenceStore is the durable boundary for degraded Store recovery.
// Implementations persist only RecoveryEvidence's secret-safe metadata.
type RecoveryEvidenceStore interface {
	PersistDegradedRecovery(RecoveryEvidence) error
}

const recoveryEvidenceAction = "fix the Store failure and rerun Install/Reconfigure"

// FileRecoveryEvidenceStore is the production durable boundary for degraded
// Store recovery evidence. It deliberately stores only sanitized metadata.
type FileRecoveryEvidenceStore struct {
	path string
}

var (
	createRecoveryEvidenceTemp          = os.CreateTemp
	renameRecoveryEvidenceFile          = os.Rename
	syncRecoveryEvidenceParentDirectory = syncRecoveryEvidenceDirectory
)

// NewFileRecoveryEvidenceStore validates the evidence file location without
// creating it. The caller retains ownership of creating the parent directory.
func NewFileRecoveryEvidenceStore(path string) (*FileRecoveryEvidenceStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("recovery evidence path is required")
	}
	path = filepath.Clean(path)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return nil, errors.New("recovery evidence path must name a file")
	}
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		return nil, errors.New("recovery evidence directory is unavailable; create it and rerun Install/Reconfigure")
	}
	if !info.IsDir() {
		return nil, errors.New("recovery evidence parent must be a directory")
	}
	return &FileRecoveryEvidenceStore{path: path}, nil
}

// PersistDegradedRecovery writes one deterministic replacement record. The
// temporary file is created beside the destination so rename remains atomic on
// supported filesystems.
func (s *FileRecoveryEvidenceStore) PersistDegradedRecovery(evidence RecoveryEvidence) error {
	if s == nil || s.path == "" {
		return errors.New("recovery evidence store is not configured")
	}
	data, err := json.Marshal(sanitizeRecoveryEvidence(evidence))
	if err != nil {
		return errors.New("recovery evidence serialization failed; rerun Install/Reconfigure")
	}
	path := filepath.Clean(s.path)
	temp, err := createRecoveryEvidenceTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return errors.New("recovery evidence temporary write failed; verify directory access and rerun Install/Reconfigure")
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return errors.New("recovery evidence permissions could not be secured; rerun Install/Reconfigure")
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return errors.New("recovery evidence write failed; verify available storage and rerun Install/Reconfigure")
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return errors.New("recovery evidence sync failed; rerun Install/Reconfigure")
	}
	if err := temp.Close(); err != nil {
		return errors.New("recovery evidence close failed; rerun Install/Reconfigure")
	}
	if err := renameRecoveryEvidenceFile(tempPath, path); err != nil {
		return errors.New("recovery evidence replacement failed; verify directory access and rerun Install/Reconfigure")
	}
	if err := syncRecoveryEvidenceParentDirectory(filepath.Dir(path)); err != nil {
		return errors.New("recovery evidence directory sync failed; verify durable storage and rerun Install/Reconfigure")
	}
	return nil
}

// LoadDegradedRecovery reads evidence through a fresh adapter/process boundary.
func (s *FileRecoveryEvidenceStore) LoadDegradedRecovery() (RecoveryEvidence, error) {
	if s == nil || s.path == "" {
		return RecoveryEvidence{}, errors.New("recovery evidence store is not configured")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return RecoveryEvidence{}, errors.New("recovery evidence is unavailable; rerun Install/Reconfigure after repairing Store targets")
	}
	var evidence RecoveryEvidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		return RecoveryEvidence{}, errors.New("recovery evidence is invalid; repair Store targets and rerun Install/Reconfigure")
	}
	return sanitizeRecoveryEvidence(evidence), nil
}

func sanitizeRecoveryEvidence(evidence RecoveryEvidence) RecoveryEvidence {
	return RecoveryEvidence{
		FailedTarget:         sanitizeRecoveryTarget(evidence.FailedTarget),
		AffectedTargets:      sanitizeRecoveryTargets(evidence.AffectedTargets),
		CompensationFailures: sanitizeRecoveryTargets(evidence.CompensationFailures),
		RecoveryAction:       recoveryEvidenceAction,
	}
}

func sanitizeRecoveryTargets(targets []string) []string {
	sanitized := make([]string, 0, len(targets))
	for _, target := range targets {
		sanitized = append(sanitized, sanitizeRecoveryTarget(target))
	}
	return sanitized
}

func sanitizeRecoveryTarget(target string) string {
	parts := strings.Split(target, "/")
	for i, part := range parts {
		if strings.ContainsAny(part, "=?:&") {
			parts[i] = "<redacted>"
		}
	}
	return strings.Join(parts, "/")
}

type Outcome string

const (
	OutcomeCompensated          Outcome = "compensated-store-failure"
	OutcomeDegradedPartialStore Outcome = "degraded-partial-store"
)

// RecoveryEvidence is deterministic and secret-safe: it identifies affected
// targets and the fix-forward action without retaining Store error text or bytes.
type RecoveryEvidence struct {
	FailedTarget         string
	AffectedTargets      []string
	CompensationFailures []string
	RecoveryAction       string
}

type ApplyReport struct {
	Outcome  Outcome
	Recovery RecoveryEvidence
}

// Apply writes a complete non-blocked plan. A blocked plan is never partially applied.
func Apply(store Store, plan Plan) error {
	if err := validatePlan(plan); err != nil {
		return err
	}
	for _, operation := range plan.Operations {
		if err := store.Write(operation.Location, operation.content, operation.Provenance); err != nil {
			return fmt.Errorf("write %s: %w", operation.Location, err)
		}
	}
	return nil
}

type compensationEntry struct {
	operation Operation
	snapshot  Snapshot
}

// ApplyWithCompensation writes a Store plan transactionally where possible. A
// failed write triggers best-effort reverse compensation of every attempted
// target, including a write that may have partially mutated before returning an
// error. It always fails closed: compensated failures remain failures, while a
// failed compensation returns an explicit degraded partial Store outcome.
func ApplyWithCompensation(store CompensationStore, evidenceStore RecoveryEvidenceStore, plan Plan) (ApplyReport, error) {
	if err := validatePlan(plan); err != nil {
		return ApplyReport{}, err
	}

	journal := make([]compensationEntry, 0, len(plan.Operations))
	for _, operation := range plan.Operations {
		snapshot, err := store.Snapshot(operation.Location)
		if err != nil {
			return ApplyReport{}, storeFailure(operation.Location)
		}
		journal = append(journal, compensationEntry{operation: operation, snapshot: snapshot})
		if err := store.Write(operation.Location, operation.content, operation.Provenance); err != nil {
			return compensate(store, evidenceStore, journal, operation.Location)
		}
	}
	return ApplyReport{}, nil
}

func compensate(store CompensationStore, evidenceStore RecoveryEvidenceStore, journal []compensationEntry, failedTarget string) (ApplyReport, error) {
	evidence := RecoveryEvidence{
		FailedTarget:   failedTarget,
		RecoveryAction: recoveryEvidenceAction,
	}
	for i := len(journal) - 1; i >= 0; i-- {
		entry := journal[i]
		evidence.AffectedTargets = append(evidence.AffectedTargets, entry.operation.Location)
		if err := restore(store, entry); err != nil {
			evidence.CompensationFailures = append(evidence.CompensationFailures, entry.operation.Location)
		}
	}
	if len(evidence.CompensationFailures) > 0 {
		report := ApplyReport{Outcome: OutcomeDegradedPartialStore, Recovery: evidence}
		if evidenceStore == nil || evidenceStore.PersistDegradedRecovery(evidence) != nil {
			return report, degradedRecoveryPersistenceFailure(failedTarget)
		}
		return report, degradedStoreFailure(failedTarget)
	}
	return ApplyReport{Outcome: OutcomeCompensated, Recovery: evidence}, storeFailure(failedTarget)
}

func restore(store CompensationStore, entry compensationEntry) error {
	if !entry.snapshot.Exists {
		return store.Delete(entry.operation.Location)
	}
	return store.Write(entry.operation.Location, entry.snapshot.Bytes, entry.snapshot.Provenance)
}

func storeFailure(target string) error {
	return fmt.Errorf("Store transition failed at %q; fix the Store failure and rerun Install/Reconfigure", target)
}

func degradedStoreFailure(target string) error {
	return fmt.Errorf("Store transition reached degraded partial state at %q; repair affected Store targets and rerun Install/Reconfigure", target)
}

func degradedRecoveryPersistenceFailure(target string) error {
	return fmt.Errorf("Store transition reached degraded partial state at %q; recovery evidence persistence failed; repair affected Store targets and rerun Install/Reconfigure", target)
}

func validatePlan(plan Plan) error {
	if plan.Blocked() {
		return fmt.Errorf("reconciliation blocked: %w", ErrAmbiguousOwnership)
	}
	for _, operation := range plan.Operations {
		if operation.Kind != OperationWrite {
			return fmt.Errorf("unsupported reconciliation operation %q", operation.Kind)
		}
		if !operation.provenanceMatchesOperation() || operation.Digest != digest(operation.content) || operation.Provenance.ManifestDigest != operation.Digest {
			return fmt.Errorf("invalid reconciliation provenance for %s", operation.Location)
		}
	}
	return nil
}

func (o Operation) provenanceMatchesOperation() bool {
	return o.Provenance.Version != "" && o.Provenance.ManifestDigest != "" &&
		o.Provenance.ManagedIdentity == o.Identity && o.Provenance.Location == o.Location
}

// Journal is the durable-record boundary. Implementations must retain only the
// secret-free metadata exposed by JournalEntry.
type Journal interface{ Record(Plan) error }

// ApplyWithJournal records the plan before the first mutation so a later
// recovery implementation has deterministic, scoped evidence to consume.
func ApplyWithJournal(store Store, journal Journal, plan Plan) error {
	if err := validatePlan(plan); err != nil {
		return err
	}
	if len(plan.Operations) == 0 {
		return nil
	}
	if journal == nil {
		return errors.New("reconciliation journal is required")
	}
	if err := journal.Record(plan); err != nil {
		return fmt.Errorf("record reconciliation journal: %w", err)
	}
	return Apply(store, plan)
}

var ErrAmbiguousOwnership = errors.New("ownership is not proven")

// JournalPhase is deliberately metadata-only; native MCP recovery is deferred.
type JournalPhase string

const JournalPlanned JournalPhase = "planned"

// JournalEntry excludes bytes, secrets, and configuration values.
type JournalEntry struct {
	Phase      JournalPhase
	Operations []JournalOperation
}

type JournalOperation struct {
	Kind, Identity, Location, Digest string
}

// MemoryJournal is a testable durable-journal shape. Persistence and native MCP
// compensation belong to the later recovery task.
type MemoryJournal struct{ Entries []JournalEntry }

func (j *MemoryJournal) Record(plan Plan) error {
	entry := JournalEntry{Phase: JournalPlanned, Operations: make([]JournalOperation, 0, len(plan.Operations))}
	for _, operation := range plan.Operations {
		entry.Operations = append(entry.Operations, JournalOperation{Kind: string(operation.Kind), Identity: operation.Identity, Location: operation.Location, Digest: operation.Digest})
	}
	j.Entries = append(j.Entries, entry)
	return nil
}
