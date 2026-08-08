package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockedProjectPredicateConsidersEveryCandidateKey(t *testing.T) {
	predicate := blockedProjectPredicate("memories.project")

	assert.Contains(t, predicate, "pb.project = memories.project",
		"the literal spelling recorded on the block must match")
	assert.Contains(t, predicate, "pb.canonical_project_key IN (memories.project,",
		"a stored canonical key must match itself")
	assert.Contains(t, predicate, "regexp_replace(lower(memories.project), '[[:space:]/_.-]+', '-', 'g')",
		"raw legacy spellings must be folded before matching")
	assert.Contains(t, predicate, "FROM project_identity_spellings pbs WHERE pbs.spelling = memories.project",
		"the identity registry must stay authoritative when it knows the spelling")
	assert.NotContains(t, predicate, "canonical_project_key(",
		"quarantine must not depend on the diverging SQL key function")
}

func TestUnblockedProjectPredicateNegatesTheBlockedPredicate(t *testing.T) {
	require.Equal(t, "NOT "+blockedProjectPredicate("$1"), unblockedProjectPredicate("$1"))
}
