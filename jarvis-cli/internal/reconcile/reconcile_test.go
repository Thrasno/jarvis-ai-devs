package reconcile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestApplyWithCompensationRestoresTouchedTargetsAfterPartialLaterWriteFailure(t *testing.T) {
	_ = t.TempDir() // The fake Store deliberately keeps all filesystem effects isolated from a real user configuration.
	store := newFakeStore(map[string][]byte{
		"claude/CLAUDE.md":       []byte("old claude"),
		"opencode/opencode.json": []byte("old opencode"),
	})
	oldClaudeProvenance := Provenance{Version: "v1", ManagedIdentity: "claude", Location: "claude/CLAUDE.md", ManifestDigest: digestFor([]byte("old claude"))}
	oldOpenCodeProvenance := Provenance{Version: "v1", ManagedIdentity: "opencode", Location: "opencode/opencode.json", ManifestDigest: digestFor([]byte("old opencode"))}
	store.provenance["claude/CLAUDE.md"] = oldClaudeProvenance
	store.provenance["opencode/opencode.json"] = oldOpenCodeProvenance
	store.writeFailures = map[string][]fakeWriteFailure{
		"opencode/opencode.json": {{err: errors.New("credential=super-secret"), mutateBeforeFail: true}},
	}

	report, err := ApplyWithCompensation(store, nil, compensationPlan())
	if err == nil {
		t.Fatal("ApplyWithCompensation() error = nil, want failed transition")
	}
	if report.Outcome != OutcomeCompensated {
		t.Fatalf("outcome = %q, want %q", report.Outcome, OutcomeCompensated)
	}
	if !bytes.Equal(store.files["claude/CLAUDE.md"], []byte("old claude")) || !bytes.Equal(store.files["opencode/opencode.json"], []byte("old opencode")) {
		t.Fatalf("files after compensation = %#v, want prior bytes", store.files)
	}
	if store.provenance["claude/CLAUDE.md"] != oldClaudeProvenance || store.provenance["opencode/opencode.json"] != oldOpenCodeProvenance {
		t.Fatalf("provenance after compensation = %#v, want prior provenance", store.provenance)
	}
	if got, want := store.writes, []string{"claude/CLAUDE.md", "opencode/opencode.json", "opencode/opencode.json", "claude/CLAUDE.md"}; !equalStrings(got, want) {
		t.Fatalf("write order = %#v, want %#v", got, want)
	}
	if report.Recovery.FailedTarget != "opencode/opencode.json" || report.Recovery.RecoveryAction != "fix the Store failure and rerun Install/Reconfigure" {
		t.Fatalf("recovery evidence = %#v", report.Recovery)
	}
	if bytes.Contains([]byte(err.Error()), []byte("super-secret")) {
		t.Fatalf("error leaked secret: %v", err)
	}
}

func TestApplyWithCompensationReportsDeterministicDegradedRecoveryEvidence(t *testing.T) {
	_ = t.TempDir()
	store := newFakeStore(map[string][]byte{
		"claude/CLAUDE.md":       []byte("old claude"),
		"opencode/opencode.json": []byte("old opencode"),
	})
	store.provenance["claude/CLAUDE.md"] = Provenance{Version: "v1", ManagedIdentity: "claude", Location: "claude/CLAUDE.md", ManifestDigest: digestFor([]byte("old claude"))}
	store.provenance["opencode/opencode.json"] = Provenance{Version: "v1", ManagedIdentity: "opencode", Location: "opencode/opencode.json", ManifestDigest: digestFor([]byte("old opencode"))}
	store.writeFailures = map[string][]fakeWriteFailure{
		"opencode/opencode.json": {
			{err: errors.New("token=initial-secret"), mutateBeforeFail: true},
			{err: errors.New("token=rollback-secret")},
		},
		"claude/CLAUDE.md": {{}, {err: errors.New("token=second-rollback-secret")}},
	}

	report, err := ApplyWithCompensation(store, nil, compensationPlan())
	if err == nil {
		t.Fatal("ApplyWithCompensation() error = nil, want degraded partial Store error")
	}
	if report.Outcome != OutcomeDegradedPartialStore {
		t.Fatalf("outcome = %q, want %q", report.Outcome, OutcomeDegradedPartialStore)
	}
	if got, want := report.Recovery.AffectedTargets, []string{"opencode/opencode.json", "claude/CLAUDE.md"}; !equalStrings(got, want) {
		t.Fatalf("affected targets = %#v, want %#v", got, want)
	}
	if got, want := report.Recovery.CompensationFailures, []string{"opencode/opencode.json", "claude/CLAUDE.md"}; !equalStrings(got, want) {
		t.Fatalf("compensation failures = %#v, want %#v", got, want)
	}
	for _, secret := range []string{"initial-secret", "rollback-secret", "second-rollback-secret"} {
		if bytes.Contains([]byte(err.Error()), []byte(secret)) {
			t.Fatalf("error leaked secret %q: %v", secret, err)
		}
	}
}

func TestApplyWithCompensationRemovesTargetsCreatedBeforeFailure(t *testing.T) {
	_ = t.TempDir()
	store := newFakeStore(map[string][]byte{"opencode/opencode.json": []byte("old opencode")})
	store.provenance["opencode/opencode.json"] = Provenance{Version: "v1", ManagedIdentity: "opencode", Location: "opencode/opencode.json", ManifestDigest: digestFor([]byte("old opencode"))}
	store.writeFailures = map[string][]fakeWriteFailure{
		"opencode/opencode.json": {{err: errors.New("token=secret"), mutateBeforeFail: true}},
	}

	report, err := ApplyWithCompensation(store, nil, compensationPlan())
	if err == nil || report.Outcome != OutcomeCompensated {
		t.Fatalf("result = (%#v, %v), want compensated failed transition", report, err)
	}
	if _, exists := store.files["claude/CLAUDE.md"]; exists {
		t.Fatalf("new target remained after compensation: %#v", store.files)
	}
	if got, want := report.Recovery.AffectedTargets, []string{"opencode/opencode.json", "claude/CLAUDE.md"}; !equalStrings(got, want) {
		t.Fatalf("affected targets = %#v, want %#v", got, want)
	}
}

func TestApplyWithCompensationPersistsDegradedRecoveryEvidenceBeforeReturning(t *testing.T) {
	store := newFakeStore(map[string][]byte{
		"claude/CLAUDE.md":       []byte("old claude"),
		"opencode/opencode.json": []byte("old opencode"),
	})
	store.writeFailures = map[string][]fakeWriteFailure{
		"opencode/opencode.json": {
			{err: errors.New("token=initial-secret"), mutateBeforeFail: true},
			{err: errors.New("token=rollback-secret")},
		},
	}
	evidenceStore := &fakeRecoveryEvidenceStore{beforePersist: func() {
		if got, want := len(store.writes), 4; got != want {
			t.Fatalf("writes before evidence persistence = %d, want %d after all compensation attempts", got, want)
		}
	}}

	report, err := ApplyWithCompensation(store, evidenceStore, compensationPlan())
	if err == nil || report.Outcome != OutcomeDegradedPartialStore {
		t.Fatalf("result = (%#v, %v), want degraded failed transition", report, err)
	}
	if len(evidenceStore.persisted) != 1 {
		t.Fatalf("persisted evidence = %#v, want one entry", evidenceStore.persisted)
	}
	if got, want := evidenceStore.persisted[0], report.Recovery; got.FailedTarget != want.FailedTarget || !equalStrings(got.AffectedTargets, want.AffectedTargets) || !equalStrings(got.CompensationFailures, want.CompensationFailures) || got.RecoveryAction != want.RecoveryAction {
		t.Fatalf("persisted evidence = %#v, want %#v", got, want)
	}
	if got, want := evidenceStore.events, []string{"persist:opencode/opencode.json"}; !equalStrings(got, want) {
		t.Fatalf("persistence order = %#v, want %#v", got, want)
	}
	for _, secret := range []string{"initial-secret", "rollback-secret"} {
		if bytes.Contains([]byte(evidenceStore.persisted[0].RecoveryAction), []byte(secret)) || bytes.Contains([]byte(err.Error()), []byte(secret)) {
			t.Fatalf("persistence leaked secret %q", secret)
		}
	}

	rerun, rerunErr := ApplyWithCompensation(store, evidenceStore, compensationPlan())
	if rerunErr != nil || rerun.Outcome != "" {
		t.Fatalf("deterministic rerun = (%#v, %v), want success", rerun, rerunErr)
	}
	if len(evidenceStore.persisted) != 1 {
		t.Fatalf("persisted evidence after successful rerun = %#v, want original evidence only", evidenceStore.persisted)
	}
}

func TestApplyWithCompensationFailsClosedWhenDegradedEvidencePersistenceFails(t *testing.T) {
	store := newFakeStore(map[string][]byte{
		"claude/CLAUDE.md":       []byte("old claude"),
		"opencode/opencode.json": []byte("old opencode"),
	})
	store.writeFailures = map[string][]fakeWriteFailure{
		"opencode/opencode.json": {
			{err: errors.New("token=initial-secret"), mutateBeforeFail: true},
			{err: errors.New("token=rollback-secret")},
		},
	}
	evidenceStore := &fakeRecoveryEvidenceStore{err: errors.New("credential=evidence-secret")}

	report, err := ApplyWithCompensation(store, evidenceStore, compensationPlan())
	if err == nil || report.Outcome != OutcomeDegradedPartialStore {
		t.Fatalf("result = (%#v, %v), want degraded failed transition", report, err)
	}
	if len(evidenceStore.persisted) != 1 || report.Recovery.FailedTarget != "opencode/opencode.json" || len(report.Recovery.CompensationFailures) != 1 {
		t.Fatalf("recovery classification = %#v, persisted = %#v, want actionable sanitized evidence", report.Recovery, evidenceStore.persisted)
	}
	for _, secret := range []string{"initial-secret", "rollback-secret", "evidence-secret"} {
		if bytes.Contains([]byte(err.Error()), []byte(secret)) {
			t.Fatalf("error leaked secret %q: %v", secret, err)
		}
	}
	if !bytes.Contains([]byte(err.Error()), []byte("recovery evidence persistence failed")) {
		t.Fatalf("error = %v, want sanitized persistence classification", err)
	}
}

func TestApplyWithCompensationDoesNotPersistEvidenceAfterSuccessOrFullCompensation(t *testing.T) {
	for _, tt := range []struct {
		name  string
		store *fakeStore
		want  Outcome
	}{
		{name: "success", store: newFakeStore(nil)},
		{name: "full compensation", store: func() *fakeStore {
			store := newFakeStore(nil)
			store.writeFailures = map[string][]fakeWriteFailure{"opencode/opencode.json": {{err: errors.New("token=secret"), mutateBeforeFail: true}}}
			return store
		}(), want: OutcomeCompensated},
	} {
		t.Run(tt.name, func(t *testing.T) {
			evidenceStore := &fakeRecoveryEvidenceStore{}
			report, err := ApplyWithCompensation(tt.store, evidenceStore, compensationPlan())
			if tt.want == "" && err != nil {
				t.Fatalf("ApplyWithCompensation() error = %v, want nil", err)
			}
			if tt.want != "" && (err == nil || report.Outcome != tt.want) {
				t.Fatalf("result = (%#v, %v), want %q failure", report, err, tt.want)
			}
			if len(evidenceStore.persisted) != 0 {
				t.Fatalf("persisted evidence = %#v, want none", evidenceStore.persisted)
			}
		})
	}
}

func TestFileRecoveryEvidenceStorePersistsDeterministicSanitizedReplacementEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery-evidence.json")
	store, err := NewFileRecoveryEvidenceStore(path)
	if err != nil {
		t.Fatalf("NewFileRecoveryEvidenceStore() error = %v", err)
	}
	first := RecoveryEvidence{
		FailedTarget:         "opencode/token=super-secret",
		AffectedTargets:      []string{"claude/CLAUDE.md", "opencode/token=super-secret"},
		CompensationFailures: []string{"opencode/token=super-secret"},
		RecoveryAction:       "do not persist this caller-controlled action",
	}
	if err := store.PersistDegradedRecovery(first); err != nil {
		t.Fatalf("PersistDegradedRecovery() error = %v", err)
	}
	firstBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(firstBytes), "super-secret") || strings.Contains(string(firstBytes), "caller-controlled") {
		t.Fatalf("persisted evidence leaked unsafe input: %s", firstBytes)
	}

	fresh, err := NewFileRecoveryEvidenceStore(path)
	if err != nil {
		t.Fatalf("fresh NewFileRecoveryEvidenceStore() error = %v", err)
	}
	got, err := fresh.LoadDegradedRecovery()
	if err != nil {
		t.Fatalf("LoadDegradedRecovery() error = %v", err)
	}
	wantFirst := RecoveryEvidence{
		FailedTarget:         "opencode/<redacted>",
		AffectedTargets:      []string{"claude/CLAUDE.md", "opencode/<redacted>"},
		CompensationFailures: []string{"opencode/<redacted>"},
		RecoveryAction:       "fix the Store failure and rerun Install/Reconfigure",
	}
	if got.FailedTarget != wantFirst.FailedTarget || !equalStrings(got.AffectedTargets, wantFirst.AffectedTargets) || !equalStrings(got.CompensationFailures, wantFirst.CompensationFailures) || got.RecoveryAction != wantFirst.RecoveryAction {
		t.Fatalf("loaded evidence = %#v, want %#v", got, wantFirst)
	}
	if err := store.PersistDegradedRecovery(first); err != nil {
		t.Fatalf("second PersistDegradedRecovery() error = %v", err)
	}
	secondBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("second ReadFile() error = %v", err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("serialized bytes changed across equivalent writes: %q != %q", firstBytes, secondBytes)
	}

	replacement := RecoveryEvidence{FailedTarget: "claude/CLAUDE.md", AffectedTargets: []string{"claude/CLAUDE.md"}, RecoveryAction: "ignored"}
	if err := store.PersistDegradedRecovery(replacement); err != nil {
		t.Fatalf("replacement PersistDegradedRecovery() error = %v", err)
	}
	got, err = fresh.LoadDegradedRecovery()
	if err != nil {
		t.Fatalf("LoadDegradedRecovery() after replacement error = %v", err)
	}
	if got.FailedTarget != replacement.FailedTarget || !equalStrings(got.AffectedTargets, replacement.AffectedTargets) || got.RecoveryAction != "fix the Store failure and rerun Install/Reconfigure" {
		t.Fatalf("replacement evidence = %#v", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("permissions = %o, want 600", got)
		}
	}
}

func TestFileRecoveryEvidenceStoreRejectsInvalidPathsAndCleansFailedTemporaryFiles(t *testing.T) {
	if _, err := NewFileRecoveryEvidenceStore(""); err == nil {
		t.Fatal("NewFileRecoveryEvidenceStore(\"\") error = nil, want validation failure")
	}
	if _, err := NewFileRecoveryEvidenceStore(t.TempDir()); err == nil {
		t.Fatal("NewFileRecoveryEvidenceStore(directory) error = nil, want validation failure")
	}
	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := NewFileRecoveryEvidenceStore(filepath.Join(parentFile, "recovery.json")); err == nil {
		t.Fatal("NewFileRecoveryEvidenceStore(file parent) error = nil, want validation failure")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "recovery.json")
	store, err := NewFileRecoveryEvidenceStore(path)
	if err != nil {
		t.Fatalf("NewFileRecoveryEvidenceStore() error = %v", err)
	}
	originalRename := renameRecoveryEvidenceFile
	renameRecoveryEvidenceFile = func(_, _ string) error { return errors.New("credential=rename-secret") }
	t.Cleanup(func() { renameRecoveryEvidenceFile = originalRename })
	if err := store.PersistDegradedRecovery(RecoveryEvidence{FailedTarget: "claude/CLAUDE.md"}); err == nil {
		t.Fatal("PersistDegradedRecovery() error = nil, want rename failure")
	} else if strings.Contains(err.Error(), "rename-secret") {
		t.Fatalf("error leaked raw rename failure: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary debris = %#v, want none", entries)
	}
}

func TestFileRecoveryEvidenceStoreSanitizesTemporaryWriteFailures(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileRecoveryEvidenceStore(filepath.Join(dir, "recovery.json"))
	if err != nil {
		t.Fatalf("NewFileRecoveryEvidenceStore() error = %v", err)
	}
	originalCreateTemp := createRecoveryEvidenceTemp
	createRecoveryEvidenceTemp = func(_, _ string) (*os.File, error) { return nil, errors.New("token=write-secret") }
	t.Cleanup(func() { createRecoveryEvidenceTemp = originalCreateTemp })
	if err := store.PersistDegradedRecovery(RecoveryEvidence{FailedTarget: "claude/CLAUDE.md"}); err == nil {
		t.Fatal("PersistDegradedRecovery() error = nil, want write failure")
	} else if strings.Contains(err.Error(), "write-secret") {
		t.Fatalf("error leaked raw write failure: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary debris = %#v, want none", entries)
	}
}

func TestFileRecoveryEvidenceStoreSyncsParentDirectoryAfterRenameAndFailsClosed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recovery.json")
	store, err := NewFileRecoveryEvidenceStore(path)
	if err != nil {
		t.Fatalf("NewFileRecoveryEvidenceStore() error = %v", err)
	}

	originalSyncDirectory := syncRecoveryEvidenceParentDirectory
	t.Cleanup(func() { syncRecoveryEvidenceParentDirectory = originalSyncDirectory })

	for _, tt := range []struct {
		name      string
		syncErr   error
		wantError string
	}{
		{name: "syncs renamed evidence", wantError: ""},
		{name: "fails closed when directory sync is uncertain", syncErr: errors.New("credential=directory-sync-secret"), wantError: "directory sync failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			syncRecoveryEvidenceParentDirectory = func(got string) error {
				called = true
				if got != dir {
					t.Fatalf("directory sync path = %q, want %q", got, dir)
				}
				return tt.syncErr
			}

			err := store.PersistDegradedRecovery(RecoveryEvidence{FailedTarget: "claude/CLAUDE.md"})
			if !called {
				t.Fatal("PersistDegradedRecovery() did not sync the parent directory after rename")
			}
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("PersistDegradedRecovery() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("PersistDegradedRecovery() error = %v, want sanitized %q failure", err, tt.wantError)
			}
			if strings.Contains(err.Error(), "directory-sync-secret") {
				t.Fatalf("error leaked raw directory sync failure: %v", err)
			}
			if _, readErr := os.ReadFile(path); readErr != nil {
				t.Fatalf("renamed evidence is unavailable after directory sync failure: %v", readErr)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatalf("ReadDir() error = %v", readErr)
			}
			if len(entries) != 1 || entries[0].Name() != "recovery.json" {
				t.Fatalf("directory state = %#v, want only renamed evidence", entries)
			}
		})
	}
}

type fakeRecoveryEvidenceStore struct {
	persisted     []RecoveryEvidence
	events        []string
	err           error
	beforePersist func()
}

func (s *fakeRecoveryEvidenceStore) PersistDegradedRecovery(evidence RecoveryEvidence) error {
	if s.beforePersist != nil {
		s.beforePersist()
	}
	s.persisted = append(s.persisted, evidence)
	s.events = append(s.events, "persist:"+evidence.FailedTarget)
	return s.err
}

type fakeStore struct {
	files          map[string][]byte
	provenance     map[string]Provenance
	writes         []string
	writeFailures  map[string][]fakeWriteFailure
	deleteFailures map[string]error
}

type fakeWriteFailure struct {
	err              error
	mutateBeforeFail bool
}

func newFakeStore(files map[string][]byte) *fakeStore {
	copyFiles := make(map[string][]byte, len(files))
	for path, content := range files {
		copyFiles[path] = append([]byte(nil), content...)
	}
	return &fakeStore{files: copyFiles, provenance: make(map[string]Provenance)}
}

func (s *fakeStore) Write(path string, content []byte, provenance Provenance) error {
	s.writes = append(s.writes, path)
	if failures := s.writeFailures[path]; len(failures) > 0 {
		failure := failures[0]
		s.writeFailures[path] = failures[1:]
		if failure.mutateBeforeFail {
			s.files[path] = append([]byte(nil), content...)
			s.provenance[path] = provenance
		}
		return failure.err
	}
	s.files[path] = append([]byte(nil), content...)
	s.provenance[path] = provenance
	return nil
}

func (s *fakeStore) Snapshot(path string) (Snapshot, error) {
	content, exists := s.files[path]
	if !exists {
		return Snapshot{}, nil
	}
	return Snapshot{Exists: true, Bytes: append([]byte(nil), content...), Provenance: s.provenance[path]}, nil
}

func (s *fakeStore) Delete(path string) error {
	if err := s.deleteFailures[path]; err != nil {
		delete(s.deleteFailures, path)
		return err
	}
	delete(s.files, path)
	delete(s.provenance, path)
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

func compensationPlan() Plan {
	claude := []byte("new claude")
	opencode := []byte("new opencode")
	return Plan{Operations: []Operation{
		{Kind: OperationWrite, Identity: "claude", Location: "claude/CLAUDE.md", Digest: digestFor(claude), Provenance: Provenance{Version: "v1", ManagedIdentity: "claude", Location: "claude/CLAUDE.md", ManifestDigest: digestFor(claude)}, content: claude},
		{Kind: OperationWrite, Identity: "opencode", Location: "opencode/opencode.json", Digest: digestFor(opencode), Provenance: Provenance{Version: "v1", ManagedIdentity: "opencode", Location: "opencode/opencode.json", ManifestDigest: digestFor(opencode)}, content: opencode},
	}}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
