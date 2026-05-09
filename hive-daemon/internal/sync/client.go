package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

// client es el HTTP client que habla con hive-api.
type client struct {
	cfg        *Config
	httpClient *http.Client
}

func newClient(cfg *Config) *client {
	return &client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// login obtiene un JWT del servidor y devuelve el token + su expiración.
func (c *client) login(ctx context.Context) (token string, expiresAt time.Time, err error) {
	body, _ := json.Marshal(map[string]string{
		"email":    c.cfg.Email,
		"password": c.cfg.Password,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.APIURL+"/auth/login", bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("login request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("login failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("decode login response: %w", err)
	}

	return result.Token, result.ExpiresAt, nil
}

// promptPayload es el formato que espera hive-api para cada prompt de usuario.
type promptPayload struct {
	SyncID    string    `json:"sync_id"`
	Project   string    `json:"project"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// sessionPayload es el formato de sesión en el wire protocol.
// Procesado ANTES de memories[] en push y pull (Decision 11: FK ordering).
type sessionPayload struct {
	ID        string     `json:"id"`
	SyncID    string     `json:"sync_id"`
	Project   string     `json:"project"`
	Directory string     `json:"directory"`
	DevID     string     `json:"dev_id"`
	Client    string     `json:"client"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Summary   *string    `json:"summary,omitempty"`
}

// syncRequest es el payload que enviamos a POST /sync.
// Sessions precede a memories para satisfacer la FK memories.session_id → sessions(id).
type syncRequest struct {
	Project  string           `json:"project"`
	Sessions []sessionPayload `json:"sessions"`
	Memories []memoryPayload  `json:"memories"`
	Prompts  []promptPayload  `json:"prompts,omitempty"`
	LastSync *time.Time       `json:"last_sync,omitempty"`
}

// memoryPayload es el formato que espera hive-api para cada memoria.
type memoryPayload struct {
	SyncID        string   `json:"sync_id"`
	Project       string   `json:"project"`
	TopicKey      *string  `json:"topic_key,omitempty"`
	Category      string   `json:"category"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Tags          []string `json:"tags"`
	FilesAffected []string `json:"files_affected"`
	CreatedBy     string   `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Confidence    float32  `json:"confidence"`
	ImpactScore   float32  `json:"impact_score"`
	// SessionID enables explicit attribution end-to-end. Empty string is dropped
	// by omitempty so legacy daemons stay backward-compatible on the wire.
	SessionID string `json:"session_id,omitempty"`
}

// syncResponse es lo que devuelve hive-api tras el sync.
// PulledSessions se aplica ANTES de Pulled para satisfacer la FK.
type syncResponse struct {
	Pushed         int              `json:"pushed"`
	Pulled         []apiMemory      `json:"pulled"`
	Conflicts      int              `json:"conflicts"`
	PromptsPushed  int              `json:"prompts_pushed"`
	PulledSessions []sessionPayload `json:"pulled_sessions,omitempty"`
}

// apiMemory es la forma que usa hive-api para devolver memorias.
type apiMemory struct {
	ID            string    `json:"id"`
	SyncID        string    `json:"sync_id"`
	Project       string    `json:"project"`
	TopicKey      *string   `json:"topic_key"`
	Category      string    `json:"category"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	Tags          []string  `json:"tags"`
	FilesAffected []string  `json:"files_affected"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Confidence    float32   `json:"confidence"`
	ImpactScore   float32   `json:"impact_score"`
	SessionID     string    `json:"session_id,omitempty"`
}

// sync envía sesiones, memorias y prompts locales, y recibe del servidor para un proyecto.
// sessions se serializa ANTES de memories (Decision 11: FK ordering).
func (c *client) sync(ctx context.Context, token, project string,
	sessions []*models.Session, toSend []*models.Memory, prompts []*models.Prompt, lastSync *time.Time) (*syncResponse, error) {

	sessionPayloads := make([]sessionPayload, 0, len(sessions))
	for _, s := range sessions {
		sessionPayloads = append(sessionPayloads, sessionPayload{
			ID:        s.ID,
			SyncID:    s.SyncID,
			Project:   s.Project,
			Directory: s.Directory,
			DevID:     s.DevID,
			Client:    s.Client,
			StartedAt: s.StartedAt,
			EndedAt:   s.EndedAt,
			Summary:   nilStringPtr(s.Summary),
		})
	}

	payloads := make([]memoryPayload, 0, len(toSend))
	for _, m := range toSend {
		payloads = append(payloads, memoryPayload{
			SyncID:        m.SyncID,
			Project:       m.Project,
			TopicKey:      m.TopicKey,
			Category:      m.Category,
			Title:         m.Title,
			Content:       m.Content,
			Tags:          orEmpty(m.Tags),
			FilesAffected: orEmpty(m.FilesAffected),
			CreatedBy:     m.CreatedBy,
			CreatedAt:     m.CreatedAt,
			UpdatedAt:     m.UpdatedAt,
			SessionID:     m.SessionID,
		})
	}

	promptPayloads := make([]promptPayload, 0, len(prompts))
	for _, p := range prompts {
		promptPayloads = append(promptPayloads, promptPayload{
			SyncID:    p.SyncID,
			Project:   p.Project,
			Content:   p.Content,
			CreatedAt: p.CreatedAt,
		})
	}

	reqBody, err := json.Marshal(syncRequest{
		Project:  project,
		Sessions: sessionPayloads,
		Memories: payloads,
		Prompts:  promptPayloads,
		LastSync: lastSync,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal sync request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.APIURL+"/sync", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("build sync request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sync request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sync failed (%d): %s", resp.StatusCode, string(body))
	}

	var result syncResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode sync response: %w", err)
	}

	return &result, nil
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nilStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
