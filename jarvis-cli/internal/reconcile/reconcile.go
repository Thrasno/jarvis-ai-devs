// Package reconcile plans and applies ownership-safe Jarvis configuration changes.
package reconcile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
	if journal == nil {
		return errors.New("reconciliation journal is required")
	}
	if err := validatePlan(plan); err != nil {
		return err
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
