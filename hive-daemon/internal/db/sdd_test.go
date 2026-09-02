package db

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testSDDArtifacts = []string{
	"explore", "proposal", "spec", "design", "tasks", "apply-progress", "verify-report", "archive-report",
}

func TestFetchSDDArtifactsProjectsLatestEffectiveTopics(t *testing.T) {
	d := openTestDB(t)
	insertSDDMemory(t, d, "alpha", nil, "sdd/change-a/explore", "legacy-null", "2026-08-01 10:00:00", false)
	blank := "\t\u2003 "
	insertSDDMemory(t, d, "alpha", &blank, "  sdd/change-a/proposal  ", "legacy-blank", "2026-08-01 10:01:00", false)
	canonicalProposal := "sdd/change-a/proposal"
	insertSDDMemory(t, d, "alpha", &canonicalProposal, "ignored", "older-canonical", "2026-08-01 09:01:00", false)
	empty := ""
	insertSDDMemory(t, d, "alpha", &empty, "sdd/change-a/archive-report", "legacy-empty", "2026-08-01 10:01:30", false)
	padded := "\u2003sdd/change-a/spec\u2003"
	insertSDDMemory(t, d, "alpha", &padded, "ignored", "padded", "2026-08-01 10:02:00", false)
	canonical := "sdd/change-a/explore"
	insertSDDMemory(t, d, "alpha", &canonical, "ignored", "canonical-wins", "2026-08-01 10:03:00", false)
	wrong := "wrong/key"
	insertSDDMemory(t, d, "alpha", &wrong, "sdd/change-a/design", "must-not-fallback", "2026-08-01 10:04:00", false)
	insertSDDMemory(t, d, "beta", &canonical, "ignored", "other-project", "2026-08-01 10:05:00", false)
	descendant := "sdd/change-a/spec/child"
	insertSDDMemory(t, d, "alpha", &descendant, "ignored", "descendant", "2026-08-01 10:06:00", false)
	sibling := "sdd/change-ab/explore"
	insertSDDMemory(t, d, "alpha", &sibling, "ignored", "sibling", "2026-08-01 10:07:00", false)
	literalPercent := "sdd/change%_/tasks"
	insertSDDMemory(t, d, "alpha", &literalPercent, "ignored", "literal", "2026-08-01 10:08:00", false)

	rows, err := d.FetchSDDArtifacts("alpha", "change-a", testSDDArtifacts)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	contents := map[string]string{}
	for _, row := range rows {
		contents[row.Topic] = row.Content
	}
	assert.Equal(t, "canonical-wins", contents["sdd/change-a/explore"])
	assert.Equal(t, "legacy-blank", contents["sdd/change-a/proposal"])
	assert.Equal(t, "padded", contents["sdd/change-a/spec"])
	assert.Equal(t, "legacy-empty", contents["sdd/change-a/archive-report"])
	assert.NotContains(t, contents, "sdd/change-a/design")

	literalRows, err := d.FetchSDDArtifacts("alpha", "change%_", testSDDArtifacts)
	require.NoError(t, err)
	require.Len(t, literalRows, 1)
	assert.Equal(t, literalPercent, literalRows[0].Topic)
}

func TestFetchSDDArtifactsProjectsBeforeAnyBound(t *testing.T) {
	d := openTestDB(t)
	for i := 0; i < 501; i++ {
		topic := "sdd/noisy/explore"
		insertSDDMemory(t, d, "alpha", &topic, "ignored", fmt.Sprintf("revision-%03d", i), fmt.Sprintf("2026-08-01 %02d:%02d:%02d", 8+i/3600, (i/60)%60, i%60), false)
	}
	proposal := "sdd/noisy/proposal"
	insertSDDMemory(t, d, "alpha", &proposal, "ignored", "proposal", "2026-08-01 07:00:00", false)
	deletedID := insertSDDMemory(t, d, "alpha", &proposal, "ignored", "deleted", "2026-08-02 07:00:00", true)
	tiedID := insertSDDMemory(t, d, "alpha", &proposal, "ignored", "tie-loser", "2026-08-03 07:00:00", false)
	winnerID := insertSDDMemory(t, d, "alpha", &proposal, "ignored", "tie-winner", "2026-08-03 07:00:00", false)

	rows, err := d.FetchSDDArtifacts("alpha", "noisy", testSDDArtifacts)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	byTopic := map[string]SDDArtifact{}
	for _, row := range rows {
		byTopic[row.Topic] = row
	}
	assert.Equal(t, winnerID, byTopic[proposal].ID)
	assert.NotEqual(t, tiedID, byTopic[proposal].ID)
	assert.NotEqual(t, deletedID, byTopic[proposal].ID)
}

func TestListSDDChangesKeysetAfterProjection(t *testing.T) {
	d := openTestDB(t)
	for _, change := range []string{"alpha", "bravo", "charlie", "delta"} {
		for revision := 0; revision < 3; revision++ {
			topic := "sdd/" + change + "/explore"
			insertSDDMemory(t, d, "project", &topic, "ignored", "content", fmt.Sprintf("2026-08-01 10:0%d:00", revision), false)
		}
	}
	invalid := "sdd/echo/unknown"
	insertSDDMemory(t, d, "project", &invalid, "ignored", "content", "2026-08-01 11:00:00", false)

	first, err := d.ListSDDChanges("project", testSDDArtifacts, "", 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, first)
	second, err := d.ListSDDChanges("project", testSDDArtifacts, first[len(first)-1], 3)
	require.NoError(t, err)
	assert.Equal(t, []string{"delta"}, second)
}

func TestSDDCandidateQueryPlan(t *testing.T) {
	d := openTestDB(t)
	query, args, err := sddCandidateQuery("project", "sdd/change", testSDDArtifacts)
	require.NoError(t, err)
	rows, err := d.sqlDB.Query("EXPLAIN QUERY PLAN "+query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		details = append(details, detail)
	}
	plan := strings.Join(details, "\n")
	t.Logf("SDD EXPLAIN QUERY PLAN:\n%s", plan)
	assert.Contains(t, plan, "idx_memories_topic_key")
	assert.NotContains(t, strings.ToLower(plan), "prompts")
	assert.NotContains(t, strings.ToLower(plan), "fts")
}

func insertSDDMemory(t *testing.T, d *DB, project string, topic *string, title, content, createdAt string, deleted bool) int64 {
	t.Helper()
	ensureManualSaveSessions(t, d, project)
	var deletedAt any
	if deleted {
		deletedAt = createdAt
	}
	result, err := d.sqlDB.Exec(`
		INSERT INTO memories
			(sync_id, project, topic_key, title, content, created_at, updated_at, session_id, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fmt.Sprintf("sdd-test-%d", topicMemorySequence.Add(1)), project, topic, title, content, createdAt, createdAt, "manual-save-"+project, deletedAt)
	require.NoError(t, err)
	id, err := result.LastInsertId()
	require.NoError(t, err)
	return id
}
