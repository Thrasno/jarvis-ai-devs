package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalSyncProjectIdentity(t *testing.T) {
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

	canonical, err := CanonicalSyncProjectIdentity(req)
	require.NoError(t, err)
	require.Equal(t, "jarvis-dev", canonical.Project)
	require.Equal(t, "jarvis-dev", canonical.Sessions[0].Project)
	require.Equal(t, "jarvis-dev", canonical.Memories[0].Project)
	require.Equal(t, "jarvis-dev", canonical.Prompts[0].Project)
	require.Equal(t, "jarvis-dev", canonical.Mutations[0].Project)
	require.Equal(t, "jarvis-dev", canonical.Mutations[0].Memory.Project)
}

func TestCanonicalSyncProjectIdentityRejectsUnknownContract(t *testing.T) {
	_, err := CanonicalSyncProjectIdentity(SyncRequest{
		Project:                "jarvis-dev",
		ProjectIdentityVersion: "v99",
	})
	require.ErrorIs(t, err, ErrProjectIdentityVersionUnsupported)
}
