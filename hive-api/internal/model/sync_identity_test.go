package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateSyncProjectIdentityLeavesProjectsVerbatim proves the API never
// rewrites a project spelling it is given. The daemon is the sole authority on
// project identity; the API stores the literal it receives.
func TestValidateSyncProjectIdentityLeavesProjectsVerbatim(t *testing.T) {
	req := SyncRequest{
		Project:                " Jarvis.Dev ",
		ProjectIdentityVersion: "",
		Sessions:               []SyncSessionPayload{{Project: "JARVIS_DEV"}},
		Memories:               []SyncMemoryPayload{{Project: "jarvis/dev"}},
		Prompts:                []SyncPromptPayload{{Project: "Jarvis Dev"}},
		Mutations: []MutationEnvelope{{
			Project: "JARVIS-DEV",
			Memory:  &MemoryPayload{Project: "jarvis.dev"},
		}},
	}

	require.NoError(t, ValidateSyncProjectIdentity(req))
	require.Equal(t, " Jarvis.Dev ", req.Project)
	require.Equal(t, "JARVIS_DEV", req.Sessions[0].Project)
	require.Equal(t, "jarvis/dev", req.Memories[0].Project)
	require.Equal(t, "Jarvis Dev", req.Prompts[0].Project)
	require.Equal(t, "JARVIS-DEV", req.Mutations[0].Project)
	require.Equal(t, "jarvis.dev", req.Mutations[0].Memory.Project)
}

// TestValidateSyncProjectIdentityKeepsDistinctSpellingsDistinct proves that a
// payload spelled "Foo.Bar" stays "Foo.Bar" — it is not folded onto "foo-bar",
// which is a different project to this API until an admin merges them.
func TestValidateSyncProjectIdentityKeepsDistinctSpellingsDistinct(t *testing.T) {
	req := SyncRequest{Project: "Foo.Bar"}
	require.NoError(t, ValidateSyncProjectIdentity(req))
	require.Equal(t, "Foo.Bar", req.Project)
}

func TestValidateSyncProjectIdentityRejectsUnknownContract(t *testing.T) {
	err := ValidateSyncProjectIdentity(SyncRequest{
		Project:                "jarvis-dev",
		ProjectIdentityVersion: "v99",
	})
	require.ErrorIs(t, err, ErrProjectIdentityVersionUnsupported)
}
