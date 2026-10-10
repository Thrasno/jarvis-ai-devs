package sddspecsync

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// memStore is an in-memory Store that can inject write failures, corrupted
// writes, and concurrent edits.
type memStore struct {
	files     map[string][]byte
	writes    []string
	removes   []string
	failWrite map[string]bool
	corrupt   map[string]bool
	afterRead func(name string)
}

func newMemStore(files map[string]string) *memStore {
	s := &memStore{files: map[string][]byte{}, failWrite: map[string]bool{}, corrupt: map[string]bool{}}
	for name, data := range files {
		s.files[name] = []byte(data)
	}
	return s
}

func (s *memStore) ReadFile(name string) ([]byte, bool, error) {
	data, ok := s.files[name]
	if s.afterRead != nil {
		s.afterRead(name)
	}
	if !ok {
		return nil, false, nil
	}
	return bytes.Clone(data), true, nil
}

func (s *memStore) WriteFile(name string, data []byte) error {
	s.writes = append(s.writes, name)
	if s.failWrite[name] {
		return errors.New("injected write failure")
	}
	if s.corrupt[name] {
		data = append(bytes.Clone(data), "corrupt"...)
	}
	s.files[name] = bytes.Clone(data)
	return nil
}

func (s *memStore) Remove(name string) error {
	s.removes = append(s.removes, name)
	delete(s.files, name)
	return nil
}

func (s *memStore) snapshot() map[string]string {
	out := map[string]string{}
	for name, data := range s.files {
		out[name] = string(data)
	}
	return out
}

const (
	authPath    = testSpecsRoot + "/auth/spec.md"
	billingPath = testSpecsRoot + "/billing/spec.md"
	zetaPath    = testSpecsRoot + "/zeta/spec.md"
)

func applyFixture(t *testing.T) (Plan, map[string]string) {
	t.Helper()
	plan, err := BuildPlan(planFS(), testChangeRoot, testSpecsRoot)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	return plan, map[string]string{
		authPath:                             authMain(),
		testSpecsRoot + "/untouched/spec.md": "# Untouched\n",
	}
}

func TestApplyWritesEveryPlannedTarget(t *testing.T) {
	plan, files := applyFixture(t)
	store := newMemStore(files)

	result, err := Apply(plan, store)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(result.Written) != 3 {
		t.Fatalf("Written = %+v", result.Written)
	}
	for i, target := range plan.Targets {
		w := result.Written[i]
		if w.Path != target.MainPath || w.Digest != target.AfterDigest || w.BeforeDigest != target.BeforeDigest || w.Created == target.Existed {
			t.Fatalf("Written[%d] = %+v, target %s", i, w, target.MainPath)
		}
		if got := store.files[target.MainPath]; !bytes.Equal(got, target.After) {
			t.Fatalf("%s = %q, want %q", target.MainPath, got, target.After)
		}
	}
	if got := string(store.files[testSpecsRoot+"/untouched/spec.md"]); got != "# Untouched\n" {
		t.Fatalf("untouched spec changed: %q", got)
	}
}

func TestApplyStaleBeforeDigestAbortsWithoutWrites(t *testing.T) {
	tests := []struct {
		name  string
		stale func(map[string]string)
		path  string
	}{
		{name: "existing main changed", stale: func(f map[string]string) { f[authPath] += "edited\n" }, path: authPath},
		{name: "existing main deleted", stale: func(f map[string]string) { delete(f, authPath) }, path: authPath},
		{name: "new capability created concurrently", stale: func(f map[string]string) { f[zetaPath] = "# Zeta\n" }, path: zetaPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, files := applyFixture(t)
			tt.stale(files)
			store := newMemStore(files)
			before := store.snapshot()

			result, err := Apply(plan, store)
			if !errors.Is(err, ErrStale) {
				t.Fatalf("Apply() error = %v, want ErrStale", err)
			}
			var applyErr *ApplyError
			if !errors.As(err, &applyErr) || applyErr.Path != tt.path || applyErr.Code() != CodeConflictRecoveryRequired {
				t.Fatalf("Apply() error = %#v", err)
			}
			if len(store.writes) != 0 || len(store.removes) != 0 || len(result.Written) != 0 {
				t.Fatalf("Apply() wrote %v removed %v result %+v", store.writes, store.removes, result)
			}
			if !equalMaps(store.snapshot(), before) {
				t.Fatal("store changed after stale abort")
			}
		})
	}
}

func TestApplyRollsBackOnFailure(t *testing.T) {
	tests := []struct {
		name   string
		inject func(*memStore)
	}{
		{name: "write failure", inject: func(s *memStore) { s.failWrite[zetaPath] = true }},
		{name: "post-write digest mismatch", inject: func(s *memStore) { s.corrupt[zetaPath] = true }},
		{name: "concurrent edit before a later write", inject: func(s *memStore) {
			s.afterRead = func(name string) {
				if name == billingPath && len(s.writes) == 1 {
					s.files[zetaPath] = []byte("# Concurrent\n")
					s.afterRead = nil
				}
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, files := applyFixture(t)
			store := newMemStore(files)
			before := store.snapshot()
			tt.inject(store)

			_, err := Apply(plan, store)
			if err == nil {
				t.Fatal("Apply() error = nil")
			}
			if errors.Is(err, ErrRecoveryRequired) {
				t.Fatalf("Apply() rollback should verify, got %v", err)
			}
			after := store.snapshot()
			if tt.name == "concurrent edit before a later write" {
				if !errors.Is(err, ErrStale) {
					t.Fatalf("Apply() error = %v, want ErrStale", err)
				}
				before[zetaPath] = "# Concurrent\n"
			} else if !errors.Is(err, ErrWriteFailed) {
				t.Fatalf("Apply() error = %v, want ErrWriteFailed", err)
			}
			if !equalMaps(after, before) {
				t.Fatalf("store not rolled back\n got  %v\n want %v", after, before)
			}
		})
	}
}

func TestApplyRollbackRefusesToOverwriteConcurrentPostWriteEdit(t *testing.T) {
	plan, files := applyFixture(t)
	store := newMemStore(files)
	store.failWrite[zetaPath] = true
	store.afterRead = func(name string) {
		// Simulate another writer editing auth after Apply wrote it.
		if name == zetaPath && len(store.writes) == 2 {
			store.files[authPath] = []byte("# Someone else\n")
			store.afterRead = nil
		}
	}

	_, err := Apply(plan, store)
	if !errors.Is(err, ErrRecoveryRequired) || !errors.Is(err, ErrWriteFailed) {
		t.Fatalf("Apply() error = %v, want ErrRecoveryRequired and ErrWriteFailed", err)
	}
	var applyErr *ApplyError
	if !errors.As(err, &applyErr) || applyErr.Code() != CodeRecoveryRequired {
		t.Fatalf("Apply() error = %#v", err)
	}
	if len(applyErr.Recovery) != 1 || applyErr.Recovery[0].Path != authPath || applyErr.Recovery[0].ExpectedDigest != sha(authMain()) {
		t.Fatalf("Recovery = %+v", applyErr.Recovery)
	}
	if got := string(store.files[authPath]); got != "# Someone else\n" {
		t.Fatalf("rollback overwrote concurrent edit: %q", got)
	}
	if _, ok := store.files[billingPath]; ok {
		t.Fatal("created billing spec was not removed")
	}
}

func TestApplyRejectsTamperedPlan(t *testing.T) {
	plan, files := applyFixture(t)
	plan.Targets[0].After = append(bytes.Clone(plan.Targets[0].After), "x"...)
	store := newMemStore(files)
	if _, err := Apply(plan, store); !errors.Is(err, ErrInvalidPlan) || len(store.writes) != 0 {
		t.Fatalf("Apply() error = %v writes = %v", err, store.writes)
	}

	plan, _ = applyFixture(t)
	plan.Targets[1].MainPath = plan.Targets[0].MainPath
	if _, err := Apply(plan, newMemStore(files)); !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("Apply(duplicate path) error = %v", err)
	}
}

func TestOSStoreAppliesPlanOnDisk(t *testing.T) {
	root := t.TempDir()
	mainPath := filepath.Join(root, filepath.FromSlash(authPath))
	if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte(authMain()), 0o640); err != nil {
		t.Fatal(err)
	}
	plan, _ := applyFixture(t)

	if _, err := Apply(plan, OSStore{Root: root}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	for _, target := range plan.Targets {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(target.MainPath)))
		if err != nil || !bytes.Equal(got, target.After) {
			t.Fatalf("%s = %q, %v", target.MainPath, got, err)
		}
	}
	info, err := os.Stat(mainPath)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("existing mode not preserved: %v %v", info.Mode(), err)
	}
	info, err = os.Stat(filepath.Join(root, filepath.FromSlash(zetaPath)))
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("new spec mode = %v, %v", info.Mode(), err)
	}
}

func TestOSStoreRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	specs := filepath.Join(root, "openspec", "specs")
	if err := os.MkdirAll(specs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(specs, "auth")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	store := OSStore{Root: root}
	if _, _, err := store.ReadFile(authPath); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("ReadFile() error = %v, want ErrUnsafePath", err)
	}
	if err := store.WriteFile(authPath, []byte("x")); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("WriteFile() error = %v, want ErrUnsafePath", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("write escaped through symlink: %v", entries)
	}
	if _, _, err := store.ReadFile("../escape/spec.md"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("ReadFile(../) error = %v, want ErrUnsafePath", err)
	}
}

func equalMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}
