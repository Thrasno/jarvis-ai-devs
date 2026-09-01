package db

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopicSelectorValidation(t *testing.T) {
	tests := []struct {
		name     string
		selector TopicSelector
		wantErr  string
	}{
		{name: "blank exact", selector: TopicSelector{TopicKey: "  "}, wantErr: "topic selector is blank"},
		{name: "blank prefix", selector: TopicSelector{TopicPrefix: "\t"}, wantErr: "topic selector is blank"},
		{name: "missing selector", selector: TopicSelector{}, wantErr: "topic selector is required"},
		{name: "exact and prefix", selector: TopicSelector{TopicKey: "a", TopicPrefix: "b"}, wantErr: "mutually exclusive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newSQLiteTopicSelection(tt.selector)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestSelectLatestTopicMemoriesLiteralSelection(t *testing.T) {
	tests := []struct {
		name     string
		selector TopicSelector
		topics   []string
		want     []string
	}{
		{name: "exact only", selector: TopicSelector{TopicKey: "  architecture/auth  "}, topics: []string{"architecture/auth", "architecture/auth/jwt", "architecture/authz"}, want: []string{"architecture/auth"}},
		{name: "prefix includes self and descendants", selector: TopicSelector{TopicPrefix: " architecture/auth "}, topics: []string{"architecture/auth", "architecture/auth/jwt", "architecture/authz", "architecture/other"}, want: []string{"architecture/auth/jwt", "architecture/auth"}},
		{name: "percent is literal", selector: TopicSelector{TopicPrefix: "release/%"}, topics: []string{"release/%", "release/%/notes", "release/v1"}, want: []string{"release/%/notes", "release/%"}},
		{name: "underscore is literal", selector: TopicSelector{TopicPrefix: "team/_"}, topics: []string{"team/_", "team/_/notes", "team/a"}, want: []string{"team/_/notes", "team/_"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := openTestDB(t)
			for i, topic := range tt.topics {
				insertTopicMemory(t, d, "project-a", topic, fmt.Sprintf("2026-08-01 10:%02d:00", i), false)
			}

			memories, err := d.SelectLatestTopicMemories("project-a", tt.selector, 20)
			require.NoError(t, err)
			assert.Equal(t, tt.want, memoryTopics(memories))
		})
	}
}

func TestSelectLatestTopicMemoriesCompatibilityAndIsolation(t *testing.T) {
	d := openTestDB(t)
	insertTopicMemory(t, d, "project-a", "\tarchitecture/auth\t", "2026-08-01 10:00:00", false)
	latestAuthID := insertTopicMemory(t, d, "project-a", "architecture/auth", "2026-08-01 10:00:00", false)
	insertTopicMemory(t, d, "project-a", " architecture/auth/jwt ", "2026-08-01 10:01:00", false)
	insertTopicMemory(t, d, "project-b", "architecture/auth", "2026-08-01 10:02:00", false)
	insertTopicMemory(t, d, "project-a", "architecture/auth/deleted", "2026-08-01 10:03:00", true)

	memories, err := d.SelectLatestTopicMemories("project-a", TopicSelector{TopicPrefix: "architecture/auth"}, 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"architecture/auth/jwt", "architecture/auth"}, memoryTopics(memories))
	for _, memory := range memories {
		assert.Equal(t, "project-a", memory.Project)
	}
	exact, err := d.SelectLatestTopicMemories("project-a", TopicSelector{TopicKey: "architecture/auth"}, 20)
	require.NoError(t, err)
	assert.Equal(t, []string{"architecture/auth"}, memoryTopics(exact))
	require.Len(t, exact, 1)
	assert.Equal(t, latestAuthID, exact[0].ID, "padded and canonical revisions must share one logical topic")
}

func TestSelectLatestTopicMemoriesProjectsBeforeLimit(t *testing.T) {
	d := openTestDB(t)
	for i := 0; i < 501; i++ {
		insertTopicMemory(t, d, "project-a", "area/noisy", fmt.Sprintf("2026-08-01 10:%02d:%02d", i/60, i%60), false)
	}
	otherID := insertTopicMemory(t, d, "project-a", "area/other", "2026-08-01 09:00:00", false)
	deletedLatest := insertTopicMemory(t, d, "project-a", "area/noisy", "2026-08-01 20:00:00", true)
	tiedLatest := insertTopicMemory(t, d, "project-a", "area/noisy", "2026-08-01 19:00:00", false)
	tiedWinner := insertTopicMemory(t, d, "project-a", "area/noisy", "2026-08-01 19:00:00", false)

	memories, err := d.SelectLatestTopicMemories("project-a", TopicSelector{TopicPrefix: "area"}, 2)
	require.NoError(t, err)
	require.Len(t, memories, 2)
	assert.Equal(t, tiedWinner, memories[0].ID, "id must break equal created_at ties")
	assert.NotEqual(t, tiedLatest, memories[0].ID)
	assert.Equal(t, otherID, memories[1].ID, "projection must happen before the response limit")
	assert.NotEqual(t, deletedLatest, memories[0].ID)
}

func TestTopicSelectorCanonicalBranchesUseTopicIndex(t *testing.T) {
	d := openTestDB(t)
	tests := []struct {
		name     string
		selector TopicSelector
		wantPlan string
	}{
		{name: "exact", selector: TopicSelector{TopicKey: "architecture/auth"}, wantPlan: "project=? AND topic_key=?"},
		{name: "prefix", selector: TopicSelector{TopicPrefix: "architecture/auth"}, wantPlan: "project=? AND topic_key>? AND topic_key<?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, err := newSQLiteTopicSelection(tt.selector)
			require.NoError(t, err)
			rows, err := d.sqlDB.Query("EXPLAIN QUERY PLAN "+selection.candidateQuery, selection.args("project-a")...)
			require.NoError(t, err)
			defer func() { _ = rows.Close() }()
			var details []string
			for rows.Next() {
				var id, parent, unused int
				var detail string
				require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
				details = append(details, detail)
			}
			require.NoError(t, rows.Err())
			plan := strings.Join(details, "\n")
			t.Logf("EXPLAIN QUERY PLAN for canonical %s selector:\n%s", tt.name, plan)
			assert.Contains(t, plan, "USING INDEX idx_memories_topic_key")
			assert.Contains(t, plan, tt.wantPlan)
			assert.Contains(t, plan, "topic_key>?)", "the compatibility branch only uses the index to bound non-NULL rows; TRIM itself is not indexed")
		})
	}
}

var topicMemorySequence atomic.Int64

func insertTopicMemory(t *testing.T, d *DB, project, topic, createdAt string, deleted bool) int64 {
	t.Helper()
	ensureManualSaveSessions(t, d, project)
	var deletedAt any
	if deleted {
		deletedAt = createdAt
	}
	result, err := d.sqlDB.Exec(`
		INSERT INTO memories
			(sync_id, project, topic_key, title, content, created_at, updated_at, session_id, deleted_at)
		VALUES (?, ?, ?, ?, 'content', ?, ?, ?, ?)`,
		fmt.Sprintf("topic-selector-%d", topicMemorySequence.Add(1)), project, topic, topic, createdAt, createdAt, "manual-save-"+project, deletedAt)
	require.NoError(t, err)
	id, err := result.LastInsertId()
	require.NoError(t, err)
	return id
}

func memoryTopics(memories []*models.Memory) []string {
	topics := make([]string, 0, len(memories))
	for _, memory := range memories {
		if memory.TopicKey != nil {
			topics = append(topics, *memory.TopicKey)
		}
	}
	return topics
}
