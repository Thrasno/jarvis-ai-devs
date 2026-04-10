package db

import (
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
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

	if _, err := d.SaveMemory(newMemory("proj", "JWT Authentication", "content A")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SaveMemory(newMemory("proj", "Unrelated topic", "content B")); err != nil {
		t.Fatal(err)
	}

	results, err := d.Search("JWT", "proj", 10)
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

	mem := newMemory("proj", "Architecture Notes", "We use SQLite for persistent storage")
	if _, err := d.SaveMemory(mem); err != nil {
		t.Fatal(err)
	}

	results, err := d.Search("SQLite", "proj", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Error("expected result for content match, got none")
	}
}

func TestSearch_SpecialCharactersNocrash(t *testing.T) {
	d := openTestDB(t)

	if _, err := d.SaveMemory(newMemory("proj", "Title", "content")); err != nil {
		t.Fatal(err)
	}

	// These should NOT panic or return an error — just 0 or more results
	specialQueries := []string{"@#user", "user@domain.com", "foo:bar", "hello*world"}
	for _, q := range specialQueries {
		t.Run(q, func(t *testing.T) {
			_, err := d.Search(q, "proj", 10)
			if err != nil {
				t.Errorf("Search(%q) should not error, got: %v", q, err)
			}
		})
	}
}

func TestSearch_EmptyQuery_ReturnsAllForProject(t *testing.T) {
	d := openTestDB(t)

	for i := 0; i < 3; i++ {
		if _, err := d.SaveMemory(newMemory("proj", "mem", "content")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.SaveMemory(newMemory("other", "other mem", "content")); err != nil {
		t.Fatal(err)
	}

	results, err := d.Search("", "proj", 10)
	if err != nil {
		t.Fatalf("Search('', 'proj') failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("empty query should return all for project, got %d results", len(results))
	}
}

func TestSearch_ProjectFilter_IsolatesResults(t *testing.T) {
	d := openTestDB(t)

	if _, err := d.SaveMemory(newMemory("foo", "Auth System", "jwt")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SaveMemory(newMemory("bar", "Auth System", "jwt")); err != nil {
		t.Fatal(err)
	}

	results, err := d.Search("auth", "foo", 10)
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

	if _, err := d.SaveMemory(newMemory("foo", "Auth System", "content")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SaveMemory(newMemory("bar", "Auth System", "content")); err != nil {
		t.Fatal(err)
	}

	results, err := d.Search("auth", "", 10)
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
	if _, err := d.SaveMemory(newMemory("proj", "Generic Title", "SQLite is the database engine")); err != nil {
		t.Fatal(err)
	}
	// Term in title → should rank higher
	if _, err := d.SaveMemory(newMemory("proj", "SQLite Architecture", "generic description here")); err != nil {
		t.Fatal(err)
	}

	results, err := d.Search("SQLite", "proj", 10)
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
		if _, err := d.SaveMemory(newMemory("proj", "Auth topic", "content")); err != nil {
			t.Fatal(err)
		}
	}

	results, err := d.Search("auth", "proj", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results with limit=3, got %d", len(results))
	}
}

// ─── 4.3 FTS ACID Compliance ───────────────────────────────────────────────

func TestFTS_TriggerRollback_ACIDCompliance(t *testing.T) {
	d := openTestDB(t)

	tx, err := d.sqlDB.Begin()
	if err != nil {
		t.Fatal(err)
	}

	// Insert within transaction — trigger memories_ai should fire
	_, err = tx.Exec(`
		INSERT INTO memories
			(sync_id, project, title, content, tags, files_affected, created_by, created_at)
		VALUES
			('acid-test-uuid', 'proj', 'ACID Rollback Test', 'unique rollback content', '[]', '[]', 'test', CURRENT_TIMESTAMP)
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

	mem := &models.Memory{
		Project: "proj",
		Title:   "Unique FTS Sync Test",
		Content: "verifying trigger sync",
		Tags:    []string{"fts", "trigger"},
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
