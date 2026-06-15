package agent

import (
	"io/fs"
	"testing"

	jarvis "github.com/Thrasno/jarvis-ai-devs/jarvis-cli"
)

// TestAgentsFS_NotEmpty verifies that jarvis.AgentsFS contains all 7 expected
// named agent definition files under embed/agents/claude/.
func TestAgentsFS_NotEmpty(t *testing.T) {
	agentsFS, err := fs.Sub(jarvis.AgentsFS, "embed/agents/claude")
	if err != nil {
		t.Fatalf("sub AgentsFS to embed/agents/claude: %v", err)
	}

	expectedFiles := []string{
		"review-risk.md",
		"review-readability.md",
		"review-reliability.md",
		"review-resilience.md",
		"jd-judge-a.md",
		"jd-judge-b.md",
		"jd-fix-agent.md",
	}

	for _, name := range expectedFiles {
		info, err := fs.Stat(agentsFS, name)
		if err != nil {
			t.Errorf("expected agent file %q in AgentsFS: %v", name, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("agent file %q exists but is empty", name)
		}
	}
}
