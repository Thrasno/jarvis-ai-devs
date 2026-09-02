package db

import (
	"fmt"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

// ─── 4.1 Query Sanitization ────────────────────────────────────────────────

func TestBuildFTS5Query_WrapsSingleTerm(t *testing.T) {
	got := buildFTS5Query("auth")
	want := `"auth"`
	if got != want {
		t.Errorf("buildFTS5Query(%q) = %q, want %q", "auth", got, want)
	}
}

func TestBuildFTS5Query_WrapsMultipleTerms(t *testing.T) {
	got := buildFTS5Query("jwt authentication")
	want := `"jwt" "authentication"`
	if got != want {
		t.Errorf("buildFTS5Query(%q) = %q, want %q", "jwt authentication", got, want)
	}
}

func TestBuildFTS5Query_SpecialCharacters(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`@#user`, `"@#user"`},
		{`user@domain.com`, `"user@domain.com"`},
		{`foo:bar`, `"foo:bar"`},
		{`hello"world`, `"hello""world"`}, // internal quotes escaped
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := buildFTS5Query(tt.input)
			if got != tt.want {
				t.Errorf("buildFTS5Query(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildFTS5Query_EmptyReturnsEmpty(t *testing.T) {
	tests := []string{"", "   ", "\t"}
	for _, input := range tests {
		got := buildFTS5Query(input)
		if got != "" {
			t.Errorf("buildFTS5Query(%q) = %q, want ''", input, got)
		}
	}
}

// ─── 4.2 Search ────────────────────────────────────────────────────────────

func TestSearch_FindsByTitle(t *testing.T) {
	d := openTestDB(t)

	if _, err := saveTestMemory(t, d, newMemory("proj", "JWT Authentication", "content A")); err != nil {
		t.Fatal(err)
	}
	if _, err := saveTestMemory(t, d, newMemory("proj", "Unrelated topic", "content B")); err != nil {
		t.Fatal(err)
	}

	results, err := search(t, d, "JWT", "proj", "", 10)
	if err != nil {
		t.Fatalf("Search() failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "JWT Authentication" {
		t.Errorf("got title %q, want 'JWT Authentication'", results[0].Title)
	}
}

func TestSearch_FindsByContent(t *testing.T) {
	d := openTestDB(t)
	ensureManualSaveSessions(t, d, "proj")

	mem := newMemory("proj", "Architecture Notes", "We use SQLite for persistent storage")
	if _, err := d.SaveMemory(mem); err != nil {
		t.Fatal(err)
	}

	results, err := search(t, d, "SQLite", "proj", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("expected result for content match, got none")
	}
}

func TestSearch_SpecialCharactersNocrash(t *testing.T) {
	d := openTestDB(t)

	if _, err := saveTestMemory(t, d, newMemory("proj", "Title", "content")); err != nil {
		t.Fatal(err)
	}

	// These should NOT panic or return an error — just 0 or more results
	specialQueries := []string{"@#user", "user@domain.com", "foo:bar", "hello*world"}
	for _, q := range specialQueries {
		t.Run(q, func(t *testing.T) {
			_, err := search(t, d, q, "proj", "", 10)
			if err != nil {
				t.Errorf("Search(%q) should not error, got: %v", q, err)
			}
		})
	}
}

func TestSearch_EmptyQuery_ReturnsValidationError(t *testing.T) {
	d := openTestDB(t)
	_, err := d.Search(models.MemorySearchCriteria{Project: "proj", Limit: 10})
	if err == nil {
		t.Fatal("Search without a criterion must fail")
	}
}

func TestSearch_StructuredTopicOnlyUsesLiteralSelectionAndReturnsRevisions(t *testing.T) {
	d := openTestDB(t)
	exact := "architecture/auth"
	prefix := "release/%"

	first := insertSearchMemory(t, d, "proj", exact, "opaque one", false)
	second := insertSearchMemory(t, d, "proj", "  "+exact+"  ", "opaque two", false)
	insertSearchMemory(t, d, "proj", exact+"z", "lookalike", false)
	insertSearchMemory(t, d, "other", exact, "other project", false)
	insertSearchMemory(t, d, "proj", exact+"/deleted", "deleted", true)
	percentSelf := insertSearchMemory(t, d, "proj", prefix, "percent self", false)
	percentChild := insertSearchMemory(t, d, "proj", prefix+"/notes", "percent child", false)
	insertSearchMemory(t, d, "proj", "release/v1", "wildcard lookalike", false)
	underscore := "team/_"
	underscoreSelf := insertSearchMemory(t, d, "proj", underscore, "underscore self", false)
	underscoreChild := insertSearchMemory(t, d, "proj", underscore+"/notes", "underscore child", false)
	insertSearchMemory(t, d, "proj", "team/a", "underscore lookalike", false)

	tests := []struct {
		name string
		key  *string
		pre  *string
		want []int64
	}{
		{name: "exact keeps ordinary revisions", key: &exact, want: []int64{second, first}},
		{name: "literal percent prefix", pre: &prefix, want: []int64{percentChild, percentSelf}},
		{name: "literal underscore prefix", pre: &underscore, want: []int64{underscoreChild, underscoreSelf}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.Search(models.MemorySearchCriteria{Project: "proj", TopicKey: tt.key, TopicPrefix: tt.pre, Limit: 20})
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(memoryIDs(got)) != fmt.Sprint(tt.want) {
				t.Fatalf("ids = %v, want %v", memoryIDs(got), tt.want)
			}
		})
	}
}

func TestSearch_QueryAndTopicFilterBeforeRankingAndLimit(t *testing.T) {
	tests := []struct {
		name string
		key  *string
		pre  *string
	}{
		{name: "exact", key: stringPointer("target/exact")},
		{name: "prefix", pre: stringPointer("target/prefix")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := openTestDB(t)
			selectedTopic := "target/exact"
			if tt.pre != nil {
				selectedTopic = "target/prefix/child"
			}
			selected := insertSearchMemory(t, d, "proj", selectedTopic, "needle in content", false)
			insertSearchMemory(t, d, "proj", "outside", "needle", false)

			got, err := d.Search(models.MemorySearchCriteria{
				Query: "needle", Project: "proj", TopicKey: tt.key, TopicPrefix: tt.pre, Limit: 1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].ID != selected {
				t.Fatalf("ids = %v, want selected id %d", memoryIDs(got), selected)
			}
		})
	}
}

func TestSearch_ValidatesStructuredCriteria(t *testing.T) {
	d := openTestDB(t)
	blank := "  "
	exact := "a"
	prefix := "b"
	tests := []models.MemorySearchCriteria{
		{Limit: 10},
		{Query: "query", TopicKey: &blank, Limit: 10},
		{Query: "query", TopicPrefix: &blank, Limit: 10},
		{TopicKey: &exact, TopicPrefix: &prefix, Limit: 10},
	}
	for i, criteria := range tests {
		if _, err := d.Search(criteria); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
}

func search(t *testing.T, d *DB, query, project, category string, limit int) ([]*models.Memory, error) {
	t.Helper()
	return d.Search(models.MemorySearchCriteria{Query: query, Project: project, Category: category, Limit: limit})
}

func stringPointer(value string) *string { return &value }

func memoryIDs(memories []*models.Memory) []int64 {
	ids := make([]int64, 0, len(memories))
	for _, memory := range memories {
		ids = append(ids, memory.ID)
	}
	return ids
}

func insertSearchMemory(t *testing.T, d *DB, project, topic, content string, deleted bool) int64 {
	t.Helper()
	id := insertTopicMemory(t, d, project, topic, "2026-08-01 10:00:00", deleted)
	_, err := d.sqlDB.Exec(`UPDATE memories SET title = 'opaque', content = ? WHERE id = ?`, content, id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSearch_ExcludesSoftDeletedMemoriesFromDefaultReads(t *testing.T) {
	d := openTestDB(t)

	active := newMemory("proj", "Active FTS Tombstone", "searchable tombstone content")
	active.Category = "architecture"
	if _, err := saveTestMemory(t, d, active); err != nil {
		t.Fatal(err)
	}

	deleted := newMemory("proj", "Deleted FTS Tombstone", "searchable tombstone content")
	deleted.Category = "architecture"
	deletedID, err := saveTestMemory(t, d, deleted)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteMemory(deletedID, "tester", "verify search hides tombstones"); err != nil {
		t.Fatalf("DeleteMemory() failed: %v", err)
	}

	tests := []struct {
		name     string
		query    string
		category string
	}{
		{name: "fts term search", query: "searchable", category: "architecture"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := search(t, d, tt.query, "proj", tt.category, 10)
			if err != nil {
				t.Fatalf("Search(%q, %q, %q) failed: %v", tt.query, "proj", tt.category, err)
			}
			if len(results) != 1 {
				t.Fatalf("expected only the active memory, got %d results", len(results))
			}
			if results[0].Title != "Active FTS Tombstone" {
				t.Fatalf("default search returned %q, want active memory only", results[0].Title)
			}
		})
	}

	deletedRead, err := d.GetDeletedMemory(deletedID)
	if err != nil {
		t.Fatalf("GetDeletedMemory(%d) failed: %v", deletedID, err)
	}
	if deletedRead.Memory.Title != "Deleted FTS Tombstone" {
		t.Fatalf("deleted read title = %q, want deleted tombstone", deletedRead.Memory.Title)
	}
	if deletedRead.DeletedBy != "tester" {
		t.Fatalf("deleted read actor = %q, want tester", deletedRead.DeletedBy)
	}
}

func TestSearch_ProjectFilter_IsolatesResults(t *testing.T) {
	d := openTestDB(t)

	if _, err := saveTestMemory(t, d, newMemory("foo", "Auth System", "jwt")); err != nil {
		t.Fatal(err)
	}
	if _, err := saveTestMemory(t, d, newMemory("bar", "Auth System", "jwt")); err != nil {
		t.Fatal(err)
	}

	results, err := search(t, d, "auth", "foo", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for project 'foo', got %d", len(results))
	}
	if results[0].Project != "foo" {
		t.Errorf("result project = %q, want 'foo'", results[0].Project)
	}
}

func TestSearch_NoProjectFilter_SearchesAll(t *testing.T) {
	d := openTestDB(t)

	if _, err := saveTestMemory(t, d, newMemory("foo", "Auth System", "content")); err != nil {
		t.Fatal(err)
	}
	if _, err := saveTestMemory(t, d, newMemory("bar", "Auth System", "content")); err != nil {
		t.Fatal(err)
	}

	results, err := search(t, d, "auth", "", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Errorf("no project filter should return all projects, got %d results", len(results))
	}
}

func TestSearch_BM25_TitleRanksAboveContent(t *testing.T) {
	d := openTestDB(t)

	// Term only in content → should rank lower
	if _, err := saveTestMemory(t, d, newMemory("proj", "Generic Title", "SQLite is the database engine")); err != nil {
		t.Fatal(err)
	}
	// Term in title → should rank higher
	if _, err := saveTestMemory(t, d, newMemory("proj", "SQLite Architecture", "generic description here")); err != nil {
		t.Fatal(err)
	}

	results, err := search(t, d, "SQLite", "proj", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "SQLite Architecture" {
		t.Errorf("title match should rank first, got %q", results[0].Title)
	}
}

func TestSearch_RespectsLimit(t *testing.T) {
	d := openTestDB(t)

	for i := 0; i < 5; i++ {
		if _, err := saveTestMemory(t, d, newMemory("proj", "Auth topic", "content")); err != nil {
			t.Fatal(err)
		}
	}

	results, err := search(t, d, "auth", "proj", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results with limit=3, got %d", len(results))
	}
}

func TestSearch_CategoryFilter(t *testing.T) {
	d := openTestDB(t)
	ensureManualSaveSessions(t, d, "proj")

	// Save observations of two different categories
	archMem := newMemory("proj", "Auth Design", "jwt authentication system")
	archMem.Category = "architecture"
	if _, err := d.SaveMemory(archMem); err != nil {
		t.Fatal(err)
	}

	bugMem := newMemory("proj", "Auth Bug Fix", "fixed jwt token validation")
	bugMem.Category = "bugfix"
	if _, err := d.SaveMemory(bugMem); err != nil {
		t.Fatal(err)
	}

	// Filter by "architecture" only
	results, err := search(t, d, "auth", "proj", "architecture", 10)
	if err != nil {
		t.Fatalf("Search with category filter failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result with category=architecture, got %d", len(results))
	}
	if results[0].Category != "architecture" {
		t.Errorf("result category = %q, want 'architecture'", results[0].Category)
	}

	// Filter by "bugfix" only
	results, err = search(t, d, "auth", "proj", "bugfix", 10)
	if err != nil {
		t.Fatalf("Search with category=bugfix failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result with category=bugfix, got %d", len(results))
	}
	if results[0].Category != "bugfix" {
		t.Errorf("result category = %q, want 'bugfix'", results[0].Category)
	}

	// No category filter — both returned
	results, err = search(t, d, "auth", "proj", "", 10)
	if err != nil {
		t.Fatalf("Search without category filter failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("without category filter, expected 2 results, got %d", len(results))
	}
}

// ─── 4.3 FTS ACID Compliance ───────────────────────────────────────────────

func TestFTS_TriggerRollback_ACIDCompliance(t *testing.T) {
	d := openTestDB(t)
	ensureManualSaveSessions(t, d, "proj")

	tx, err := d.sqlDB.Begin()
	if err != nil {
		t.Fatal(err)
	}

	// Insert within transaction — trigger memories_ai should fire
	_, err = tx.Exec(`
		INSERT INTO memories
			(sync_id, project, title, content, tags, files_affected, created_by, created_at, session_id)
		VALUES
			('acid-test-uuid', 'proj', 'ACID Rollback Test', 'unique rollback content', '[]', '[]', 'test', CURRENT_TIMESTAMP, 'manual-save-proj')
	`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("insert in tx failed: %v", err)
	}

	// FTS should see the entry within the open transaction
	var countBefore int
	err = tx.QueryRow(
		`SELECT COUNT(*) FROM memories_fts WHERE memories_fts MATCH '"ACID Rollback Test"'`,
	).Scan(&countBefore)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("fts query in tx failed: %v", err)
	}
	if countBefore == 0 {
		_ = tx.Rollback()
		t.Error("FTS should contain entry within open transaction")
	}

	// ROLLBACK — both memories and memories_fts should revert
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	// Verify FTS entry was rolled back too (ACID compliance)
	var countAfter int
	err = d.sqlDB.QueryRow(
		`SELECT COUNT(*) FROM memories_fts WHERE memories_fts MATCH '"ACID Rollback Test"'`,
	).Scan(&countAfter)
	if err != nil {
		t.Fatalf("fts query after rollback failed: %v", err)
	}
	if countAfter != 0 {
		t.Errorf("FTS entry should be rolled back (ACID), but COUNT = %d", countAfter)
	}
}

func TestFTS_InsertSync_MemoryAppearsinFTS(t *testing.T) {
	d := openTestDB(t)
	ensureManualSaveSessions(t, d, "proj")

	mem := &models.Memory{
		Project:   "proj",
		Title:     "Unique FTS Sync Test",
		Content:   "verifying trigger sync",
		Tags:      []string{"fts", "trigger"},
		SessionID: "manual-save-proj",
	}
	if _, err := d.SaveMemory(mem); err != nil {
		t.Fatal(err)
	}

	var count int
	err := d.sqlDB.QueryRow(
		`SELECT COUNT(*) FROM memories_fts WHERE memories_fts MATCH '"Unique FTS Sync Test"'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("fts query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 FTS entry after insert, got %d", count)
	}
}
