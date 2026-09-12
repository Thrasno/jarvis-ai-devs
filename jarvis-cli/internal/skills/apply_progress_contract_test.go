package skills

import (
	"strings"
	"testing"
)

func TestCatalogContract_ApplyProgressLifecycleGuidanceIsShared(t *testing.T) {
	const sharedPath = "embed/skills/_shared/apply-progress.md"
	content := readEmbeddedSkillAsset(t, sharedPath)
	for _, snippet := range []string{
		"Continuation Lifecycle",
		"stream_sha256",
		"next_entry_index",
		"next_entry_id",
		"atomic `stream_sha256`, `next_entry_index`, and `next_entry_id` continuation group",
		"advances the cursor",
		"ordered references",
		"atomic continuation group",
		"never repairs or recomputes evidence",
	} {
		if !strings.Contains(content, snippet) {
			t.Fatalf("expected %s to contain apply-progress guidance %q", sharedPath, snippet)
		}
	}

	for path, reference := range map[string]string{
		"embed/skills/sdd-apply/SKILL.md":   "skills/_shared/apply-progress.md",
		"embed/skills/sdd-archive/SKILL.md": "skills/_shared/apply-progress.md",
		"embed/skills/sdd-verify/SKILL.md":  "../_shared/apply-progress.md",
	} {
		if !strings.Contains(readEmbeddedSkillAsset(t, path), reference) {
			t.Fatalf("expected %s to reference shared apply-progress guidance %q", path, reference)
		}
	}
}
