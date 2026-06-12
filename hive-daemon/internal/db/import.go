package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/google/uuid"
)

const importActorID = "engram-import"

var (
	ErrImportSessionAliasNotFound = errors.New("import session alias not found")
	ErrImportRunConflict          = errors.New("import run id already exists with different metadata")
	ErrImportAliasContentChanged  = errors.New("import source alias content hash changed")
)

type ImportRun struct {
	ID                string
	SourceSystem      string
	SourcePath        string
	SourceFingerprint string
	Mode              string
}

type SourceAliasKey struct {
	SourceSystem  string
	SourceTable   string
	SourceID      string
	SourceProject string
}

type ImportSourceAlias struct {
	SourceSystem  string
	SourceTable   string
	SourceID      string
	SourceProject string
	HiveTable     string
	HivePK        string
	HiveSyncID    string
	ContentHash   string
	RunID         string
}

type ImportSession struct {
	SourceID    string
	Project     string
	Directory   string
	DevID       string
	Client      string
	StartedAt   string
	EndedAt     string
	Summary     string
	ContentHash string
}

type ImportPrompt struct {
	SourceID    string
	Project     string
	Content     string
	CreatedAt   string
	ContentHash string
}

type ImportMemory struct {
	SourceID        string
	Project         string
	TopicKey        *string
	Category        string
	Title           string
	Content         string
	SessionSourceID string
	CreatedAt       string
	UpdatedAt       string
	ContentHash     string
}

type ImportBatch struct {
	Sessions []ImportSession
	Prompts  []ImportPrompt
	Memories []ImportMemory
}

type ImportCounts struct {
	Imported  int
	Reused    int
	Ambiguous int
}

type ImportAmbiguousDuplicate struct {
	SourceID string `json:"source_id"`
	Project  string `json:"project"`
	Title    string `json:"title"`
	Reason   string `json:"reason"`
}

type ImportResult struct {
	RunID               string
	Counts              ImportCounts
	AmbiguousDuplicates []ImportAmbiguousDuplicate
}

func (d *DB) ImportEngramBatch(ctx context.Context, run ImportRun, batch ImportBatch) (ImportResult, error) {
	run = normalizeImportRun(run)
	tx, err := d.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, fmt.Errorf("begin import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := insertImportRun(ctx, tx, run); err != nil {
		return ImportResult{}, err
	}

	result := ImportResult{RunID: run.ID}
	for _, session := range batch.Sessions {
		imported, err := importSession(ctx, tx, run, session)
		if err != nil {
			return ImportResult{}, err
		}
		addImportCount(&result.Counts, imported)
	}
	for _, prompt := range batch.Prompts {
		imported, err := importPrompt(ctx, tx, run, prompt)
		if err != nil {
			return ImportResult{}, err
		}
		addImportCount(&result.Counts, imported)
	}
	for _, memory := range batch.Memories {
		imported, ambiguous, err := importMemory(ctx, tx, run, memory)
		if err != nil {
			return ImportResult{}, err
		}
		if ambiguous {
			result.Counts.Ambiguous++
			result.AmbiguousDuplicates = append(result.AmbiguousDuplicates, ImportAmbiguousDuplicate{SourceID: memory.SourceID, Project: memory.Project, Title: memory.Title, Reason: "multiple active Hive memories match project and title"})
		} else {
			addImportCount(&result.Counts, imported)
		}
	}

	if err := tx.Commit(); err != nil {
		return ImportResult{}, fmt.Errorf("commit import: %w", err)
	}
	return result, nil
}

func normalizeImportRun(run ImportRun) ImportRun {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	if run.SourceSystem == "" {
		run.SourceSystem = "engram"
	}
	if run.Mode == "" {
		run.Mode = "execute"
	}
	return run
}

func insertImportRun(ctx context.Context, tx *sql.Tx, run ImportRun) error {
	var existing ImportRun
	err := tx.QueryRowContext(ctx, `
SELECT id, source_system, source_path, source_fingerprint, mode
FROM import_runs
WHERE id = ?`, run.ID).Scan(&existing.ID, &existing.SourceSystem, &existing.SourcePath, &existing.SourceFingerprint, &existing.Mode)
	if err == nil {
		if existing.SourceSystem == run.SourceSystem && existing.SourcePath == run.SourcePath && existing.SourceFingerprint == run.SourceFingerprint && existing.Mode == run.Mode {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrImportRunConflict, run.ID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read import run: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO import_runs (id, source_system, source_path, source_fingerprint, mode, status, completed_at, report_json)
VALUES (?, ?, ?, ?, ?, 'completed', CURRENT_TIMESTAMP, '{}')`,
		run.ID, run.SourceSystem, run.SourcePath, run.SourceFingerprint, run.Mode,
	)
	if err != nil {
		return fmt.Errorf("insert import run: %w", err)
	}
	return nil
}

func importSession(ctx context.Context, tx *sql.Tx, run ImportRun, session ImportSession) (bool, error) {
	key := SourceAliasKey{SourceSystem: run.SourceSystem, SourceTable: "sessions", SourceID: session.SourceID, SourceProject: session.Project}
	if alias, found, err := findImportAlias(ctx, tx, key); err != nil || found {
		return false, validateReusedImportAlias(alias, found, session.ContentHash, err)
	}

	hiveID := "import-engram-session-" + uuid.NewString()
	syncID := uuid.NewString()
	_, err := tx.ExecContext(ctx, `
INSERT INTO sessions (id, sync_id, project, directory, dev_id, client, started_at, ended_at, summary)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		hiveID, syncID, session.Project, session.Directory, defaultString(session.DevID, importActorID), defaultString(session.Client, "engram"), defaultTime(session.StartedAt), nullEmpty(session.EndedAt), nullEmpty(session.Summary),
	)
	if err != nil {
		return false, fmt.Errorf("insert imported session %s: %w", session.SourceID, err)
	}
	return true, insertImportAlias(ctx, tx, run, key, "sessions", hiveID, syncID, session.ContentHash)
}

func importPrompt(ctx context.Context, tx *sql.Tx, run ImportRun, prompt ImportPrompt) (bool, error) {
	key := SourceAliasKey{SourceSystem: run.SourceSystem, SourceTable: "user_prompts", SourceID: prompt.SourceID, SourceProject: prompt.Project}
	if alias, found, err := findImportAlias(ctx, tx, key); err != nil || found {
		return false, validateReusedImportAlias(alias, found, prompt.ContentHash, err)
	}

	syncID := uuid.NewString()
	res, err := tx.ExecContext(ctx, `
INSERT INTO user_prompts (sync_id, project, content, created_at)
VALUES (?, ?, ?, ?)`, syncID, prompt.Project, prompt.Content, defaultTime(prompt.CreatedAt))
	if err != nil {
		return false, fmt.Errorf("insert imported prompt %s: %w", prompt.SourceID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return false, fmt.Errorf("read imported prompt id: %w", err)
	}
	return true, insertImportAlias(ctx, tx, run, key, "user_prompts", strconv.FormatInt(id, 10), syncID, prompt.ContentHash)
}

func importMemory(ctx context.Context, tx *sql.Tx, run ImportRun, memory ImportMemory) (bool, bool, error) {
	key := SourceAliasKey{SourceSystem: run.SourceSystem, SourceTable: "observations", SourceID: memory.SourceID, SourceProject: memory.Project}
	if alias, found, err := findImportAlias(ctx, tx, key); err != nil || found {
		return false, false, validateReusedImportAlias(alias, found, memory.ContentHash, err)
	}
	ambiguous, err := ambiguousMemoryDuplicate(ctx, tx, memory)
	if err != nil {
		return false, false, err
	}
	if ambiguous {
		return false, true, nil
	}

	sessionAlias, found, err := findImportAlias(ctx, tx, SourceAliasKey{SourceSystem: run.SourceSystem, SourceTable: "sessions", SourceID: memory.SessionSourceID, SourceProject: memory.Project})
	if err != nil {
		return false, false, err
	}
	if !found {
		return false, false, fmt.Errorf("%w: %s", ErrImportSessionAliasNotFound, memory.SessionSourceID)
	}

	syncID := uuid.NewString()
	createdAt := defaultTime(memory.CreatedAt)
	updatedAt := defaultString(memory.UpdatedAt, createdAt)
	res, err := tx.ExecContext(ctx, `
INSERT INTO memories
    (sync_id, project, topic_key, category, title, content, tags, files_affected, created_by, created_at, updated_at, session_id)
VALUES (?, ?, ?, ?, ?, ?, '[]', '[]', ?, ?, ?, ?)`,
		syncID, memory.Project, memory.TopicKey, memory.Category, memory.Title, memory.Content, importActorID, createdAt, updatedAt, sessionAlias.HivePK,
	)
	if err != nil {
		return false, false, fmt.Errorf("insert imported memory %s: %w", memory.SourceID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return false, false, fmt.Errorf("read imported memory id: %w", err)
	}
	mem := &models.Memory{Project: memory.Project, TopicKey: memory.TopicKey, Category: memory.Category, Title: memory.Title, Content: memory.Content, CreatedBy: importActorID, CreatedAt: parseImportTime(createdAt), UpdatedAt: parseImportTime(updatedAt), SessionID: sessionAlias.HivePK}
	if err := insertMemoryMutation(tx, memoryMutationRecord{EventID: uuid.NewString(), EntitySyncID: syncID, Project: memory.Project, Op: MutationOpCreate, OccurredAt: updatedAt, ActorID: importActorID, Payload: mutationPayload{Memory: memoryPayloadFromModel(mem, syncID, importActorID, parseImportTime(updatedAt))}}); err != nil {
		return false, false, fmt.Errorf("journal imported memory mutation: %w", err)
	}
	return true, false, insertImportAlias(ctx, tx, run, key, "memories", strconv.FormatInt(id, 10), syncID, memory.ContentHash)
}

func ambiguousMemoryDuplicate(ctx context.Context, tx *sql.Tx, memory ImportMemory) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM memories
WHERE project = ? AND title = ? AND deleted_at IS NULL`, memory.Project, memory.Title).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("detect ambiguous imported memory %s: %w", memory.SourceID, err)
	}
	return count > 1, nil
}

func validateReusedImportAlias(alias ImportSourceAlias, found bool, contentHash string, err error) error {
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if alias.ContentHash != "" && contentHash != "" && alias.ContentHash != contentHash {
		return fmt.Errorf("%w: %s/%s project %s", ErrImportAliasContentChanged, alias.SourceTable, alias.SourceID, alias.SourceProject)
	}
	return nil
}

func addImportCount(counts *ImportCounts, imported bool) {
	if imported {
		counts.Imported++
	} else {
		counts.Reused++
	}
}

func findImportAlias(ctx context.Context, tx *sql.Tx, key SourceAliasKey) (ImportSourceAlias, bool, error) {
	var alias ImportSourceAlias
	err := tx.QueryRowContext(ctx, `
SELECT source_system, source_table, source_id, source_project, hive_table, hive_pk, hive_sync_id, content_hash, run_id
FROM import_source_aliases
WHERE source_system = ? AND source_table = ? AND source_id = ? AND source_project = ?`,
		key.SourceSystem, key.SourceTable, key.SourceID, key.SourceProject,
	).Scan(&alias.SourceSystem, &alias.SourceTable, &alias.SourceID, &alias.SourceProject, &alias.HiveTable, &alias.HivePK, &alias.HiveSyncID, &alias.ContentHash, &alias.RunID)
	if errors.Is(err, sql.ErrNoRows) {
		return ImportSourceAlias{}, false, nil
	}
	if err != nil {
		return ImportSourceAlias{}, false, fmt.Errorf("find import alias: %w", err)
	}
	return alias, true, nil
}

func insertImportAlias(ctx context.Context, tx *sql.Tx, run ImportRun, key SourceAliasKey, hiveTable, hivePK, hiveSyncID, contentHash string) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO import_source_aliases
    (source_system, source_table, source_id, source_project, hive_table, hive_pk, hive_sync_id, content_hash, run_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key.SourceSystem, key.SourceTable, key.SourceID, key.SourceProject, hiveTable, hivePK, hiveSyncID, contentHash, run.ID,
	)
	if err != nil {
		return fmt.Errorf("insert import alias: %w", err)
	}
	return nil
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func defaultTime(value string) string {
	if value != "" {
		return value
	}
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}

func nullEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func parseImportTime(value string) time.Time {
	if parsed, ok := parseDBTimestamp("import_time", value); ok {
		return parsed
	}
	return time.Now().UTC()
}
