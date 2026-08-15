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
	"os"
	"path/filepath"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/config"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/state"
)

// CloudManualActionMessage is the report line for a cloud portion sync cannot
// carry out itself.
const CloudManualActionMessage = "Cloud scope is recorded but ~/.jarvis/sync.json is missing, unreadable or incomplete: " +
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
	// Parseability is not the question. A truncated or emptied sync.json — the
	// shape a half-finished `jarvis login` leaves behind — still decodes, and
	// `null` decodes without an error at all, so a syntax check would call an
	// unusable cloud portion fine and never name the command that repairs it.
	// The judgement belongs to the writer's own contract, so ask it directly.
	if !config.SyncCredentialsComplete(credentials) {
		return CloudManualActionMessage
	}
	return ""
}
