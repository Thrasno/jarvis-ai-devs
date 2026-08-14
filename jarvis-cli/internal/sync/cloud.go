// This file holds the one thing replay has to say about the cloud half of an
// installation, which is a sentence rather than an action.
//
// Sync is machine-scoped: it replays local artifacts. A manifest recording
// local+cloud scope therefore describes something sync only partly covers, and
// the honest response is to name the command that covers the rest rather than
// to abort the half that can be replayed. ~/.jarvis/sync.json is read-only
// here: it is never written, never created and never repaired, because
// credentials belong to `jarvis login`.
package sync

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// CloudManualActionMessage is the report line for a cloud portion sync cannot
// carry out itself.
const CloudManualActionMessage = "Cloud scope is recorded but ~/.jarvis/sync.json is missing or unreadable: " +
	"run `jarvis login` to restore the cloud portion. The local replay continues."

// CloudManualAction reports the manual step the cloud portion needs, or an
// empty string when there is nothing to report. It reads at most one file and
// writes nothing at all, so a caller may consult it before the backup is taken.
func CloudManualAction(home string, scope state.Scope) string {
	if scope != state.ScopeLocalCloud {
		return ""
	}
	credentials, err := os.ReadFile(filepath.Join(home, ".jarvis", "sync.json"))
	if err != nil {
		return CloudManualActionMessage
	}
	fields := map[string]any{}
	if json.Unmarshal(credentials, &fields) != nil {
		return CloudManualActionMessage
	}
	return ""
}
