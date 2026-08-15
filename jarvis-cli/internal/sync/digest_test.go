package sync

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedAssetDigestUsesStableIdentityIndependentOfRoot(t *testing.T) {
	asset := func(root string) TrackedPath {
		return TrackedPath{Agent: "claude", Path: filepath.Join(root, ".claude", "settings.json"), Identity: ".claude/settings.json", Mode: ManagedFileMode, Semantic: &ManagedJSON{Fragments: map[string]any{"outputStyle": "Neutra"}}}
	}
	first := ManagedAssetDigest(Plan{Tracked: []TrackedPath{asset("/home/one")}})
	second := ManagedAssetDigest(Plan{Tracked: []TrackedPath{asset("/different/root")}})
	if first != second {
		t.Fatalf("digest depends on ReplayInput.Root: %q != %q", first, second)
	}
	if !strings.HasPrefix(first, managedAssetDigestVersion+":") {
		t.Fatalf("digest %q is not versioned", first)
	}
}

func TestManagedAssetDigestIsDeterministicFramedAndSensitive(t *testing.T) {
	base := TrackedPath{Agent: "claude", Identity: ".claude/style.md", Path: "/ignored/style.md", Mode: ManagedFileMode, Desired: "content", Semantic: &ManagedJSON{Fragments: map[string]any{"alpha": true, "outputStyle": "Neutra"}}}
	other := TrackedPath{Agent: "claude", Identity: ".claude/hook.sh", Path: "/ignored/hook.sh", Mode: ManagedExecutableMode, Desired: "script"}
	baseline := ManagedAssetDigest(Plan{Tracked: []TrackedPath{base, other}})

	reorderedSemantic := base
	reorderedSemantic.Semantic = &ManagedJSON{Fragments: map[string]any{"outputStyle": "Neutra", "alpha": true}}
	if got := ManagedAssetDigest(Plan{Tracked: []TrackedPath{other, reorderedSemantic}}); got != baseline {
		t.Fatalf("digest is unstable across asset or semantic-map order: %q != %q", got, baseline)
	}

	tests := []struct {
		name   string
		change func(*TrackedPath)
	}{
		{name: "mode", change: func(v *TrackedPath) { v.Mode = ManagedExecutableMode }},
		{name: "content", change: func(v *TrackedPath) { v.Desired = "different" }},
		{name: "path identity", change: func(v *TrackedPath) { v.Identity = ".claude/other-style.md" }},
		{name: "semantic fragment", change: func(v *TrackedPath) {
			v.Semantic = &ManagedJSON{Fragments: map[string]any{"alpha": true, "outputStyle": "Argentino"}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := base
			tt.change(&changed)
			if got := ManagedAssetDigest(Plan{Tracked: []TrackedPath{changed, other}}); got == baseline {
				t.Fatalf("digest ignored changed %s", tt.name)
			}
		})
	}

	left := TrackedPath{Agent: "ab", Identity: "c", Mode: ManagedFileMode}
	right := TrackedPath{Agent: "a", Identity: "bc", Mode: ManagedFileMode}
	if ManagedAssetDigest(Plan{Tracked: []TrackedPath{left}}) == ManagedAssetDigest(Plan{Tracked: []TrackedPath{right}}) {
		t.Fatal("length framing did not distinguish adjacent field boundaries")
	}
}
