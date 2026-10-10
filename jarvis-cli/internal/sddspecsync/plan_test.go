package sddspecsync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"testing/fstest"
)

const (
	testChangeRoot = "openspec/changes/lean-flow"
	testSpecsRoot  = "openspec/specs"
)

func sha(data string) string {
	sum := sha256.Sum256([]byte(data))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func newCapabilitySpec(title string) string {
	return "# " + title + " Specification\n\n## Requirements\n\n### Requirement: " + title + "\n\nThe system MUST " + title + ".\n"
}

func planFS() fstest.MapFS {
	return fstest.MapFS{
		testSpecsRoot + "/auth/spec.md":           {Data: []byte(authMain())},
		testSpecsRoot + "/untouched/spec.md":      {Data: []byte("# Untouched\n")},
		testChangeRoot + "/specs/zeta/spec.md":    {Data: []byte(newCapabilitySpec("Zeta"))},
		testChangeRoot + "/specs/auth/spec.md":    {Data: []byte(delta("## ADDED Requirements\n\n" + reqLogout))},
		testChangeRoot + "/specs/billing/spec.md": {Data: []byte(newCapabilitySpec("Billing"))},
		testChangeRoot + "/specs/auth/notes.txt":  {Data: []byte("ignored")},
	}
}

func TestBuildPlanComputesSortedTargetsWithDigests(t *testing.T) {
	plan, err := BuildPlan(planFS(), testChangeRoot, testSpecsRoot)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	wantAuth := mainHead + reqLogin + "\n" + reqExpiry + "\n" + reqLegacy + "\n" + reqLogout + mainTail
	want := []struct {
		capability, mainPath, deltaPath string
		existed                         bool
		beforeDigest, after             string
	}{
		{"auth", testSpecsRoot + "/auth/spec.md", testChangeRoot + "/specs/auth/spec.md", true, sha(authMain()), wantAuth},
		{"billing", testSpecsRoot + "/billing/spec.md", testChangeRoot + "/specs/billing/spec.md", false, AbsentDigest, newCapabilitySpec("Billing")},
		{"zeta", testSpecsRoot + "/zeta/spec.md", testChangeRoot + "/specs/zeta/spec.md", false, AbsentDigest, newCapabilitySpec("Zeta")},
	}
	if len(plan.Targets) != len(want) {
		t.Fatalf("len(Targets) = %d, want %d", len(plan.Targets), len(want))
	}
	for i, w := range want {
		got := plan.Targets[i]
		if got.Capability != w.capability || got.MainPath != w.mainPath || got.DeltaPath != w.deltaPath || got.Existed != w.existed {
			t.Fatalf("Targets[%d] = %+v, want %+v", i, got, w)
		}
		if got.BeforeDigest != w.beforeDigest || string(got.After) != w.after || got.AfterDigest != sha(w.after) {
			t.Fatalf("Targets[%d] digests/after = %q %q %q", i, got.BeforeDigest, got.AfterDigest, got.After)
		}
		if w.existed && string(got.Before) != authMain() {
			t.Fatalf("Targets[%d].Before = %q", i, got.Before)
		}
		if !w.existed && got.Before != nil {
			t.Fatalf("Targets[%d].Before = %q, want nil", i, got.Before)
		}
	}
	if !plan.Targets[1].Changes.NewCapability || plan.Targets[0].Changes.NewCapability {
		t.Fatalf("NewCapability flags = %v %v", plan.Targets[0].Changes.NewCapability, plan.Targets[1].Changes.NewCapability)
	}

	again, err := BuildPlan(planFS(), testChangeRoot, testSpecsRoot)
	if err != nil {
		t.Fatalf("BuildPlan() second run error = %v", err)
	}
	for i := range plan.Targets {
		if plan.Targets[i].AfterDigest != again.Targets[i].AfterDigest {
			t.Fatalf("BuildPlan() is not deterministic at %d", i)
		}
	}
}

func TestBuildPlanFailsClosedOnAnyCapability(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(fstest.MapFS)
		wantErr error
		wantCap string
	}{
		{
			name: "one invalid capability blocks the whole plan",
			mutate: func(fsys fstest.MapFS) {
				fsys[testChangeRoot+"/specs/auth/spec.md"] = &fstest.MapFile{Data: []byte(delta("## MODIFIED Requirements\n\n### Requirement: Ghost\n\nText.\n"))}
			},
			wantErr: ErrRequirementNotFound,
			wantCap: "auth",
		},
		{
			name: "capability directory without spec.md",
			mutate: func(fsys fstest.MapFS) {
				fsys[testChangeRoot+"/specs/empty/README.md"] = &fstest.MapFile{Data: []byte("x")}
			},
			wantErr: ErrInvalidDelta,
			wantCap: "empty",
		},
		{
			name: "file directly under specs",
			mutate: func(fsys fstest.MapFS) {
				fsys[testChangeRoot+"/specs/stray.md"] = &fstest.MapFile{Data: []byte("x")}
			},
			wantErr: ErrInvalidDelta,
			wantCap: "stray.md",
		},
		{
			name: "hidden capability directory",
			mutate: func(fsys fstest.MapFS) {
				fsys[testChangeRoot+"/specs/.hidden/spec.md"] = &fstest.MapFile{Data: []byte(newCapabilitySpec("Hidden"))}
			},
			wantErr: ErrInvalidDelta,
			wantCap: ".hidden",
		},
		{
			name: "blank delta spec",
			mutate: func(fsys fstest.MapFS) {
				fsys[testChangeRoot+"/specs/zeta/spec.md"] = &fstest.MapFile{Data: []byte("\n \n")}
			},
			wantErr: ErrInvalidDelta,
			wantCap: "zeta",
		},
		{
			name: "no delta specs",
			mutate: func(fsys fstest.MapFS) {
				for name := range fsys {
					delete(fsys, name)
				}
				fsys[testChangeRoot+"/proposal.md"] = &fstest.MapFile{Data: []byte("x")}
			},
			wantErr: ErrNoDeltaSpecs,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := planFS()
			tt.mutate(fsys)
			plan, err := BuildPlan(fsys, testChangeRoot, testSpecsRoot)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("BuildPlan() error = %v, want %v", err, tt.wantErr)
			}
			if len(plan.Targets) != 0 {
				t.Fatalf("BuildPlan() returned %d targets on error", len(plan.Targets))
			}
			if tt.wantCap != "" {
				var mergeErr *MergeError
				if !errors.As(err, &mergeErr) || mergeErr.Capability != tt.wantCap {
					t.Fatalf("BuildPlan() error = %#v, want capability %q", err, tt.wantCap)
				}
			}
		})
	}
}
