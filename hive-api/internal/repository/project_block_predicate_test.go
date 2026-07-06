package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlockedProjectPredicateHelpers(t *testing.T) {
	require.Equal(t,
		"NOT EXISTS (SELECT 1 FROM project_blocks pb WHERE pb.blocked = true AND pb.canonical_project_key = canonical_project_key(memories.project))",
		unblockedProjectPredicate("memories.project"),
	)
	require.Equal(t,
		"EXISTS (SELECT 1 FROM project_blocks pb WHERE pb.blocked = true AND pb.canonical_project_key = canonical_project_key($1))",
		blockedProjectPredicate("$1"),
	)
}
