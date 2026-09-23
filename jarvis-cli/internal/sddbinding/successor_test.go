package sddbinding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

func mkdirSuccessorTestDirs(paths ...string) error {
	for _, path := range paths {
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
	}
	return nil
}
func hiveRequest(b Binding) hiveclient.SDDStoreBindingRequest {
	return hiveclient.SDDStoreBindingRequest{Mode: hiveclient.SDDStoreMode(b.Mode()), Provenance: b.Provenance()}
}

type successorHiveStore struct {
	entries map[string]*fakeHiveBindingStore
}

func (s *successorHiveStore) store(change string) *fakeHiveBindingStore {
	if s.entries[change] == nil {
		s.entries[change] = &fakeHiveBindingStore{}
	}
	return s.entries[change]
}
func (s *successorHiveStore) GetSDDStoreBinding(ctx context.Context, p, c string) (hiveclient.SDDStoreBinding, bool, error) {
	return s.store(c).GetSDDStoreBinding(ctx, p, c)
}
func (s *successorHiveStore) AdoptSDDStoreBinding(ctx context.Context, p, c string, r hiveclient.SDDStoreBindingRequest) (hiveclient.SDDStoreBinding, bool, error) {
	return s.store(c).AdoptSDDStoreBinding(ctx, p, c, r)
}

type successorLocalStore struct {
	entries map[string]*fakeOpenSpecBindingStore
}

func (s *successorLocalStore) store(path string) *fakeOpenSpecBindingStore {
	if s.entries[path] == nil {
		s.entries[path] = &fakeOpenSpecBindingStore{}
	}
	return s.entries[path]
}
func (s *successorLocalStore) ReadOpenSpec(path string) (*Binding, error) {
	return s.store(path).ReadOpenSpec(path)
}
func (s *successorLocalStore) AdoptOpenSpec(path string, b Binding) (Binding, bool, error) {
	return s.store(path).AdoptOpenSpec(path, b)
}

type successorSource struct {
	projection sddstatus.SupersessionProjection
	err        error
	calls      int
}

func (s *successorSource) FetchArtifacts(context.Context, string) (map[string]sddstatus.ArtifactState, map[string]string, error) {
	return nil, nil, nil
}
func (s *successorSource) ListChanges(context.Context) ([]string, error) { return nil, nil }
func (s *successorSource) ResolveSupersession(_ context.Context, _ string, _ applyprogress.Snapshot) (sddstatus.SupersessionProjection, error) {
	s.calls++
	return s.projection, s.err
}
func readySuccessorSource(genesis applyprogress.Snapshot) *successorSource {
	return &successorSource{projection: sddstatus.SupersessionProjection{State: sddstatus.SupersessionReady, Change: genesis.Change, Head: &genesis}}
}

func successorPair(t *testing.T) (applyprogress.Snapshot, applyprogress.Snapshot) {
	t.Helper()
	seal, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SupersessionSnapshotSchema, Project: "project", Change: "old", Generation: 1, Revision: 2, TaskManifestSHA256: strings.Repeat("a", 64), Status: applyprogress.StatusSuperseded, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}, SealIntent: &applyprogress.SealIntent{SuccessorProject: "project", SuccessorChange: "new", SuccessorManifestSHA256: strings.Repeat("b", 64), Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "request-1"}})
	if err != nil {
		t.Fatal(err)
	}
	pointer := &applyprogress.SupersedesPointer{Project: seal.Project, Change: seal.Change, SealDigest: seal.Digest, OriginalManifestSHA256: seal.TaskManifestSHA256, Actor: seal.SealIntent.Actor, Reason: seal.SealIntent.Reason, Timestamp: seal.SealIntent.Timestamp, OperationID: seal.SealIntent.OperationID}
	genesis, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SupersessionSnapshotSchema, Project: "project", Change: "new", Generation: 1, Revision: 1, TaskManifestSHA256: strings.Repeat("b", 64), Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{}, Supersedes: pointer})
	if err != nil {
		t.Fatal(err)
	}
	return seal, genesis
}

func TestAdoptSuccessorGenesisRejectsFakeReadyAuthority(t *testing.T) {
	seal, genesis := successorPair(t)
	workspace := t.TempDir()
	old := filepath.Join(workspace, "openspec", "changes", "old")
	target := filepath.Join(workspace, "openspec", "changes", "new")
	if err := mkdirSuccessorTestDirs(old, target); err != nil {
		t.Fatal(err)
	}
	h := &successorHiveStore{entries: map[string]*fakeHiveBindingStore{}}
	l := &successorLocalStore{entries: map[string]*fakeOpenSpecBindingStore{}}
	b, _ := New(sddruntime.StoreModeHive, "prior")
	h.AdoptSDDStoreBinding(context.Background(), "project", "old", hiveRequest(b))
	source := readySuccessorSource(genesis)
	_, err := (LegacyResolver{HiveBindings: h, OpenSpecBindings: l, HiveSource: source, OpenSpecChangeDir: target}).AdoptSuccessorGenesis(context.Background(), "project", "old", seal, genesis)
	if err == nil || len(h.store("new").adopted) != 0 || source.calls != 0 {
		t.Fatalf("fake authority accepted: err=%v adoptions=%d calls=%d", err, len(h.store("new").adopted), source.calls)
	}
}

func TestAdoptSuccessorGenesisRejectsBeforeAdoption(t *testing.T) {
	seal, genesis := successorPair(t)
	workspace := t.TempDir()
	old := filepath.Join(workspace, "openspec", "changes", "old")
	target := filepath.Join(workspace, "openspec", "changes", "new")
	if err := mkdirSuccessorTestDirs(old, target); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		setup       func(*successorHiveStore, *successorLocalStore)
		seal        applyprogress.Snapshot
		genesis     applyprogress.Snapshot
		target      string
		hiveSource  *successorSource
		localSource *successorSource
	}{
		{name: "missing predecessor", seal: seal, genesis: genesis, target: target},
		{name: "fabricated pair without published stores", seal: seal, genesis: genesis, target: target, setup: func(h *successorHiveStore, _ *successorLocalStore) {
			b, _ := New(sddruntime.StoreModeHive, "prior")
			h.AdoptSDDStoreBinding(context.Background(), "project", "old", hiveRequest(b))
		}},
		{name: "pending publication", seal: seal, genesis: genesis, target: target, hiveSource: &successorSource{projection: sddstatus.SupersessionProjection{State: sddstatus.SupersessionPending}}, setup: func(h *successorHiveStore, _ *successorLocalStore) {
			b, _ := New(sddruntime.StoreModeHive, "prior")
			h.AdoptSDDStoreBinding(context.Background(), "project", "old", hiveRequest(b))
		}},
		{name: "foreign authority", seal: seal, genesis: genesis, target: target, hiveSource: &successorSource{err: errors.New("foreign seal")}, setup: func(h *successorHiveStore, _ *successorLocalStore) {
			b, _ := New(sddruntime.StoreModeHive, "prior")
			h.AdoptSDDStoreBinding(context.Background(), "project", "old", hiveRequest(b))
		}},
		{name: "head mismatch", seal: seal, genesis: genesis, target: target, hiveSource: readySuccessorSource(seal), setup: func(h *successorHiveStore, _ *successorLocalStore) {
			b, _ := New(sddruntime.StoreModeHive, "prior")
			h.AdoptSDDStoreBinding(context.Background(), "project", "old", hiveRequest(b))
		}},
		{name: "hybrid local pending", seal: seal, genesis: genesis, target: target, hiveSource: readySuccessorSource(genesis), localSource: &successorSource{projection: sddstatus.SupersessionProjection{State: sddstatus.SupersessionPending}}, setup: func(h *successorHiveStore, l *successorLocalStore) {
			b, _ := New(sddruntime.StoreModeHybrid, "prior")
			h.AdoptSDDStoreBinding(context.Background(), "project", "old", hiveRequest(b))
			l.AdoptOpenSpec(old, b)
		}},
		{name: "hybrid missing local source", seal: seal, genesis: genesis, target: target, hiveSource: readySuccessorSource(genesis), setup: func(h *successorHiveStore, l *successorLocalStore) {
			b, _ := New(sddruntime.StoreModeHybrid, "prior")
			h.AdoptSDDStoreBinding(context.Background(), "project", "old", hiveRequest(b))
			l.AdoptOpenSpec(old, b)
		}},
		{name: "foreign local binding", seal: seal, genesis: genesis, target: target, localSource: readySuccessorSource(genesis), setup: func(_ *successorHiveStore, l *successorLocalStore) {
			b, _ := New(sddruntime.StoreModeOpenSpec, "prior")
			l.AdoptOpenSpec(old, b)
			foreign, _ := New(sddruntime.StoreModeOpenSpec, "foreign")
			l.AdoptOpenSpec(target, foreign)
		}},
		{name: "invalid pair", seal: seal, genesis: applyprogress.Snapshot{}, target: target},
		{name: "wrong target", seal: seal, genesis: genesis, target: old},
		{name: "foreign binding", seal: seal, genesis: genesis, target: target, setup: func(h *successorHiveStore, l *successorLocalStore) {
			b, _ := New(sddruntime.StoreModeHive, "prior")
			h.AdoptSDDStoreBinding(context.Background(), "project", "old", hiveRequest(b))
			h.AdoptSDDStoreBinding(context.Background(), "project", "new", hiveclient.SDDStoreBindingRequest{Mode: hiveclient.SDDStoreModeHive, Provenance: "foreign"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &successorHiveStore{entries: map[string]*fakeHiveBindingStore{}}
			l := &successorLocalStore{entries: map[string]*fakeOpenSpecBindingStore{}}
			if tc.setup != nil {
				tc.setup(h, l)
			}
			preexisting := len(h.store("new").adopted)
			preexistingLocal := len(l.store(target).adopted)
			_, err := (LegacyResolver{HiveBindings: h, OpenSpecBindings: l, HiveSource: tc.hiveSource, OpenSpecSource: tc.localSource, OpenSpecChangeDir: tc.target}).AdoptSuccessorGenesis(context.Background(), "project", "old", tc.seal, tc.genesis)
			if err == nil {
				t.Fatal("expected rejection")
			}
			if len(h.store("new").adopted) != preexisting || len(l.store(target).adopted) != preexistingLocal {
				t.Fatalf("unexpected adoption: %v", err)
			}
		})
	}
}

func realOpenSpecSuccessor(t *testing.T, root string) (applyprogress.Snapshot, applyprogress.Snapshot) {
	t.Helper()
	old := filepath.Join(root, "openspec", "changes", "old")
	if err := mkdirSuccessorTestDirs(old); err != nil {
		t.Fatal(err)
	}
	original := "- [ ] 1.1 original\n"
	if err := os.WriteFile(filepath.Join(old, "tasks.md"), []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	_, manifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "original"}})
	if err != nil {
		t.Fatal(err)
	}
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "project", Change: "old", BatchID: "apb-00000000000000000000000000000001", Entries: []applyprogress.EvidenceEntry{{EntryID: "evidence", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
	initial, _, err := applyprogress.SealSnapshot(applyprogress.Snapshot{Schema: applyprogress.SnapshotSchema, Project: "project", Change: "old", Generation: 1, Revision: 1, TaskManifestSHA256: manifest, Status: applyprogress.StatusPartial, Coverage: []applyprogress.Coverage{}, Batches: []applyprogress.BatchRef{{BatchID: batch.BatchID, SHA256: batch.SHA256}}})
	if err != nil {
		t.Fatal(err)
	}
	store := sddprogress.OpenSpec{Root: old}
	if _, err := store.Advance(sddprogress.AdvanceRequest{RequestID: "initial-request", Snapshot: initial, Batches: []applyprogress.Batch{batch}}); err != nil {
		t.Fatal(err)
	}
	revised := "- [ ] 1.1 revised\n"
	if err := os.WriteFile(filepath.Join(old, "tasks.md"), []byte(revised), 0600); err != nil {
		t.Fatal(err)
	}
	_, revisedManifest, err := applyprogress.TaskManifest([]applyprogress.Task{{ID: "1.1", Text: "revised"}})
	if err != nil {
		t.Fatal(err)
	}
	seal := initial
	seal.Schema = applyprogress.SupersessionSnapshotSchema
	seal.Revision++
	seal.PreviousDigest = initial.Digest
	seal.Status = applyprogress.StatusSuperseded
	seal.SealIntent = &applyprogress.SealIntent{SuccessorProject: "project", SuccessorChange: "new", SuccessorManifestSHA256: revisedManifest, Actor: "agent", Reason: "replanned", Timestamp: "2026-01-01T00:00:00Z", OperationID: "seal-request"}
	seal, _, err = applyprogress.SealSnapshot(seal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Advance(sddprogress.AdvanceRequest{RequestID: "seal-request", ExpectedGeneration: initial.Generation, ExpectedRevision: initial.Revision, ExpectedDigest: initial.Digest, Snapshot: seal}); err != nil {
		t.Fatal(err)
	}
	target := sddprogress.OpenSpec{Root: filepath.Join(root, "openspec", "changes", "new")}
	if err := mkdirSuccessorTestDirs(target.Root); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{".apply-progress-successor-seal-digest": seal.Digest, "tasks.md": revised} {
		if err := os.WriteFile(filepath.Join(target.Root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, err = store.PublishSuccessorGenesis(target)
	if err != nil {
		t.Fatal(err)
	}
	genesis, err := target.InspectPublication()
	if err != nil || genesis == nil {
		t.Fatalf("published genesis: %v", err)
	}
	return seal, *genesis
}

func realHiveSuccessor(t *testing.T, seal, genesis applyprogress.Snapshot) *sddstatus.HiveSource {
	t.Helper()
	revised := "- [ ] 1.1 revised\n"
	batch, _, err := applyprogress.SealBatch(applyprogress.Batch{Schema: applyprogress.EvidenceSchema, Project: "project", Change: "old", BatchID: "apb-00000000000000000000000000000001", Entries: []applyprogress.EvidenceEntry{{EntryID: "evidence", TaskIDs: []string{}, CompletesTaskIDs: []string{}, Kind: applyprogress.EvidenceGreen, Summary: "green", Command: "go test", Outcome: applyprogress.OutcomePass, Files: []string{}}}})
	if err != nil {
		t.Fatal(err)
	}
	oldData, _ := json.Marshal(seal)
	genesisData, _ := json.Marshal(genesis)
	batches, _ := json.Marshal([]applyprogress.Batch{batch})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sdd/changes/old/artifacts", "/sdd/changes/new/artifacts":
			fmt.Fprintf(w, `{"artifacts":[{"artifact":"tasks","content":%q}]}`, revised)
		case "/sdd/changes/old/apply-progress":
			fmt.Fprintf(w, `{"outcome":"current","state":{"snapshot":%s,"batches":%s}}`, oldData, batches)
		case "/sdd/changes/new/apply-progress":
			fmt.Fprintf(w, `{"outcome":"current","state":{"snapshot":%s,"batches":[]}}`, genesisData)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	client, err := hiveclient.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return sddstatus.NewHiveSource(client, "project")
}

func TestAdoptSuccessorGenesisConcreteSourceMissingPublication(t *testing.T) {
	seal, genesis := successorPair(t)
	root := t.TempDir()
	old := filepath.Join(root, "openspec", "changes", "old")
	target := filepath.Join(root, "openspec", "changes", "new")
	if err := mkdirSuccessorTestDirs(old, target); err != nil {
		t.Fatal(err)
	}
	h := &successorHiveStore{entries: map[string]*fakeHiveBindingStore{}}
	l := &successorLocalStore{entries: map[string]*fakeOpenSpecBindingStore{}}
	b, _ := New(sddruntime.StoreModeOpenSpec, "prior")
	l.AdoptOpenSpec(old, b)
	_, err := (LegacyResolver{HiveBindings: h, OpenSpecBindings: l, OpenSpecChangeDir: target, OpenSpecSource: sddstatus.NewOpenSpecSourceForProject(root, "project")}).AdoptSuccessorGenesis(context.Background(), "project", "old", seal, genesis)
	if err == nil || len(l.store(target).adopted) != 0 {
		t.Fatalf("fabricated genesis accepted: %v, adoptions %d", err, len(l.store(target).adopted))
	}
}

func TestAdoptSuccessorGenesisRealOpenSpec(t *testing.T) {
	root := t.TempDir()
	seal, genesis := realOpenSpecSuccessor(t, root)
	old := filepath.Join(root, "openspec", "changes", "old")
	target := filepath.Join(root, "openspec", "changes", "new")
	h := &successorHiveStore{entries: map[string]*fakeHiveBindingStore{}}
	l := &successorLocalStore{entries: map[string]*fakeOpenSpecBindingStore{}}
	b, _ := New(sddruntime.StoreModeOpenSpec, "prior")
	l.AdoptOpenSpec(old, b)
	r := LegacyResolver{HiveBindings: h, OpenSpecBindings: l, OpenSpecChangeDir: target, OpenSpecSource: sddstatus.NewOpenSpecSourceForProject(root, "project")}
	for i := 0; i < 2; i++ {
		got, err := r.AdoptSuccessorGenesis(context.Background(), "project", "old", seal, genesis)
		if err != nil || got.Mode != sddruntime.StoreModeOpenSpec {
			t.Fatalf("attempt %d: %+v %v", i, got, err)
		}
	}
}

func TestAdoptSuccessorGenesisRejectsForeignLocalCopyWithRealSource(t *testing.T) {
	root := t.TempDir()
	seal, genesis := realOpenSpecSuccessor(t, root)
	old := filepath.Join(root, "openspec", "changes", "old")
	target := filepath.Join(root, "openspec", "changes", "new")
	h := &successorHiveStore{entries: map[string]*fakeHiveBindingStore{}}
	l := &successorLocalStore{entries: map[string]*fakeOpenSpecBindingStore{}}
	b, _ := New(sddruntime.StoreModeOpenSpec, "prior")
	l.AdoptOpenSpec(old, b)
	foreign, _ := New(sddruntime.StoreModeOpenSpec, "foreign")
	l.AdoptOpenSpec(target, foreign)
	before := len(l.store(target).adopted)
	_, err := (LegacyResolver{HiveBindings: h, OpenSpecBindings: l, OpenSpecChangeDir: target, OpenSpecSource: sddstatus.NewOpenSpecSourceForProject(root, "project")}).AdoptSuccessorGenesis(context.Background(), "project", "old", seal, genesis)
	if err == nil || len(l.store(target).adopted) != before {
		t.Fatalf("foreign copy accepted: %v", err)
	}
}

func TestAdoptSuccessorGenesisPartialHybridRepair(t *testing.T) {
	workspace := t.TempDir()
	seal, genesis := realOpenSpecSuccessor(t, workspace)
	old := filepath.Join(workspace, "openspec", "changes", "old")
	target := filepath.Join(workspace, "openspec", "changes", "new")
	if err := mkdirSuccessorTestDirs(old, target); err != nil {
		t.Fatal(err)
	}
	h := &successorHiveStore{entries: map[string]*fakeHiveBindingStore{}}
	l := &successorLocalStore{entries: map[string]*fakeOpenSpecBindingStore{}}
	b, _ := New(sddruntime.StoreModeHybrid, "prior")
	h.AdoptSDDStoreBinding(context.Background(), "project", "old", hiveRequest(b))
	l.AdoptOpenSpec(old, b)
	l.store(target).adoptErr = errors.New("temporary failure")
	resolver := LegacyResolver{HiveBindings: h, OpenSpecBindings: l, HiveSource: realHiveSuccessor(t, seal, genesis), OpenSpecSource: sddstatus.NewOpenSpecSourceForProject(workspace, "project"), OpenSpecChangeDir: target}
	_, err := resolver.AdoptSuccessorGenesis(context.Background(), "project", "old", seal, genesis)
	var partial *PartialAdoptionError
	if !errors.As(err, &partial) {
		t.Fatalf("error = %v", err)
	}
	l.store(target).adoptErr = nil
	got, err := resolver.AdoptSuccessorGenesis(context.Background(), "project", "old", seal, genesis)
	if err != nil || got.Mode != sddruntime.StoreModeHybrid {
		t.Fatalf("repair = %+v, %v", got, err)
	}
}

func TestAdoptSuccessorGenesisHiveWithoutLocalDirectories(t *testing.T) {
	seal, genesis := realOpenSpecSuccessor(t, t.TempDir())
	workspace := t.TempDir()
	target := filepath.Join(workspace, "openspec", "changes", "new")
	hive := &successorHiveStore{entries: map[string]*fakeHiveBindingStore{}}
	prior, err := New(sddruntime.StoreModeHive, "original")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := hive.AdoptSDDStoreBinding(context.Background(), "project", "old", hiveRequest(prior)); err != nil {
		t.Fatal(err)
	}
	resolver := LegacyResolver{HiveBindings: hive, HiveSource: realHiveSuccessor(t, seal, genesis), OpenSpecChangeDir: target}
	got, err := resolver.AdoptSuccessorGenesis(context.Background(), "project", "old", seal, genesis)
	if err != nil || got.Mode != sddruntime.StoreModeHive || !got.Persisted {
		t.Fatalf("Hive-only adoption = %+v, %v", got, err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target directory was created: %v", err)
	}
}

func TestAdoptSuccessorGenesis(t *testing.T) {
	for _, mode := range []sddruntime.StoreMode{sddruntime.StoreModeHive, sddruntime.StoreModeOpenSpec, sddruntime.StoreModeHybrid} {
		t.Run(string(mode), func(t *testing.T) {
			workspace := t.TempDir()
			seal, genesis := realOpenSpecSuccessor(t, workspace)
			old := filepath.Join(workspace, "openspec", "changes", "old")
			target := filepath.Join(workspace, "openspec", "changes", "new")
			if err := mkdirSuccessorTestDirs(old, target); err != nil {
				t.Fatal(err)
			}
			hive := &successorHiveStore{entries: map[string]*fakeHiveBindingStore{}}
			local := &successorLocalStore{entries: map[string]*fakeOpenSpecBindingStore{}}
			binding, err := New(mode, "original")
			if err != nil {
				t.Fatal(err)
			}
			if mode != sddruntime.StoreModeOpenSpec {
				_, _, err = hive.AdoptSDDStoreBinding(context.Background(), "project", "old", hiveRequest(binding))
				if err != nil {
					t.Fatal(err)
				}
			}
			if mode != sddruntime.StoreModeHive {
				_, _, err = local.AdoptOpenSpec(old, binding)
				if err != nil {
					t.Fatal(err)
				}
			}
			hive.store("old").adopted = nil
			local.store(old).adopted = nil
			resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: local, HiveSource: realHiveSuccessor(t, seal, genesis), OpenSpecSource: sddstatus.NewOpenSpecSourceForProject(workspace, "project"), OpenSpecChangeDir: target}
			// The predecessor store is addressed by the same resolver with its old coordinate.
			resolution, err := resolver.AdoptSuccessorGenesis(context.Background(), "project", "old", seal, genesis)
			if err != nil {
				t.Fatal(err)
			}
			if resolution.Mode != mode || !resolution.Persisted || !strings.HasPrefix(resolution.Provenance, "supersession:"+seal.Digest+":") {
				t.Fatalf("resolution = %+v", resolution)
			}
			if _, err := resolver.AdoptSuccessorGenesis(context.Background(), "project", "old", seal, genesis); err != nil {
				t.Fatalf("replay: %v", err)
			}
		})
	}
}
