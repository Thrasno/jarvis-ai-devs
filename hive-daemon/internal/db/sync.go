package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

// GetUnsynced devuelve todas las memorias que aún no se han enviado al servidor
// (synced_at IS NULL). Son las que hay que incluir en el próximo push.
func (d *DB) GetUnsynced(project string) ([]*models.Memory, error) {
	q := `
SELECT id, sync_id, project, topic_key, category, title, content, tags, files_affected,
       created_by, created_at, updated_at, synced_at, confidence, impact_score
FROM memories
WHERE synced_at IS NULL`

	args := []any{}
	if project != "" {
		q += " AND project = ?"
		args = append(args, project)
	}
	q += " ORDER BY created_at ASC"

	rows, err := d.sqlDB.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("get unsynced: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*models.Memory
	for rows.Next() {
		mem, err := scanSyncRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan unsynced row: %w", err)
		}
		results = append(results, mem)
	}
	return results, rows.Err()
}

// MarkSynced marca una memoria como sincronizada con el servidor.
func (d *DB) MarkSynced(syncID string, at time.Time) error {
	_, err := d.sqlDB.Exec(
		`UPDATE memories SET synced_at = ? WHERE sync_id = ?`,
		at.UTC().Format("2006-01-02 15:04:05"), syncID,
	)
	return err
}

// SaveFromRemote guarda una memoria recibida del servidor (pull).
// La marca como ya sincronizada para no reenviarla en el próximo push.
// INSERT OR IGNORE: si el sync_id ya existe localmente, no tocamos nada.
func (d *DB) SaveFromRemote(mem *models.Memory) error {
	tagsJSON, err := json.Marshal(orNil(mem.Tags))
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}
	filesJSON, err := json.Marshal(orNil(mem.FilesAffected))
	if err != nil {
		return fmt.Errorf("marshal files: %w", err)
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	createdAt := mem.CreatedAt.UTC().Format("2006-01-02 15:04:05")
	updatedAt := mem.UpdatedAt.UTC().Format("2006-01-02 15:04:05")

	_, err = d.sqlDB.Exec(`
INSERT OR IGNORE INTO memories
    (sync_id, project, topic_key, category, title, content, tags, files_affected,
     created_by, created_at, updated_at, synced_at, confidence, impact_score)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mem.SyncID, mem.Project, mem.TopicKey, mem.Category,
		mem.Title, mem.Content, string(tagsJSON), string(filesJSON),
		mem.CreatedBy, createdAt, updatedAt, now,
		mem.Confidence, mem.ImpactScore,
	)
	return err
}

// GetLastSync devuelve el timestamp del último sync exitoso para un proyecto.
func (d *DB) GetLastSync(project string) (time.Time, error) {
	var ts sql.NullString
	err := d.sqlDB.QueryRow(
		`SELECT last_sync_at FROM sync_state WHERE project = ?`, project,
	).Scan(&ts)
	if err == sql.ErrNoRows || !ts.Valid {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return parseTimeStr(ts.String)
}

// SetLastSync actualiza el timestamp del último sync para un proyecto.
func (d *DB) SetLastSync(project string, at time.Time) error {
	_, err := d.sqlDB.Exec(`
INSERT INTO sync_state (project, last_sync_at)
VALUES (?, ?)
ON CONFLICT(project) DO UPDATE SET last_sync_at = excluded.last_sync_at`,
		project, at.UTC().Format("2006-01-02 15:04:05"),
	)
	return err
}

// GetJWT devuelve el JWT almacenado si aún es válido (margen de 1 hora).
func (d *DB) GetJWT() string {
	var token, expires sql.NullString
	err := d.sqlDB.QueryRow(
		`SELECT jwt_token, jwt_expires_at FROM sync_state WHERE project = '__auth__'`,
	).Scan(&token, &expires)
	if err != nil || !token.Valid || !expires.Valid {
		return ""
	}
	exp, err := parseTimeStr(expires.String)
	if err != nil || time.Now().Add(time.Hour).After(exp) {
		return ""
	}
	return token.String
}

// SetJWT almacena el JWT con su fecha de expiración.
func (d *DB) SetJWT(token string, expiresAt time.Time) error {
	_, err := d.sqlDB.Exec(`
INSERT INTO sync_state (project, jwt_token, jwt_expires_at)
VALUES ('__auth__', ?, ?)
ON CONFLICT(project) DO UPDATE SET
    jwt_token      = excluded.jwt_token,
    jwt_expires_at = excluded.jwt_expires_at`,
		token, expiresAt.UTC().Format("2006-01-02 15:04:05"),
	)
	return err
}

// --- helpers privados ---

type syncScanner interface {
	Scan(...any) error
}

func scanSyncRow(s syncScanner) (*models.Memory, error) {
	var (
		mem          models.Memory
		topicKey     sql.NullString
		tagsJSON     string
		filesJSON    string
		createdAtStr string
		updatedAtStr string
		syncedAtStr  sql.NullString
	)

	err := s.Scan(
		&mem.ID, &mem.SyncID, &mem.Project, &topicKey,
		&mem.Category, &mem.Title, &mem.Content,
		&tagsJSON, &filesJSON,
		&mem.CreatedBy, &createdAtStr, &updatedAtStr, &syncedAtStr,
		&mem.Confidence, &mem.ImpactScore,
	)
	if err != nil {
		return nil, err
	}

	if topicKey.Valid {
		mem.TopicKey = &topicKey.String
	}

	mem.CreatedAt, _ = parseTimeStr(createdAtStr)
	mem.UpdatedAt, _ = parseTimeStr(updatedAtStr)

	if syncedAtStr.Valid {
		t, _ := parseTimeStr(syncedAtStr.String)
		mem.SyncedAt = &t
	}

	_ = json.Unmarshal([]byte(tagsJSON), &mem.Tags)
	_ = json.Unmarshal([]byte(filesJSON), &mem.FilesAffected)

	return &mem, nil
}

func parseTimeStr(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
	}
	return t, err
}

func orNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
