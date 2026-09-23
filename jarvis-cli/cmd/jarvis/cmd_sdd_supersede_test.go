package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
)

func TestPlanNewSupersessionSeal(t *testing.T) {
	old, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "project", Change: "old", Generation: 1, Revision: 1, TaskManifestSHA256: strings.Repeat("a", 64), Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	preflight := newSupersessionPreflightResult{Predecessor: old, TaskManifest: strings.Repeat("b", 64)}
	now := time.Date(2025, 1, 2, 3, 4, 5, 6, time.FixedZone("offset", 3600))
	req, err := planNewSupersessionSeal(preflight, "next", "actor", "reason", now, "operation")
	if err != nil {
		t.Fatal(err)
	}
	if req.RequestID != "operation" || req.ExpectedDigest != old.Digest || req.ExpectedRevision != old.Revision || req.Snapshot.SealIntent.Timestamp != now.UTC().Format(time.RFC3339Nano) || !applyprogress.IsSupersessionSeal(old, req.Snapshot) {
		t.Fatalf("invalid request: %+v", req)
	}
	retry, err := retrySupersessionSealRequest(req.Snapshot, "next", nil, nil)
	if err != nil || !reflect.DeepEqual(retry, req) {
		t.Fatalf("retry differs: %+v, %v", retry, err)
	}
	for _, change := range []func(*newSupersessionPreflightResult){func(p *newSupersessionPreflightResult) { p.TaskManifest = old.TaskManifestSHA256 }, func(p *newSupersessionPreflightResult) { p.TaskManifest = "bad" }} {
		p := preflight
		change(&p)
		if _, err := planNewSupersessionSeal(p, "next", "actor", "reason", now, "operation"); err == nil {
			t.Fatal("accepted invalid manifest")
		}
	}
}

func TestPlanNewSupersessionSealInvalidAttribution(t *testing.T) {
	old, _ := preflightEvidence(t)
	preflight := newSupersessionPreflightResult{Predecessor: old, TaskManifest: strings.Repeat("b", 64)}
	for _, tc := range []struct{ actor, reason string }{{"e\u0301", "reason"}, {"actor", "e\u0301"}, {"a\u200db", "reason"}, {"actor", "a\ue000b"}, {"\u2003", "reason"}, {"actor", "\u00a0"}, {strings.Repeat("a", 129), "reason"}, {"actor", strings.Repeat("a", 1025)}} {
		if _, err := planNewSupersessionSeal(preflight, "successor", tc.actor, tc.reason, time.Now(), "operation"); err == nil {
			t.Fatalf("accepted invalid attribution %q, %q", tc.actor, tc.reason)
		}
	}
	if _, err := planNewSupersessionSeal(preflight, "successor", "Élodie", "keep\u2003space", time.Now(), "operation"); err != nil {
		t.Fatalf("rejected exact valid Unicode: %v", err)
	}
}

func TestPlanNewSupersessionSealCreditedEvidence(t *testing.T) {
	old, batch := preflightEvidence(t)
	next := batch.Entries[0]
	next.EntryID = "entry-next"
	stream, err := applyprogress.StreamSHA256([]applyprogress.EvidenceEntry{batch.Entries[0], next})
	if err != nil {
		t.Fatal(err)
	}
	old.StreamSHA256, old.NextEntryIndex, old.NextEntryID = stream, 1, next.EntryID
	old, _, err = applyprogress.SealSnapshot(old)
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "revised"}, {ID: "1.2", Text: "remaining"}})
	if err != nil {
		t.Fatal(err)
	}
	req, err := planNewSupersessionSeal(newSupersessionPreflightResult{Predecessor: old, TaskManifest: manifest}, "successor", "Élodie", "Preserve evidence", time.Date(2025, 2, 3, 4, 5, 6, 7, time.UTC), "operation")
	if err != nil {
		t.Fatal(err)
	}
	seal := req.Snapshot
	if !applyprogress.IsSupersessionSeal(old, seal) || !reflect.DeepEqual(old.Batches, seal.Batches) || !reflect.DeepEqual(old.Coverage, seal.Coverage) || seal.StreamSHA256 != "" || seal.NextEntryIndex != 0 || seal.NextEntryID != "" || !reflect.DeepEqual(req.Batches, []applyprogress.Batch(nil)) || req.ExpectedGeneration != old.Generation || req.ExpectedRevision != old.Revision || req.ExpectedDigest != old.Digest || seal.PreviousDigest != old.Digest || seal.TaskManifestSHA256 != old.TaskManifestSHA256 || seal.SealIntent.SuccessorManifestSHA256 != manifest {
		t.Fatalf("credited evidence or continuation changed: %+v", req)
	}
	if old.StreamSHA256 != stream || old.NextEntryIndex != 1 || old.NextEntryID != next.EntryID {
		t.Fatal("mutated signed predecessor")
	}
	actor, reason := seal.SealIntent.Actor, seal.SealIntent.Reason
	for _, flags := range []struct{ actor, reason *string }{{nil, nil}, {&actor, nil}, {nil, &reason}, {&actor, &reason}} {
		retry, err := retrySupersessionSealRequest(seal, "successor", flags.actor, flags.reason)
		if err != nil || !reflect.DeepEqual(retry, req) {
			t.Fatalf("retry changed signed request: %+v, %v", retry, err)
		}
	}
	for _, flags := range []struct{ actor, reason *string }{{ptr("other"), nil}, {nil, ptr("other")}, {ptr("e\u0301"), nil}} {
		_, err := retrySupersessionSealRequest(seal, "successor", flags.actor, flags.reason)
		if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%q", actor)) || !strings.Contains(err.Error(), fmt.Sprintf("%q", reason)) || !strings.Contains(err.Error(), fmt.Sprintf("%q", "successor")) {
			t.Fatalf("mismatch did not quote SIGNED values: %v", err)
		}
	}
}

func ptr(value string) *string { return &value }

func TestPlanNewSupersessionSealCapacity(t *testing.T) {
	old, _ := preflightEvidence(t)
	// A signed near-ceiling head may fit while the added intent does not.
	var size int
	for i := 0; i < 500; i++ {
		candidate := old
		candidate.Batches = append(append([]applyprogress.BatchRef(nil), old.Batches...), applyprogress.BatchRef{BatchID: fmt.Sprintf("apb-%032x", i+2), SHA256: strings.Repeat("a", 64)})
		sealed, data, err := applyprogress.SealSnapshot(candidate)
		if err != nil {
			break
		}
		old, size = sealed, len(data)
	}
	if size < applyprogress.MaxDocumentRunes-500 {
		t.Fatalf("failed to construct near-ceiling signed head: %d", size)
	}
	req, err := planNewSupersessionSeal(newSupersessionPreflightResult{Predecessor: old, TaskManifest: strings.Repeat("b", 64)}, "successor", "actor", strings.Repeat("r", 1024), time.Now(), "operation")
	var capacity *applyprogress.CapacityError
	if req.RequestID != "" || !errors.As(err, &capacity) || capacity.Document != "snapshot" || capacity.Limit != applyprogress.MaxDocumentRunes || capacity.Runes <= capacity.Limit {
		t.Fatalf("expected explicit capacity error, got %+v, %v", req, err)
	}
}

func TestRetrySupersessionSealRequest(t *testing.T) {
	old, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "project", Change: "old", Generation: 1, Revision: 1, TaskManifestSHA256: strings.Repeat("a", 64), Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}})
	if err != nil {
		t.Fatal(err)
	}
	req, err := planNewSupersessionSeal(newSupersessionPreflightResult{Predecessor: old, TaskManifest: strings.Repeat("b", 64)}, "next", "actor", "reason", time.Now(), "operation")
	if err != nil {
		t.Fatal(err)
	}
	actor, reason := "actor", "reason"
	if _, err := retrySupersessionSealRequest(req.Snapshot, "next", &actor, &reason); err != nil {
		t.Fatal(err)
	}
	actor = "other"
	if _, err := retrySupersessionSealRequest(req.Snapshot, "next", &actor, nil); err == nil || !strings.Contains(err.Error(), `"actor"`) {
		t.Fatalf("missing signed actor: %v", err)
	}
	if _, err := retrySupersessionSealRequest(req.Snapshot, "wrong", nil, nil); err == nil {
		t.Fatal("accepted wrong target")
	}
	modified := req.Snapshot
	modified.SealIntent = new(applyprogress.SealIntent)
	*modified.SealIntent = *req.Snapshot.SealIntent
	modified.SealIntent.Reason = "substituted"
	if _, err := retrySupersessionSealRequest(modified, "next", nil, nil); err == nil {
		t.Fatal("accepted unsigned intent modification")
	}
	modified = req.Snapshot
	modified.Digest = strings.Repeat("0", 64)
	if _, err := retrySupersessionSealRequest(modified, "next", nil, nil); err == nil {
		t.Fatal("accepted unsigned seal modification")
	}
}

func TestValidateSupersessionAttribution(t *testing.T) {
	tests := []struct {
		name   string
		actor  string
		reason string
		valid  bool
	}{
		{name: "plain", actor: "operator", reason: "corrected seal", valid: true},
		{name: "NFC multibyte", actor: "Élodie 🦊", reason: "Révision nécessaire", valid: true},
		{name: "maximum rune lengths", actor: strings.Repeat("é", 128), reason: strings.Repeat("界", 1024), valid: true},
		{name: "empty actor", actor: "", reason: "reason"},
		{name: "empty reason", actor: "actor", reason: ""},
		{name: "whitespace actor", actor: " \t ", reason: "reason"},
		{name: "whitespace reason", actor: "actor", reason: " \n "},
		{name: "actor newline", actor: "a\nb", reason: "reason"},
		{name: "reason tab", actor: "actor", reason: "a\tb"},
		{name: "actor control", actor: "a\u0085b", reason: "reason"},
		{name: "reason control", actor: "actor", reason: "a\u202eb"},
		{name: "non-ASCII whitespace actor", actor: "\u2003", reason: "reason"},
		{name: "non-ASCII whitespace reason", actor: "actor", reason: "\u00a0"},
		{name: "format control actor", actor: "a\u200db", reason: "reason"},
		{name: "private use reason", actor: "actor", reason: "a\ue000b"},
		{name: "non-ASCII meaningful whitespace", actor: "actor", reason: "keep\u2003space", valid: true},
		{name: "decomposed actor", actor: "e\u0301", reason: "reason"},
		{name: "decomposed reason", actor: "actor", reason: "e\u0301"},
		{name: "invalid UTF8 actor", actor: "\xff", reason: "reason"},
		{name: "invalid UTF8 reason", actor: "actor", reason: "\xff"},
		{name: "oversize actor", actor: strings.Repeat("a", 129), reason: "reason"},
		{name: "oversize reason", actor: "actor", reason: strings.Repeat("a", 1025)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSupersessionAttribution(tt.actor, tt.reason)
			if (err == nil) != tt.valid {
				t.Fatalf("validateSupersessionAttribution() error = %v, want valid = %t", err, tt.valid)
			}
		})
	}
}
