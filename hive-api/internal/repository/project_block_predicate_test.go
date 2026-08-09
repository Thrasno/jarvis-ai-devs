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
	// The identity-fold vocabulary this predicate must avoid is pinned
	// module-wide by TestNoProjectIdentityDerivationInAPISources; only the
	// COALESCE sentinel is specific to this predicate.
	assert.NotContains(t, predicate, "COALESCE",
		"the empty-string sentinel could quarantine every row on the backend")
}

func TestUnblockedProjectPredicateNegatesTheBlockedPredicate(t *testing.T) {
	require.Equal(t, "NOT "+blockedProjectPredicate("$1"), unblockedProjectPredicate("$1"))
}
