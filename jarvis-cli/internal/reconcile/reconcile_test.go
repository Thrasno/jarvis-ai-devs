package reconcile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestClassifyRequiresVersionedManifestBoundProvenance(t *testing.T) {
	managedBytes := []byte("managed bytes")
	manifest := Manifest{Version: "v1", Artifacts: map[string]ManifestEntry{
		"claude-instructions": {Location: "claude/CLAUDE.md", Digest: digestFor(managedBytes)},
	}}

	for _, tt := range []struct {
		name     string
		artifact Artifact
		want     Ownership
	}{
		{
			name:     "reserved name is not ownership proof",
			artifact: Artifact{Identity: "claude-instructions", Location: "claude/CLAUDE.md", Bytes: []byte("user bytes")},
			want:     OwnershipAmbiguousLegacy,
		},
		{
			name:     "path is not ownership proof",
			artifact: Artifact{Identity: "other", Location: "claude/CLAUDE.md", Bytes: []byte("user bytes")},
			want:     OwnershipForeign,
		},
		{
			name:     "wrong manifest digest is ambiguous",
			artifact: Artifact{Identity: "claude-instructions", Location: "claude/CLAUDE.md", Bytes: managedBytes, Provenance: &Provenance{Version: "v1", ManagedIdentity: "claude-instructions", Location: "claude/CLAUDE.md", ManifestDigest: "wrong"}},
			want:     OwnershipAmbiguousLegacy,
		},
		{
			name:     "versioned manifest bound marker proves jarvis ownership",
			artifact: Artifact{Identity: "claude-instructions", Location: "claude/CLAUDE.md", Bytes: managedBytes, Provenance: &Provenance{Version: "v1", ManagedIdentity: "claude-instructions", Location: "claude/CLAUDE.md", ManifestDigest: digestFor(managedBytes)}},
			want:     OwnershipProvenJarvis,
		},
		{
			name:     "cross identity marker is not ownership proof",
			artifact: Artifact{Identity: "other", Location: "claude/CLAUDE.md", Provenance: &Provenance{Version: "v1", ManagedIdentity: "claude-instructions", Location: "claude/CLAUDE.md", ManifestDigest: "expected"}},
			want:     OwnershipForeign,
		},
		{
			name:     "wrong location marker is not ownership proof",
			artifact: Artifact{Identity: "claude-instructions", Location: "copied/CLAUDE.md", Provenance: &Provenance{Version: "v1", ManagedIdentity: "claude-instructions", Location: "claude/CLAUDE.md", ManifestDigest: "expected"}},
			want:     OwnershipAmbiguousLegacy,
		},
		{
			name:     "replayed marker from another version is not ownership proof",
			artifact: Artifact{Identity: "claude-instructions", Location: "claude/CLAUDE.md", Provenance: &Provenance{Version: "v0", ManagedIdentity: "claude-instructions", Location: "claude/CLAUDE.md", ManifestDigest: "expected"}},
			want:     OwnershipAmbiguousLegacy,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.artifact, manifest); got != tt.want {
				t.Fatalf("Classify() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyRejectsTamperedBytesWithValidProvenance(t *testing.T) {
	managedBytes := []byte("managed bytes")
	manifest := Manifest{Version: "v1", Artifacts: map[string]ManifestEntry{
		"claude-instructions": {Location: "claude/CLAUDE.md", Digest: digestFor(managedBytes)},
	}}
	artifact := Artifact{
		Identity: "claude-instructions", Location: "claude/CLAUDE.md", Bytes: []byte("tampered bytes"),
		Provenance: &Provenance{Version: "v1", ManagedIdentity: "claude-instructions", Location: "claude/CLAUDE.md", ManifestDigest: digestFor(managedBytes)},
	}

	if got := Classify(artifact, manifest); got != OwnershipAmbiguousLegacy {
		t.Fatalf("Classify() = %q, want %q for tampered bytes", got, OwnershipAmbiguousLegacy)
	}
}

func TestPlanAndApplyPreserveAmbiguousLegacyBytesAndReferences(t *testing.T) {
	const location = "claude/settings.json"
	userBytes := []byte(`{"outputStyle":"Argentino","userKey":{"keep":true}}`)
	store := newFakeStore(map[string][]byte{location: userBytes})
	desired := DesiredState{Manifest: Manifest{Version: "v1", Artifacts: map[string]ManifestEntry{
		"claude-output-style": {Location: location, Digest: digestFor([]byte(`{"outputStyle":"Jarvis"}`))},
	}}, Artifacts: []DesiredArtifact{{Identity: "claude-output-style", Location: location, Bytes: []byte(`{"outputStyle":"Jarvis"}`)}}}
	inventory := Inventory{Artifacts: []Artifact{{Identity: "claude-output-style", Location: location, Bytes: userBytes, References: []string{"outputStyle"}}}}

	plan := BuildPlan(inventory, desired)
	if !plan.Blocked() || len(plan.Blockers) != 1 {
		t.Fatalf("plan = %#v, want one blocking collision", plan)
	}
	if plan.Blockers[0].RecoveryCommand == "" {
		t.Fatal("ambiguous collision must provide recovery guidance")
	}
	if err := Apply(store, plan); err == nil {
		t.Fatal("Apply() error = nil, want blocked plan error")
	}
	if got := store.files[location]; !bytes.Equal(got, userBytes) {
		t.Fatalf("user bytes changed: %q", got)
	}
	if len(store.writes) != 0 {
		t.Fatalf("writes = %#v, want none", store.writes)
	}
}

func TestReconciliationConvergesAndIsIdempotent(t *testing.T) {
	const location = "opencode/opencode.json"
	desiredBytes := []byte(`{"mcp":{"hive":{"type":"local"}},"userKey":"preserved"}`)
	desired := DesiredState{Manifest: Manifest{Version: "v1", Artifacts: map[string]ManifestEntry{
		"opencode-config": {Location: location, Digest: digestFor(desiredBytes)},
	}}, Artifacts: []DesiredArtifact{{Identity: "opencode-config", Location: location, Bytes: desiredBytes}}}
	store := newFakeStore(nil)

	first := BuildPlan(store.Inventory(), desired)
	if err := Apply(store, first); err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}
	if got := store.files[location]; !bytes.Equal(got, desiredBytes) {
		t.Fatalf("first output = %q, want %q", got, desiredBytes)
	}
	if got := store.provenance[location]; got != (Provenance{Version: "v1", ManagedIdentity: "opencode-config", Location: location, ManifestDigest: digestFor(desiredBytes)}) {
		t.Fatalf("persisted provenance = %#v", got)
	}

	second := BuildPlan(store.Inventory(), desired)
	if len(second.Operations) != 0 || second.Blocked() {
		t.Fatalf("second plan = %#v, want no-op", second)
	}
	if err := Apply(store, second); err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}
	if len(store.writes) != 1 {
		t.Fatalf("writes = %#v, want exactly initial write", store.writes)
	}
}

func TestJournaledReconciliationConvergesWithoutDurableNoOp(t *testing.T) {
	const location = "opencode/opencode.json"
	desiredBytes := []byte(`{"mcp":{"hive":{"type":"local"}}}`)
	desired := DesiredState{Manifest: Manifest{Version: "v1", Artifacts: map[string]ManifestEntry{
		"opencode-config": {Location: location, Digest: digestFor(desiredBytes)},
	}}, Artifacts: []DesiredArtifact{{Identity: "opencode-config", Location: location, Bytes: desiredBytes}}}
	store := newFakeStore(nil)
	journal := &MemoryJournal{}

	for run := 0; run < 2; run++ {
		if err := ApplyWithJournal(store, journal, BuildPlan(store.Inventory(), desired)); err != nil {
			t.Fatalf("reconciliation run %d: %v", run+1, err)
		}
	}
	if len(journal.Entries) != 1 || len(store.writes) != 1 || len(store.provenance) != 1 {
		t.Fatalf("journal = %#v, writes = %#v, provenance = %#v, want only the initial durable mutation", journal.Entries, store.writes, store.provenance)
	}
}

func TestBuildPlanBlocksDesiredManifestMismatchWithoutMutation(t *testing.T) {
	store := newFakeStore(nil)
	desired := DesiredState{Manifest: Manifest{Version: "v1", Artifacts: map[string]ManifestEntry{
		"claude-instructions": {Location: "claude/CLAUDE.md", Digest: digestFor([]byte("managed bytes"))},
	}}, Artifacts: []DesiredArtifact{{Identity: "claude-instructions", Location: "copied/CLAUDE.md", Bytes: []byte("managed bytes")}}}

	plan := BuildPlan(store.Inventory(), desired)
	if !plan.Blocked() || len(plan.Operations) != 0 {
		t.Fatalf("plan = %#v, want blocked plan without operations", plan)
	}
	if err := Apply(store, plan); err == nil {
		t.Fatal("Apply() error = nil, want blocked plan error")
	}
	if len(store.writes) != 0 {
		t.Fatalf("writes = %#v, want none", store.writes)
	}
}

func TestBuildPlanBlocksCopiedMarkerAtWrongLocationWithoutMutation(t *testing.T) {
	const managedLocation = "claude/CLAUDE.md"
	const copiedLocation = "copied/CLAUDE.md"
	userBytes := []byte("user bytes")
	store := newFakeStore(map[string][]byte{copiedLocation: userBytes})
	store.provenance[copiedLocation] = Provenance{Version: "v1", ManagedIdentity: "claude-instructions", Location: managedLocation, ManifestDigest: digestFor([]byte("managed bytes"))}
	desired := DesiredState{Manifest: Manifest{Version: "v1", Artifacts: map[string]ManifestEntry{
		"claude-instructions": {Location: managedLocation, Digest: digestFor([]byte("managed bytes"))},
	}}, Artifacts: []DesiredArtifact{{Identity: "claude-instructions", Location: managedLocation, Bytes: []byte("managed bytes")}}}

	plan := BuildPlan(store.Inventory(), desired)
	if !plan.Blocked() || len(plan.Operations) != 0 {
		t.Fatalf("plan = %#v, want blocked plan without operations", plan)
	}
	if err := Apply(store, plan); err == nil {
		t.Fatal("Apply() error = nil, want blocked plan error")
	}
	if got := store.files[copiedLocation]; !bytes.Equal(got, userBytes) {
		t.Fatalf("copied user bytes changed: %q", got)
	}
	if _, exists := store.files[managedLocation]; exists {
		t.Fatal("managed location was written despite copied marker blocker")
	}
}

func TestBuildPlanBlocksCrossIdentityMarkerWithoutMutation(t *testing.T) {
	const location = "claude/CLAUDE.md"
	userBytes := []byte("user bytes")
	store := newFakeStore(map[string][]byte{location: userBytes})
	desired := DesiredState{Manifest: Manifest{Version: "v1", Artifacts: map[string]ManifestEntry{
		"claude-instructions": {Location: location, Digest: digestFor([]byte("managed bytes"))},
	}}, Artifacts: []DesiredArtifact{{Identity: "claude-instructions", Location: location, Bytes: []byte("managed bytes")}}}
	inventory := Inventory{Artifacts: []Artifact{{
		Identity: "other-identity", Location: location, Bytes: userBytes,
		Provenance: &Provenance{Version: "v1", ManagedIdentity: "claude-instructions", Location: location, ManifestDigest: digestFor([]byte("managed bytes"))},
	}}}

	plan := BuildPlan(inventory, desired)
	if !plan.Blocked() || len(plan.Operations) != 0 {
		t.Fatalf("plan = %#v, want blocked plan without operations", plan)
	}
	if err := Apply(store, plan); err == nil {
		t.Fatal("Apply() error = nil, want blocked plan error")
	}
	if got := store.files[location]; !bytes.Equal(got, userBytes) {
		t.Fatalf("user bytes changed: %q", got)
	}
}

func TestBuildPlanBlocksValidCrossIdentitySameLocationWithoutMutation(t *testing.T) {
	const location = "shared/config.json"
	identityABytes := []byte("desired A bytes")
	identityBBytes := []byte("managed B bytes")
	store := newFakeStore(map[string][]byte{location: identityBBytes})
	store.provenance[location] = Provenance{Version: "v1", ManagedIdentity: "identity-b", Location: location, ManifestDigest: digestFor(identityBBytes)}
	desired := DesiredState{Manifest: Manifest{Version: "v1", Artifacts: map[string]ManifestEntry{
		"identity-a": {Location: location, Digest: digestFor(identityABytes)},
		"identity-b": {Location: location, Digest: digestFor(identityBBytes)},
	}}, Artifacts: []DesiredArtifact{{Identity: "identity-a", Location: location, Bytes: identityABytes}}}

	plan := BuildPlan(store.Inventory(), desired)
	if !plan.Blocked() || len(plan.Operations) != 0 {
		t.Fatalf("plan = %#v, want blocked plan without operations", plan)
	}
	if err := Apply(store, plan); err == nil {
		t.Fatal("Apply() error = nil, want blocked plan error")
	}
	if got := store.files[location]; !bytes.Equal(got, identityBBytes) {
		t.Fatalf("identity B bytes changed: %q", got)
	}
	if got := store.provenance[location]; got.ManagedIdentity != "identity-b" {
		t.Fatalf("identity B provenance changed: %#v", got)
	}
}

func TestBuildPlanBlocksDesiredBytesDigestMismatchWithoutMutation(t *testing.T) {
	const location = "claude/CLAUDE.md"
	store := newFakeStore(nil)
	desired := DesiredState{Manifest: Manifest{Version: "v1", Artifacts: map[string]ManifestEntry{
		"claude-instructions": {Location: location, Digest: digestFor([]byte("other bytes"))},
	}}, Artifacts: []DesiredArtifact{{Identity: "claude-instructions", Location: location, Bytes: []byte("managed bytes")}}}

	plan := BuildPlan(store.Inventory(), desired)
	if !plan.Blocked() || len(plan.Operations) != 0 {
		t.Fatalf("plan = %#v, want blocked plan without operations", plan)
	}
	if err := Apply(store, plan); err == nil {
		t.Fatal("Apply() error = nil, want blocked plan error")
	}
	if _, exists := store.files[location]; exists {
		t.Fatal("artifact was written despite desired digest mismatch")
	}
	if _, exists := store.provenance[location]; exists {
		t.Fatal("provenance was written despite desired digest mismatch")
	}
}

func TestJournalRecordsOnlySecretFreeOperationMetadata(t *testing.T) {
	journal := &MemoryJournal{}
	content := []byte(`{"mcp":{"hive":{"type":"local"}}}`)
	plan := Plan{Operations: []Operation{{
		Kind:       OperationWrite,
		Identity:   "hive",
		Location:   "opencode/opencode.json",
		Digest:     digestFor(content),
		Provenance: Provenance{Version: "v1", ManagedIdentity: "hive", Location: "opencode/opencode.json", ManifestDigest: digestFor(content)},
		content:    content,
	}}}
	if err := ApplyWithJournal(newFakeStore(nil), journal, plan); err != nil {
		t.Fatalf("ApplyWithJournal() error = %v", err)
	}
	if len(journal.Entries) != 1 || journal.Entries[0].Phase != JournalPlanned {
		t.Fatalf("entries = %#v", journal.Entries)
	}
}

func TestApplyWithJournalRejectsBlockedOrInvalidPlansBeforeRecording(t *testing.T) {
	for _, tt := range []struct {
		name string
		plan Plan
	}{
		{
			name: "blocked plan",
			plan: Plan{Blockers: []Blocker{{Identity: "claude-instructions", Location: "claude/CLAUDE.md", Ownership: OwnershipAmbiguousLegacy}}},
		},
		{
			name: "invalid write operation",
			plan: Plan{Operations: []Operation{{
				Kind: OperationWrite, Identity: "claude-instructions", Location: "claude/CLAUDE.md",
				Digest: "sha256:invalid", Provenance: Provenance{Version: "v1", ManagedIdentity: "claude-instructions", Location: "claude/CLAUDE.md", ManifestDigest: "sha256:invalid"},
			}}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore(nil)
			journal := &MemoryJournal{}
			if err := ApplyWithJournal(store, journal, tt.plan); err == nil {
				t.Fatal("ApplyWithJournal() error = nil, want validation failure")
			}
			if len(journal.Entries) != 0 {
				t.Fatalf("journal entries = %#v, want none", journal.Entries)
			}
			if len(store.writes) != 0 || len(store.provenance) != 0 {
				t.Fatalf("writes = %#v, provenance = %#v, want no mutation", store.writes, store.provenance)
			}
		})
	}
}

func TestBuildPlanBlocksDuplicateInventoryLocationsRegardlessOrder(t *testing.T) {
	const location = "claude/CLAUDE.md"
	desiredBytes := []byte("managed bytes")
	desired := DesiredState{Manifest: Manifest{Version: "v1", Artifacts: map[string]ManifestEntry{
		"claude-instructions": {Location: location, Digest: digestFor(desiredBytes)},
	}}, Artifacts: []DesiredArtifact{{Identity: "claude-instructions", Location: location, Bytes: desiredBytes}}}
	duplicates := []Artifact{
		{Identity: "claude-instructions", Location: location, Bytes: desiredBytes, Provenance: &Provenance{Version: "v1", ManagedIdentity: "claude-instructions", Location: location, ManifestDigest: digestFor(desiredBytes)}},
		{Identity: "foreign", Location: location, Bytes: []byte("foreign bytes")},
	}

	for _, tt := range []struct {
		name      string
		artifacts []Artifact
	}{
		{name: "proven entry first", artifacts: duplicates},
		{name: "foreign entry first", artifacts: []Artifact{duplicates[1], duplicates[0]}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore(nil)
			journal := &MemoryJournal{}
			plan := BuildPlan(Inventory{Artifacts: tt.artifacts}, desired)
			if !plan.Blocked() || len(plan.Blockers) != 1 || plan.Blockers[0].Ownership != OwnershipAmbiguousLegacy {
				t.Fatalf("plan = %#v, want deterministic ambiguous-location blocker", plan)
			}
			if err := ApplyWithJournal(store, journal, plan); err == nil {
				t.Fatal("ApplyWithJournal() error = nil, want blocked plan error")
			}
			if len(journal.Entries) != 0 || len(store.writes) != 0 || len(store.provenance) != 0 {
				t.Fatalf("journal = %#v, writes = %#v, provenance = %#v, want no durable mutation", journal.Entries, store.writes, store.provenance)
			}
		})
	}
}

func TestBuildPlanBlocksDuplicateDesiredLocationsRegardlessOrder(t *testing.T) {
	const location = "shared/config.json"
	identityA := DesiredArtifact{Identity: "identity-a", Location: location, Bytes: []byte("managed A bytes")}
	identityB := DesiredArtifact{Identity: "identity-b", Location: location, Bytes: []byte("managed B bytes")}
	desired := func(artifacts []DesiredArtifact) DesiredState {
		return DesiredState{Manifest: Manifest{Version: "v1", Artifacts: map[string]ManifestEntry{
			"identity-a": {Location: location, Digest: digestFor(identityA.Bytes)},
			"identity-b": {Location: location, Digest: digestFor(identityB.Bytes)},
		}}, Artifacts: artifacts}
	}

	for _, tt := range []struct {
		name      string
		artifacts []DesiredArtifact
	}{
		{name: "identity a first", artifacts: []DesiredArtifact{identityA, identityB}},
		{name: "identity b first", artifacts: []DesiredArtifact{identityB, identityA}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore(nil)
			journal := &MemoryJournal{}
			plan := BuildPlan(store.Inventory(), desired(tt.artifacts))
			if !plan.Blocked() || len(plan.Operations) != 0 || len(plan.Blockers) != 1 {
				t.Fatalf("plan = %#v, want one duplicate-location blocker without operations", plan)
			}
			if got := plan.Blockers[0]; got.Identity != "identity-a" || got.Location != location || got.RecoveryCommand != "jarvis reconcile recover --artifact identity-a" {
				t.Fatalf("blocker = %#v, want deterministic actionable identity-a blocker", got)
			}
			if err := ApplyWithJournal(store, journal, plan); err == nil {
				t.Fatal("ApplyWithJournal() error = nil, want duplicate desired location error")
			}
			if len(journal.Entries) != 0 || len(store.writes) != 0 || len(store.provenance) != 0 {
				t.Fatalf("journal = %#v, writes = %#v, provenance = %#v, want no durable mutation", journal.Entries, store.writes, store.provenance)
			}
		})
	}
}

type fakeStore struct {
	files      map[string][]byte
	provenance map[string]Provenance
	writes     []string
}

func newFakeStore(files map[string][]byte) *fakeStore {
	copyFiles := make(map[string][]byte, len(files))
	for path, content := range files {
		copyFiles[path] = append([]byte(nil), content...)
	}
	return &fakeStore{files: copyFiles, provenance: make(map[string]Provenance)}
}

func (s *fakeStore) Write(path string, content []byte, provenance Provenance) error {
	s.files[path] = append([]byte(nil), content...)
	s.provenance[path] = provenance
	s.writes = append(s.writes, path)
	return nil
}

func (s *fakeStore) Inventory() Inventory {
	inventory := Inventory{Artifacts: make([]Artifact, 0, len(s.files))}
	for location, content := range s.files {
		marker, hasMarker := s.provenance[location]
		artifact := Artifact{Location: location, Bytes: append([]byte(nil), content...)}
		if hasMarker {
			artifact.Identity = marker.ManagedIdentity
			artifact.Provenance = &marker
		}
		inventory.Artifacts = append(inventory.Artifacts, artifact)
	}
	return inventory
}

func digestFor(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}
