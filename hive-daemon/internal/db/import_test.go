package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestImportSchemaCreatesRunsAliasesAndIndexes(t *testing.T) {
	d := openTestDB(t)

	for _, tt := range []struct {
		kind string
		name string
	}{
		{kind: "table", name: "import_runs"},
		{kind: "table", name: "import_source_aliases"},
		{kind: "index", name: "idx_import_source_aliases_source"},
		{kind: "index", name: "idx_import_source_aliases_hive"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var name string
			err := d.sqlDB.QueryRow(
				"SELECT name FROM sqlite_master WHERE type = ? AND name = ?", tt.kind, tt.name,
			).Scan(&name)
			require.NoErrorf(t, err, "%s %s should exist", tt.kind, tt.name)
		})
	}
}

func TestImportEngramBatchInsertsRowsAliasesAndMemoryMutation(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	result, err := d.ImportEngramBatch(ctx, ImportRun{
		ID:                "run-1",
		SourceSystem:      "engram",
		SourcePath:        "C:/tmp/engram.db",
		SourceFingerprint: "fingerprint-1",
		Mode:              "execute",
	}, ImportBatch{
		Sessions: []ImportSession{{SourceID: "engram-session-1", Project: "proj", Directory: "C:/src/proj", DevID: "dev", Client: "opencode", StartedAt: "2026-06-11 10:00:00", Summary: "summary"}},
		Prompts:  []ImportPrompt{{SourceID: "42", Project: "proj", Content: "import this", CreatedAt: "2026-06-11 10:01:00", ContentHash: "prompt-hash"}},
		Memories: []ImportMemory{{SourceID: "7", Project: "proj", TopicKey: stringPtr("topic"), Category: "decision", Title: "Imported", Content: "Imported content", SessionSourceID: "engram-session-1", CreatedAt: "2026-06-11 10:02:00", UpdatedAt: "2026-06-11 10:03:00", ContentHash: "memory-hash"}},
	})
	require.NoError(t, err)
	require.Equal(t, ImportCounts{Imported: 3}, result.Counts)

	sessionAlias := requireAlias(t, d.sqlDB, SourceAliasKey{SourceSystem: "engram", SourceTable: "sessions", SourceID: "engram-session-1", SourceProject: "proj"})
	require.Equal(t, "sessions", sessionAlias.HiveTable)
	require.NotEqual(t, "engram-session-1", sessionAlias.HivePK)
	require.NoError(t, uuid.Validate(sessionAlias.HiveSyncID))

	promptAlias := requireAlias(t, d.sqlDB, SourceAliasKey{SourceSystem: "engram", SourceTable: "user_prompts", SourceID: "42", SourceProject: "proj"})
	require.Equal(t, "user_prompts", promptAlias.HiveTable)
	require.NoError(t, uuid.Validate(promptAlias.HiveSyncID))

	memoryAlias := requireAlias(t, d.sqlDB, SourceAliasKey{SourceSystem: "engram", SourceTable: "observations", SourceID: "7", SourceProject: "proj"})
	require.Equal(t, "memories", memoryAlias.HiveTable)
	require.NoError(t, uuid.Validate(memoryAlias.HiveSyncID))
	require.Equal(t, sessionAlias.HivePK, queryString(t, d.sqlDB, `SELECT session_id FROM memories WHERE sync_id = ?`, memoryAlias.HiveSyncID))

	mutationPayload := queryString(t, d.sqlDB, `SELECT payload_json FROM memory_mutations WHERE entity_sync_id = ? AND op = 'create'`, memoryAlias.HiveSyncID)
	var payload struct {
		Memory struct {
			SyncID    string `json:"sync_id"`
			Project   string `json:"project"`
			SessionID string `json:"session_id"`
		} `json:"memory"`
	}
	require.NoError(t, json.Unmarshal([]byte(mutationPayload), &payload))
	require.Equal(t, memoryAlias.HiveSyncID, payload.Memory.SyncID)
	require.Equal(t, "proj", payload.Memory.Project)
	require.Equal(t, sessionAlias.HivePK, payload.Memory.SessionID)
}

func TestImportEngramBatchReusesExistingAliasesWithoutDuplicatingRows(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	batch := ImportBatch{
		Sessions: []ImportSession{{SourceID: "s1", Project: "proj", StartedAt: "2026-06-11 10:00:00"}},
		Prompts:  []ImportPrompt{{SourceID: "p1", Project: "proj", Content: "prompt", CreatedAt: "2026-06-11 10:01:00"}},
		Memories: []ImportMemory{{SourceID: "m1", Project: "proj", Title: "Title", Content: "Body", SessionSourceID: "s1", CreatedAt: "2026-06-11 10:02:00"}},
	}

	first, err := d.ImportEngramBatch(ctx, ImportRun{ID: "run-1", SourceSystem: "engram", SourcePath: "one.db", Mode: "execute"}, batch)
	require.NoError(t, err)
	second, err := d.ImportEngramBatch(ctx, ImportRun{ID: "run-2", SourceSystem: "engram", SourcePath: "one.db", Mode: "execute"}, batch)
	require.NoError(t, err)

	require.Equal(t, ImportCounts{Imported: 3}, first.Counts)
	require.Equal(t, ImportCounts{Reused: 3}, second.Counts)
	require.Equal(t, 1, queryInt(t, d.sqlDB, `SELECT COUNT(*) FROM sessions WHERE project = 'proj'`))
	require.Equal(t, 1, queryInt(t, d.sqlDB, `SELECT COUNT(*) FROM user_prompts WHERE project = 'proj'`))
	require.Equal(t, 1, queryInt(t, d.sqlDB, `SELECT COUNT(*) FROM memories WHERE project = 'proj'`))
}

func TestImportEngramBatchRollsBackWhenMemorySessionAliasIsMissing(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	_, err := d.ImportEngramBatch(ctx, ImportRun{ID: "run-1", SourceSystem: "engram", SourcePath: "bad.db", Mode: "execute"}, ImportBatch{
		Prompts:  []ImportPrompt{{SourceID: "p1", Project: "proj", Content: "prompt", CreatedAt: "2026-06-11 10:01:00"}},
		Memories: []ImportMemory{{SourceID: "m1", Project: "proj", Title: "Title", Content: "Body", SessionSourceID: "missing-session", CreatedAt: "2026-06-11 10:02:00"}},
	})
	require.Error(t, err)
	require.Equal(t, 0, queryInt(t, d.sqlDB, `SELECT COUNT(*) FROM import_runs`))
	require.Equal(t, 0, queryInt(t, d.sqlDB, `SELECT COUNT(*) FROM user_prompts`))
	require.Equal(t, 0, queryInt(t, d.sqlDB, `SELECT COUNT(*) FROM memories`))
	require.Equal(t, 0, queryInt(t, d.sqlDB, `SELECT COUNT(*) FROM import_source_aliases`))
}

func TestImportEngramBatchAllowsDuplicateRunIDRetryWithSameRunMetadata(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	run := ImportRun{ID: "run-1", SourceSystem: "engram", SourcePath: "one.db", SourceFingerprint: "fingerprint-1", Mode: "execute"}
	batch := ImportBatch{Sessions: []ImportSession{{SourceID: "s1", Project: "proj", StartedAt: "2026-06-11 10:00:00", ContentHash: "session-hash"}}}

	first, err := d.ImportEngramBatch(ctx, run, batch)
	require.NoError(t, err)
	second, err := d.ImportEngramBatch(ctx, run, batch)
	require.NoError(t, err)

	require.Equal(t, ImportCounts{Imported: 1}, first.Counts)
	require.Equal(t, ImportCounts{Reused: 1}, second.Counts)
	require.Equal(t, 1, queryInt(t, d.sqlDB, `SELECT COUNT(*) FROM import_runs WHERE id = 'run-1'`))
	require.Equal(t, 1, queryInt(t, d.sqlDB, `SELECT COUNT(*) FROM sessions WHERE project = 'proj'`))
}

func TestImportEngramBatchRejectsDuplicateRunIDWithDifferentMetadata(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	_, err := d.ImportEngramBatch(ctx, ImportRun{ID: "run-1", SourceSystem: "engram", SourcePath: "one.db", SourceFingerprint: "fingerprint-1", Mode: "execute"}, ImportBatch{})
	require.NoError(t, err)

	_, err = d.ImportEngramBatch(ctx, ImportRun{ID: "run-1", SourceSystem: "engram", SourcePath: "two.db", SourceFingerprint: "fingerprint-2", Mode: "execute"}, ImportBatch{})
	require.ErrorIs(t, err, ErrImportRunConflict)
}

func TestImportEngramBatchRejectsExistingAliasWithChangedContentHash(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	_, err := d.ImportEngramBatch(ctx, ImportRun{ID: "run-1", SourceSystem: "engram", SourcePath: "one.db", Mode: "execute"}, ImportBatch{
		Prompts: []ImportPrompt{{SourceID: "p1", Project: "proj", Content: "prompt", CreatedAt: "2026-06-11 10:01:00", ContentHash: "hash-1"}},
	})
	require.NoError(t, err)

	_, err = d.ImportEngramBatch(ctx, ImportRun{ID: "run-2", SourceSystem: "engram", SourcePath: "one.db", Mode: "execute"}, ImportBatch{
		Prompts: []ImportPrompt{{SourceID: "p1", Project: "proj", Content: "changed prompt", CreatedAt: "2026-06-11 10:01:00", ContentHash: "hash-2"}},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrImportAliasContentChanged))
	require.ErrorContains(t, err, "user_prompts/p1")
}

func TestImportEngramBatchReportsAmbiguousMemoryDuplicateWithoutAlias(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	require.NoError(t, d.CreateSession("existing-session", "proj", "/repo/proj", "tester", "test"))
	_, err := d.SaveMemory(&models.Memory{Project: "proj", Title: "Existing duplicate", Content: "first existing", SessionID: "existing-session"})
	require.NoError(t, err)
	_, err = d.SaveMemory(&models.Memory{Project: "proj", Title: "Existing duplicate", Content: "second existing", SessionID: "existing-session"})
	require.NoError(t, err)

	result, err := d.ImportEngramBatch(ctx, ImportRun{ID: "run-ambiguous", SourceSystem: "engram", SourcePath: "one.db", Mode: "execute"}, ImportBatch{
		Sessions: []ImportSession{{SourceID: "s1", Project: "proj", StartedAt: "2026-06-11 10:00:00"}},
		Memories: []ImportMemory{{SourceID: "m1", Project: "proj", Title: "Existing duplicate", Content: "imported content", SessionSourceID: "s1", CreatedAt: "2026-06-11 10:02:00"}},
	})
	require.NoError(t, err)

	require.Equal(t, ImportCounts{Imported: 1, Ambiguous: 1}, result.Counts)
	require.Equal(t, []ImportAmbiguousDuplicate{{SourceID: "m1", Project: "proj", Title: "Existing duplicate", Reason: "multiple active Hive memories match project and title"}}, result.AmbiguousDuplicates)
	require.Equal(t, 2, queryInt(t, d.sqlDB, `SELECT COUNT(*) FROM memories WHERE project = 'proj' AND title = 'Existing duplicate'`))
	require.Equal(t, 0, queryInt(t, d.sqlDB, `SELECT COUNT(*) FROM import_source_aliases WHERE source_table = 'observations' AND source_id = 'm1'`))
}

func requireAlias(t *testing.T, sqlDB *sql.DB, key SourceAliasKey) ImportSourceAlias {
	t.Helper()
	var alias ImportSourceAlias
	err := sqlDB.QueryRow(`
SELECT source_system, source_table, source_id, source_project, hive_table, hive_pk, hive_sync_id, content_hash, run_id
FROM import_source_aliases
WHERE source_system = ? AND source_table = ? AND source_id = ? AND source_project = ?`,
		key.SourceSystem, key.SourceTable, key.SourceID, key.SourceProject,
	).Scan(&alias.SourceSystem, &alias.SourceTable, &alias.SourceID, &alias.SourceProject, &alias.HiveTable, &alias.HivePK, &alias.HiveSyncID, &alias.ContentHash, &alias.RunID)
	require.NoError(t, err)
	return alias
}

func queryString(t *testing.T, sqlDB *sql.DB, query string, args ...any) string {
	t.Helper()
	var value string
	require.NoError(t, sqlDB.QueryRow(query, args...).Scan(&value))
	return value
}

func queryInt(t *testing.T, sqlDB *sql.DB, query string, args ...any) int {
	t.Helper()
	var value int
	require.NoError(t, sqlDB.QueryRow(query, args...).Scan(&value))
	return value
}
