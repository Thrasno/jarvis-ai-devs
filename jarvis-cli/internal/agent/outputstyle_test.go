package agent

import (
	"testing"

	"github.com/Thrasno/jarvis-dev/jarvis-cli/internal/persona"
)

// TestAgentInterfaceExtension verifies that the Agent interface includes
// the output-style methods required by SPEC-001.
func TestAgentInterfaceExtension(t *testing.T) {
	// This test ensures the interface has been extended correctly.
	// It will fail to compile if the methods don't exist on the interface.

	var _ interface {
		SupportsOutputStyles() bool
		WriteOutputStyle(*persona.Preset) error
	} = (Agent)(nil)

	// If we reach here, the interface has the required methods
	t.Log("Agent interface has SupportsOutputStyles and WriteOutputStyle methods")
}
