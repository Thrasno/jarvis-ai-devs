package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockedProjectPredicateComparesTheStoredLiteralExactly(t *testing.T) {
	predicate := blockedProjectPredicate("memories.project")

	assert.Equal(t,
		"EXISTS (SELECT 1 FROM project_blocks pb WHERE pb.blocked = true AND pb.canonical_project_key = memories.project)",
		predicate)
	assert.NotContains(t, predicate, "regexp_replace",
		"quarantine must not fold spellings together")
	assert.NotContains(t, predicate, "COALESCE",
		"the empty-string sentinel could quarantine every row on the backend")
	assert.NotContains(t, predicate, "project_identity_spellings",
		"quarantine must not resolve identity through the registry")
	assert.NotContains(t, predicate, "canonical_project_key(",
		"quarantine must not depend on the diverging SQL key function")
}

func TestUnblockedProjectPredicateNegatesTheBlockedPredicate(t *testing.T) {
	require.Equal(t, "NOT "+blockedProjectPredicate("$1"), unblockedProjectPredicate("$1"))
}
