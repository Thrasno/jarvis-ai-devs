package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlockedProjectPredicateHelpers(t *testing.T) {
	require.Equal(t,
		"NOT EXISTS (SELECT 1 FROM project_identity_spellings pis JOIN project_blocks pb ON pb.canonical_project_key = pis.project_key WHERE pb.blocked = true AND pis.spelling = memories.project)",
		unblockedProjectPredicate("memories.project"),
	)
	require.Equal(t,
		"EXISTS (SELECT 1 FROM project_identity_spellings pis JOIN project_blocks pb ON pb.canonical_project_key = pis.project_key WHERE pb.blocked = true AND pis.spelling = $1)",
		blockedProjectPredicate("$1"),
	)
}
