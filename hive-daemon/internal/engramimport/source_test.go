package engramimport

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestResolveSourceUsesExplicitPathThenEnvDataDirThenHomeDefault(t *testing.T) {
	t.Run("explicit path", func(t *testing.T) {
		path := createEngramFixture(t, nil)
		resolved, err := ResolveSource(SourceOptions{ExplicitPath: path})
		require.NoError(t, err)
		require.Equal(t, path, resolved.Path)
	})

	t.Run("env data dir", func(t *testing.T) {
		dataDir := t.TempDir()
		path := filepath.Join(dataDir, "engram.db")
		createEngramFixtureAt(t, path, nil)

		resolved, err := ResolveSource(SourceOptions{EnvDataDir: dataDir, HomeDir: t.TempDir()})
		require.NoError(t, err)
		require.Equal(t, path, resolved.Path)
	})

	t.Run("missing source reports checked paths", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "missing-env")
		homeDir := t.TempDir()

		_, err := ResolveSource(SourceOptions{EnvDataDir: dataDir, HomeDir: homeDir})
		require.ErrorIs(t, err, ErrSourceNotFound)
		require.ErrorContains(t, err, filepath.Join(dataDir, "engram.db"))
		require.ErrorContains(t, err, filepath.Join(homeDir, ".engram", "engram.db"))
	})
}

func TestAnalyzeSourceReadsInScopeRowsAndReportsInvalidAndSkippedRelations(t *testing.T) {
	path := createEngramFixture(t, func(sqlDB *sql.DB) {
		_, err := sqlDB.Exec(`
INSERT INTO sessions (id, project, directory, dev_id, client, started_at, ended_at, summary) VALUES
  ('ses-1', 'proj-a', 'C:/src/a', 'dev-a', 'opencode', '2026-06-11 10:00:00', NULL, 'summary');
INSERT INTO user_prompts (id, project, content, created_at) VALUES
  (11, 'proj-a', 'prompt content', '2026-06-11 10:01:00');
INSERT INTO observations (id, project, title, content, type, topic_key, session_id, created_at, updated_at) VALUES
  (21, 'proj-a', 'Decision', 'Keep daemon-owned import', 'decision', 'topic-a', 'ses-1', '2026-06-11 10:02:00', '2026-06-11 10:03:00'),
  (22, 'proj-a', '', '', 'bugfix', NULL, 'ses-1', '2026-06-11 10:04:00', '2026-06-11 10:05:00');
INSERT INTO memory_relations (id, source_id, target_id, relation) VALUES
  (1, 21, 22, 'related');`)
		require.NoError(t, err)
	})

	analysis, err := AnalyzeSource(context.Background(), Source{Path: path})
	require.NoError(t, err)
	require.Equal(t, 1, analysis.Counts.Sessions)
	require.Equal(t, 1, analysis.Counts.Prompts)
	require.Equal(t, 1, analysis.Counts.Observations)
	require.Equal(t, 1, analysis.SkippedRelations)
	require.Len(t, analysis.InvalidRows, 1)
	require.Equal(t, "observations", analysis.InvalidRows[0].Table)
	require.Equal(t, "22", analysis.InvalidRows[0].SourceID)
	require.Equal(t, []string{"proj-a"}, analysis.Projects)

	batch := BuildImportBatch(analysis)
	require.Len(t, batch.Sessions, 1)
	require.Len(t, batch.Prompts, 1)
	require.Len(t, batch.Memories, 1)
	require.Equal(t, "ses-1", batch.Memories[0].SessionSourceID)
}

func TestAnalyzeSourceRejectsMissingRequiredTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engram.db")
	sqlDB, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`CREATE TABLE observations (id INTEGER PRIMARY KEY, project TEXT, title TEXT, content TEXT)`)
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = AnalyzeSource(context.Background(), Source{Path: path})
	require.ErrorIs(t, err, ErrInvalidSchema)
	require.ErrorContains(t, err, "sessions")
	require.ErrorContains(t, err, "user_prompts")
}

func TestImportSourcePersistsMappedRowsWithHiveUUIDSyncIDs(t *testing.T) {
	path := createEngramFixture(t, func(sqlDB *sql.DB) {
		_, err := sqlDB.Exec(`
INSERT INTO sessions (id, project, directory, dev_id, client, started_at) VALUES
  ('ses-1', 'proj-a', 'C:/src/a', 'dev-a', 'opencode', '2026-06-11 10:00:00');
INSERT INTO user_prompts (id, project, content, created_at) VALUES
  (11, 'proj-a', 'prompt content', '2026-06-11 10:01:00');
INSERT INTO observations (id, project, title, content, type, session_id, created_at) VALUES
  (21, 'proj-a', 'Decision', 'Keep daemon-owned import', 'decision', 'ses-1', '2026-06-11 10:02:00');`)
		require.NoError(t, err)
	})
	hive := openHiveDBForImportTest(t)

	report, err := ImportSource(context.Background(), hive, ImportRequest{Source: Source{Path: path}, RunID: "run-1"})
	require.NoError(t, err)
	require.Equal(t, hivedb.ImportCounts{Imported: 3}, report.Counts)
	require.Equal(t, 0, report.SkippedRelations)
	require.Empty(t, report.InvalidRows)
	require.NoError(t, uuid.Validate(aliasSyncID(t, hive, "sessions", "ses-1")))
	require.NoError(t, uuid.Validate(aliasSyncID(t, hive, "user_prompts", "11")))
	require.NoError(t, uuid.Validate(aliasSyncID(t, hive, "observations", "21")))
	require.NotEqual(t, "21", aliasSyncID(t, hive, "observations", "21"))
}

func TestAnalyzeSourceSkipsObservationsWithMissingSessionAlias(t *testing.T) {
	path := createEngramFixture(t, func(sqlDB *sql.DB) {
		_, err := sqlDB.Exec(`
INSERT INTO sessions (id, project, directory, dev_id, client, started_at) VALUES
  ('ses-1', 'proj-a', 'C:/src/a', 'dev-a', 'opencode', '2026-06-11 10:00:00');
INSERT INTO observations (id, project, title, content, type, session_id, created_at) VALUES
  (21, 'proj-a', 'Valid', 'Valid content', 'decision', 'ses-1', '2026-06-11 10:02:00'),
  (22, 'proj-a', 'Orphan', 'Orphan content', 'decision', 'missing-session', '2026-06-11 10:03:00');`)
		require.NoError(t, err)
	})

	analysis, err := AnalyzeSource(context.Background(), Source{Path: path})
	require.NoError(t, err)
	require.Equal(t, 1, analysis.Counts.Observations)
	require.Len(t, analysis.Memories, 1)
	require.Equal(t, "21", analysis.Memories[0].SourceID)
	require.Len(t, analysis.InvalidRows, 1)
	require.Equal(t, InvalidRow{Table: "observations", SourceID: "22", Reason: "session_id references missing or skipped session"}, analysis.InvalidRows[0])
}

func TestAnalyzeSourceContentHashCoversMeaningfulSourceFields(t *testing.T) {
	baseSeed := func(sqlDB *sql.DB) {
		_, err := sqlDB.Exec(`
INSERT INTO sessions (id, project, directory, dev_id, client, started_at, ended_at, summary) VALUES
  ('ses-1', 'proj-a', 'C:/src/a', 'dev-a', 'opencode', '2026-06-11 10:00:00', '2026-06-11 10:30:00', 'summary');
INSERT INTO user_prompts (id, project, content, created_at) VALUES
  (11, 'proj-a', 'prompt content', '2026-06-11 10:01:00');
INSERT INTO observations (id, project, title, content, type, topic_key, session_id, created_at, updated_at) VALUES
  (21, 'proj-a', 'Decision', 'Keep daemon-owned import', 'decision', 'topic-a', 'ses-1', '2026-06-11 10:02:00', '2026-06-11 10:03:00');`)
		require.NoError(t, err)
	}
	baseSessionHash, basePromptHash, baseMemoryHash := analyzeSingleContentHashes(t, baseSeed)

	tests := []struct {
		name       string
		mutate     string
		wantChange string
	}{
		{name: "session source id", mutate: `UPDATE sessions SET id = 'ses-2' WHERE id = 'ses-1'; UPDATE observations SET session_id = 'ses-2' WHERE session_id = 'ses-1'`, wantChange: "session"},
		{name: "session project", mutate: `UPDATE sessions SET project = 'proj-b' WHERE id = 'ses-1'; UPDATE observations SET project = 'proj-b' WHERE session_id = 'ses-1'`, wantChange: "session"},
		{name: "session directory", mutate: `UPDATE sessions SET directory = 'C:/src/b' WHERE id = 'ses-1'`, wantChange: "session"},
		{name: "session dev id", mutate: `UPDATE sessions SET dev_id = 'dev-b' WHERE id = 'ses-1'`, wantChange: "session"},
		{name: "session client", mutate: `UPDATE sessions SET client = 'claude' WHERE id = 'ses-1'`, wantChange: "session"},
		{name: "session started at", mutate: `UPDATE sessions SET started_at = '2026-06-11 10:15:00' WHERE id = 'ses-1'`, wantChange: "session"},
		{name: "session ended at", mutate: `UPDATE sessions SET ended_at = '2026-06-11 10:45:00' WHERE id = 'ses-1'`, wantChange: "session"},
		{name: "session summary", mutate: `UPDATE sessions SET summary = 'updated summary' WHERE id = 'ses-1'`, wantChange: "session"},
		{name: "prompt source id", mutate: `UPDATE user_prompts SET id = 12 WHERE id = 11`, wantChange: "prompt"},
		{name: "prompt project", mutate: `UPDATE user_prompts SET project = 'proj-b' WHERE id = 11`, wantChange: "prompt"},
		{name: "prompt content", mutate: `UPDATE user_prompts SET content = 'updated prompt content' WHERE id = 11`, wantChange: "prompt"},
		{name: "prompt created at", mutate: `UPDATE user_prompts SET created_at = '2026-06-11 10:06:00' WHERE id = 11`, wantChange: "prompt"},
		{name: "observation source id", mutate: `UPDATE observations SET id = 22 WHERE id = 21`, wantChange: "memory"},
		{name: "observation project", mutate: `UPDATE sessions SET project = 'proj-b' WHERE id = 'ses-1'; UPDATE observations SET project = 'proj-b' WHERE id = 21`, wantChange: "memory"},
		{name: "observation title", mutate: `UPDATE observations SET title = 'Updated decision' WHERE id = 21`, wantChange: "memory"},
		{name: "observation content", mutate: `UPDATE observations SET content = 'Updated imported content' WHERE id = 21`, wantChange: "memory"},
		{name: "observation category", mutate: `UPDATE observations SET type = 'bugfix' WHERE id = 21`, wantChange: "memory"},
		{name: "observation topic key", mutate: `UPDATE observations SET topic_key = 'topic-b' WHERE id = 21`, wantChange: "memory"},
		{name: "observation session id", mutate: `INSERT INTO sessions (id, project, directory, dev_id, client, started_at) VALUES ('ses-2', 'proj-a', 'C:/src/a', 'dev-a', 'opencode', '2026-06-11 10:05:00'); UPDATE observations SET session_id = 'ses-2' WHERE id = 21`, wantChange: "memory"},
		{name: "observation created at", mutate: `UPDATE observations SET created_at = '2026-06-11 10:04:00' WHERE id = 21`, wantChange: "memory"},
		{name: "observation updated at", mutate: `UPDATE observations SET updated_at = '2026-06-11 10:05:00' WHERE id = 21`, wantChange: "memory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionHash, promptHash, memoryHash := analyzeSingleContentHashes(t, func(sqlDB *sql.DB) {
				baseSeed(sqlDB)
				_, err := sqlDB.Exec(tt.mutate)
				require.NoError(t, err)
			})
			switch tt.wantChange {
			case "session":
				require.NotEqual(t, baseSessionHash, sessionHash)
			case "prompt":
				require.NotEqual(t, basePromptHash, promptHash)
			case "memory":
				require.NotEqual(t, baseMemoryHash, memoryHash)
			}
		})
	}
}

func TestImportSourceRejectsAliasReuseWhenMeaningfulSourceContentChanges(t *testing.T) {
	tests := []struct {
		name          string
		mutate        string
		wantErrorPart string
	}{
		{name: "session imported field", mutate: `UPDATE sessions SET directory = 'C:/src/b' WHERE id = 'ses-1'`, wantErrorPart: "sessions/ses-1"},
		{name: "observation imported field", mutate: `UPDATE observations SET type = 'bugfix' WHERE id = 21`, wantErrorPart: "observations/21"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := createEngramFixture(t, func(sqlDB *sql.DB) {
				_, err := sqlDB.Exec(`
INSERT INTO sessions (id, project, directory, dev_id, client, started_at) VALUES
  ('ses-1', 'proj-a', 'C:/src/a', 'dev-a', 'opencode', '2026-06-11 10:00:00');
INSERT INTO observations (id, project, title, content, type, session_id, created_at) VALUES
  (21, 'proj-a', 'Decision', 'Keep daemon-owned import', 'decision', 'ses-1', '2026-06-11 10:02:00');`)
				require.NoError(t, err)
			})
			hive := openHiveDBForImportTest(t)

			_, err := ImportSource(context.Background(), hive, ImportRequest{Source: Source{Path: path}, RunID: "run-1"})
			require.NoError(t, err)
			execEngramSQL(t, path, tt.mutate)

			_, err = ImportSource(context.Background(), hive, ImportRequest{Source: Source{Path: path}, RunID: "run-2"})
			require.ErrorIs(t, err, hivedb.ErrImportAliasContentChanged)
			require.ErrorContains(t, err, tt.wantErrorPart)
		})
	}
}

func TestFingerprintIncludesSQLiteSidecarFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "engram.db")
	require.NoError(t, os.WriteFile(path, []byte("main"), 0o600))

	withoutSidecars := fingerprint(path)
	require.NoError(t, os.WriteFile(path+"-wal", []byte("wal-v1"), 0o600))
	withWAL := fingerprint(path)
	require.NotEqual(t, withoutSidecars, withWAL)

	require.NoError(t, os.WriteFile(path+"-shm", []byte("shm-v1"), 0o600))
	withSHM := fingerprint(path)
	require.NotEqual(t, withWAL, withSHM)
}

func analyzeSingleContentHashes(t *testing.T, seed func(*sql.DB)) (string, string, string) {
	t.Helper()
	analysis, err := AnalyzeSource(context.Background(), Source{Path: createEngramFixture(t, seed)})
	require.NoError(t, err)
	require.NotEmpty(t, analysis.Sessions)
	require.Len(t, analysis.Prompts, 1)
	require.Len(t, analysis.Memories, 1)
	return analysis.Sessions[0].ContentHash, analysis.Prompts[0].ContentHash, analysis.Memories[0].ContentHash
}

func execEngramSQL(t *testing.T, path, statement string) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = sqlDB.Exec(statement)
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func createEngramFixture(t *testing.T, seed func(*sql.DB)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engram.db")
	createEngramFixtureAt(t, path, seed)
	return path
}

func createEngramFixtureAt(t *testing.T, path string, seed func(*sql.DB)) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = sqlDB.Exec(`
CREATE TABLE observations (
  id INTEGER PRIMARY KEY,
  project TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL DEFAULT '',
  topic_key TEXT,
  session_id TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT
);
CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  project TEXT NOT NULL DEFAULT '',
  directory TEXT NOT NULL DEFAULT '',
  dev_id TEXT NOT NULL DEFAULT '',
  client TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  ended_at TEXT,
  summary TEXT
);
CREATE TABLE user_prompts (
  id INTEGER PRIMARY KEY,
  project TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE memory_relations (
  id INTEGER PRIMARY KEY,
  source_id INTEGER,
  target_id INTEGER,
  relation TEXT
);`)
	require.NoError(t, err)
	if seed != nil {
		seed(sqlDB)
	}
	require.NoError(t, sqlDB.Close())
}

func openHiveDBForImportTest(t *testing.T) *hivedb.DB {
	t.Helper()
	d, err := hivedb.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func aliasSyncID(t *testing.T, hive *hivedb.DB, table, sourceID string) string {
	t.Helper()
	var syncID string
	err := hive.RawDB().QueryRow(`
SELECT hive_sync_id FROM import_source_aliases
WHERE source_system = 'engram' AND source_table = ? AND source_id = ? AND source_project = 'proj-a'`, table, sourceID).Scan(&syncID)
	require.NoError(t, err)
	return syncID
}
