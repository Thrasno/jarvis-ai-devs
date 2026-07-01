package engramimport

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	hivedb "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	_ "modernc.org/sqlite"
)

var (
	ErrSourceNotFound = errors.New("engram source not found")
	ErrInvalidSchema  = errors.New("invalid engram schema")
)

type SourceOptions struct {
	ExplicitPath string
	EnvDataDir   string
	HomeDir      string
}

type Source struct {
	Path         string
	CheckedPaths []string
}

type Counts struct {
	Sessions     int
	Prompts      int
	Observations int
}

type ProjectImpact struct {
	Project string
	Counts  Counts
}

type InvalidRow struct {
	Table    string `json:"table"`
	SourceID string `json:"source_id"`
	Reason   string `json:"reason"`
}

type Analysis struct {
	SourcePath         string
	SourceFingerprint  string
	Projects           []string
	ProjectedByProject []ProjectImpact
	Counts             Counts
	SkippedRelations   int
	InvalidRows        []InvalidRow
	Sessions           []hivedb.ImportSession
	Prompts            []hivedb.ImportPrompt
	Memories           []hivedb.ImportMemory
}

type ImportRequest struct {
	Source Source
	RunID  string
}

type ImportReport struct {
	SourcePath          string
	SourceFingerprint   string
	Counts              hivedb.ImportCounts
	AmbiguousDuplicates []hivedb.ImportAmbiguousDuplicate
	ProjectedByProject  []ProjectImpact
	SkippedRelations    int
	InvalidRows         []InvalidRow
}

type sourceSnapshot struct {
	path        string
	fingerprint string
	cleanup     func()
}

func ResolveSource(options SourceOptions) (Source, error) {
	checked := make([]string, 0, 3)
	if strings.TrimSpace(options.ExplicitPath) != "" {
		path := filepath.Clean(options.ExplicitPath)
		checked = append(checked, path)
		if readableFile(path) {
			return Source{Path: path, CheckedPaths: checked}, nil
		}
		return Source{}, fmt.Errorf("%w: checked %s", ErrSourceNotFound, strings.Join(checked, ", "))
	}
	if strings.TrimSpace(options.EnvDataDir) != "" {
		checked = append(checked, filepath.Join(options.EnvDataDir, "engram.db"))
	}
	home := options.HomeDir
	if home == "" {
		if detected, err := os.UserHomeDir(); err == nil {
			home = detected
		}
	}
	if home != "" {
		checked = append(checked, filepath.Join(home, ".engram", "engram.db"))
	}
	for _, path := range checked {
		if readableFile(path) {
			return Source{Path: path, CheckedPaths: checked}, nil
		}
	}
	return Source{}, fmt.Errorf("%w: checked %s", ErrSourceNotFound, strings.Join(checked, ", "))
}

func AnalyzeSource(ctx context.Context, source Source) (Analysis, error) {
	snapshot, err := createSourceSnapshot(ctx, source.Path)
	if err != nil {
		return Analysis{}, err
	}
	defer snapshot.cleanup()

	sqlDB, err := openReadOnly(snapshot.path)
	if err != nil {
		return Analysis{}, err
	}
	defer sqlDB.Close()

	if err := validateSchema(ctx, sqlDB); err != nil {
		return Analysis{}, err
	}
	analysis := Analysis{SourcePath: source.Path, SourceFingerprint: snapshot.fingerprint}
	projects := map[string]struct{}{}
	validSessions := map[string]map[string]struct{}{}
	if err := readSessions(ctx, sqlDB, &analysis, projects, validSessions); err != nil {
		return Analysis{}, err
	}
	if err := readPrompts(ctx, sqlDB, &analysis, projects); err != nil {
		return Analysis{}, err
	}
	if err := readObservations(ctx, sqlDB, &analysis, projects, validSessions); err != nil {
		return Analysis{}, err
	}
	analysis.SkippedRelations = relationCount(ctx, sqlDB)
	for project := range projects {
		analysis.Projects = append(analysis.Projects, project)
	}
	sort.Strings(analysis.Projects)
	analysis.ProjectedByProject = projectedByProject(analysis)
	return analysis, nil
}

func projectedByProject(analysis Analysis) []ProjectImpact {
	countsByProject := make(map[string]Counts)
	for _, session := range analysis.Sessions {
		counts := countsByProject[session.Project]
		counts.Sessions++
		countsByProject[session.Project] = counts
	}
	for _, prompt := range analysis.Prompts {
		counts := countsByProject[prompt.Project]
		counts.Prompts++
		countsByProject[prompt.Project] = counts
	}
	for _, memory := range analysis.Memories {
		counts := countsByProject[memory.Project]
		counts.Observations++
		countsByProject[memory.Project] = counts
	}
	projects := make([]string, 0, len(countsByProject))
	for project := range countsByProject {
		projects = append(projects, project)
	}
	sort.Strings(projects)
	impacts := make([]ProjectImpact, 0, len(projects))
	for _, project := range projects {
		impacts = append(impacts, ProjectImpact{Project: project, Counts: countsByProject[project]})
	}
	return impacts
}

func BuildImportBatch(analysis Analysis) hivedb.ImportBatch {
	return hivedb.ImportBatch{Sessions: analysis.Sessions, Prompts: analysis.Prompts, Memories: analysis.Memories}
}

func ImportSource(ctx context.Context, hive *hivedb.DB, request ImportRequest) (ImportReport, error) {
	analysis, err := AnalyzeSource(ctx, request.Source)
	if err != nil {
		return ImportReport{}, err
	}
	result, err := hive.ImportEngramBatch(ctx, hivedb.ImportRun{ID: request.RunID, SourceSystem: "engram", SourcePath: analysis.SourcePath, SourceFingerprint: analysis.SourceFingerprint, Mode: "execute"}, BuildImportBatch(analysis))
	if err != nil {
		return ImportReport{}, err
	}
	return ImportReport{SourcePath: analysis.SourcePath, SourceFingerprint: analysis.SourceFingerprint, Counts: result.Counts, AmbiguousDuplicates: result.AmbiguousDuplicates, ProjectedByProject: analysis.ProjectedByProject, SkippedRelations: analysis.SkippedRelations, InvalidRows: analysis.InvalidRows}, nil
}

func createSourceSnapshot(ctx context.Context, path string) (sourceSnapshot, error) {
	tempDir, err := os.MkdirTemp("", "jarvis-engram-import-*")
	if err != nil {
		return sourceSnapshot{}, fmt.Errorf("prepare engram source snapshot: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	snapshotPath := filepath.Join(tempDir, filepath.Base(path))

	sqlDB, err := sql.Open("sqlite", sqliteFileURI(path))
	if err != nil {
		cleanup()
		return sourceSnapshot{}, fmt.Errorf("open engram source for snapshot: %w", err)
	}
	defer sqlDB.Close()
	if _, err := sqlDB.ExecContext(ctx, `PRAGMA busy_timeout=5000`); err != nil {
		cleanup()
		return sourceSnapshot{}, fmt.Errorf("configure engram source snapshot: %w", err)
	}
	if _, err := sqlDB.ExecContext(ctx, `VACUUM main INTO ?`, snapshotPath); err != nil {
		cleanup()
		return sourceSnapshot{}, fmt.Errorf("snapshot engram source: %w", err)
	}
	return sourceSnapshot{path: snapshotPath, fingerprint: fingerprint(snapshotPath), cleanup: cleanup}, nil
}

func openReadOnly(path string) (*sql.DB, error) {
	dsn := sqliteFileURI(path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open engram source: %w", err)
	}
	if _, err := sqlDB.Exec(`PRAGMA query_only=ON; PRAGMA busy_timeout=5000;`); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("configure engram read-only source: %w", err)
	}
	return sqlDB, nil
}

func sqliteFileURI(path string) string {
	escapedPath := strings.ReplaceAll(url.PathEscape(filepath.ToSlash(path)), "%2F", "/")
	return "file:" + escapedPath + "?mode=ro"
}

func validateSchema(ctx context.Context, sqlDB *sql.DB) error {
	missing := make([]string, 0)
	for _, table := range []string{"observations", "sessions", "user_prompts"} {
		if !tableExists(ctx, sqlDB, table) {
			missing = append(missing, table)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing required tables: %s", ErrInvalidSchema, strings.Join(missing, ", "))
	}
	return nil
}

func readSessions(ctx context.Context, sqlDB *sql.DB, analysis *Analysis, projects map[string]struct{}, validSessions map[string]map[string]struct{}) error {
	columns, err := existingColumns(ctx, sqlDB, "sessions")
	if err != nil {
		return err
	}
	// id and project carry session identity; without them a row is meaningless.
	for _, required := range []string{"id", "project"} {
		if _, ok := columns[required]; !ok {
			return fmt.Errorf("%w: sessions missing required column %s", ErrInvalidSchema, required)
		}
	}
	// Optional metadata columns vary across Engram versions; default when absent
	// so schema drift does not break the import.
	query := fmt.Sprintf(
		`SELECT id, project, %s, %s, %s, %s, %s FROM sessions ORDER BY id`,
		optionalColumn(columns, "directory", "''"),
		optionalColumn(columns, "client", "''"),
		optionalColumn(columns, "started_at", "''"),
		optionalColumn(columns, "ended_at", "NULL"),
		optionalColumn(columns, "summary", "NULL"),
	)
	rows, err := sqlDB.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("read engram sessions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		// Engram allows NULL in these text columns, so scan through NullString
		// and normalize; scanning directly into string would fail on NULL before
		// the required-field validation below can skip the row.
		var idVal, projectVal, directoryVal, clientVal, startedAtVal, endedAt, summary sql.NullString
		if err := rows.Scan(&idVal, &projectVal, &directoryVal, &clientVal, &startedAtVal, &endedAt, &summary); err != nil {
			return fmt.Errorf("scan engram session: %w", err)
		}
		id := nullableString(idVal)
		project := nullableString(projectVal)
		directory := nullableString(directoryVal)
		client := nullableString(clientVal)
		startedAt := nullableString(startedAtVal)
		if strings.TrimSpace(id) == "" || strings.TrimSpace(project) == "" {
			analysis.InvalidRows = append(analysis.InvalidRows, InvalidRow{Table: "sessions", SourceID: id, Reason: "id and project are required"})
			continue
		}
		projects[project] = struct{}{}
		if validSessions[project] == nil {
			validSessions[project] = map[string]struct{}{}
		}
		validSessions[project][id] = struct{}{}
		analysis.Counts.Sessions++
		analysis.Sessions = append(analysis.Sessions, hivedb.ImportSession{SourceID: id, Project: project, Directory: directory, Client: client, StartedAt: startedAt, EndedAt: nullableString(endedAt), Summary: nullableString(summary), ContentHash: hashStrings(id, project, directory, client, startedAt, nullableString(endedAt), nullableString(summary))})
	}
	return rows.Err()
}

func readPrompts(ctx context.Context, sqlDB *sql.DB, analysis *Analysis, projects map[string]struct{}) error {
	rows, err := sqlDB.QueryContext(ctx, `SELECT id, project, content, created_at FROM user_prompts ORDER BY id`)
	if err != nil {
		return fmt.Errorf("read engram prompts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var projectVal, contentVal, createdAtVal sql.NullString
		if err := rows.Scan(&id, &projectVal, &contentVal, &createdAtVal); err != nil {
			return fmt.Errorf("scan engram prompt: %w", err)
		}
		project := nullableString(projectVal)
		content := nullableString(contentVal)
		createdAt := nullableString(createdAtVal)
		sourceID := strconv.FormatInt(id, 10)
		if strings.TrimSpace(project) == "" || strings.TrimSpace(content) == "" {
			analysis.InvalidRows = append(analysis.InvalidRows, InvalidRow{Table: "user_prompts", SourceID: sourceID, Reason: "project and content are required"})
			continue
		}
		projects[project] = struct{}{}
		analysis.Counts.Prompts++
		analysis.Prompts = append(analysis.Prompts, hivedb.ImportPrompt{SourceID: sourceID, Project: project, Content: content, CreatedAt: createdAt, ContentHash: hashStrings(sourceID, project, content, createdAt)})
	}
	return rows.Err()
}

func readObservations(ctx context.Context, sqlDB *sql.DB, analysis *Analysis, projects map[string]struct{}, validSessions map[string]map[string]struct{}) error {
	rows, err := sqlDB.QueryContext(ctx, `SELECT id, project, title, content, type, topic_key, session_id, created_at, updated_at FROM observations ORDER BY id`)
	if err != nil {
		return fmt.Errorf("read engram observations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var projectVal, titleVal, contentVal, categoryVal, createdAtVal sql.NullString
		var topicKey, sessionID, updatedAt sql.NullString
		if err := rows.Scan(&id, &projectVal, &titleVal, &contentVal, &categoryVal, &topicKey, &sessionID, &createdAtVal, &updatedAt); err != nil {
			return fmt.Errorf("scan engram observation: %w", err)
		}
		project := nullableString(projectVal)
		title := nullableString(titleVal)
		content := nullableString(contentVal)
		category := nullableString(categoryVal)
		createdAt := nullableString(createdAtVal)
		sourceID := strconv.FormatInt(id, 10)
		if strings.TrimSpace(project) == "" || strings.TrimSpace(title) == "" || strings.TrimSpace(content) == "" || !sessionID.Valid || strings.TrimSpace(sessionID.String) == "" {
			analysis.InvalidRows = append(analysis.InvalidRows, InvalidRow{Table: "observations", SourceID: sourceID, Reason: "project, title, content, and session_id are required"})
			continue
		}
		if _, ok := validSessions[project][sessionID.String]; !ok {
			analysis.InvalidRows = append(analysis.InvalidRows, InvalidRow{Table: "observations", SourceID: sourceID, Reason: "session_id references missing or skipped session"})
			continue
		}
		projects[project] = struct{}{}
		analysis.Counts.Observations++
		analysis.Memories = append(analysis.Memories, hivedb.ImportMemory{SourceID: sourceID, Project: project, TopicKey: nullableStringPtr(topicKey), Category: category, Title: title, Content: content, SessionSourceID: sessionID.String, CreatedAt: createdAt, UpdatedAt: nullableString(updatedAt), ContentHash: hashStrings(sourceID, project, title, content, category, nullableString(topicKey), sessionID.String, createdAt, nullableString(updatedAt))})
	}
	return rows.Err()
}

func relationCount(ctx context.Context, sqlDB *sql.DB) int {
	if !tableExists(ctx, sqlDB, "memory_relations") {
		return 0
	}
	var count int
	_ = sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_relations`).Scan(&count)
	return count
}

// existingColumns returns the set of column names for table. The table name is
// always a hardcoded constant, so interpolating it into the PRAGMA is safe.
func existingColumns(ctx context.Context, sqlDB *sql.DB, table string) (map[string]struct{}, error) {
	rows, err := sqlDB.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, fmt.Errorf("inspect %s columns: %w", table, err)
	}
	defer rows.Close()
	columns := make(map[string]struct{})
	for rows.Next() {
		var (
			cid          int
			name, ctype  string
			notNull, pk  int
			defaultValue sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultValue, &pk); err != nil {
			return nil, fmt.Errorf("scan %s column info: %w", table, err)
		}
		columns[name] = struct{}{}
	}
	return columns, rows.Err()
}

// optionalColumn returns the column name when present, otherwise a defaulted
// SQL literal aliased to the column name so the scan layout stays stable.
func optionalColumn(columns map[string]struct{}, name, fallback string) string {
	if _, ok := columns[name]; ok {
		return name
	}
	return fallback + " AS " + name
}

func tableExists(ctx context.Context, sqlDB *sql.DB, table string) bool {
	var name string
	return sqlDB.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name) == nil
}

func readableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func fingerprint(path string) string {
	h := sha256.New()
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if !readableFile(candidate) {
			continue
		}
		_, _ = h.Write([]byte(filepath.Base(candidate)))
		_, _ = h.Write([]byte{0})
		file, err := os.Open(candidate)
		if err != nil {
			return ""
		}
		_, _ = io.Copy(h, file)
		_ = file.Close()
		_, _ = h.Write([]byte{0})
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

func hashStrings(values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

func nullableString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func nullableStringPtr(value sql.NullString) *string {
	if !value.Valid || value.String == "" {
		return nil
	}
	return &value.String
}
