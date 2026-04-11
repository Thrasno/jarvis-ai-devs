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

// syncRequest es el payload que enviamos a POST /sync.
type syncRequest struct {
	Project  string          `json:"project"`
	Memories []memoryPayload `json:"memories"`
	LastSync *time.Time      `json:"last_sync,omitempty"`
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
}

// syncResponse es lo que devuelve hive-api tras el sync.
type syncResponse struct {
	Pushed    int           `json:"pushed"`
	Pulled    []apiMemory   `json:"pulled"`
	Conflicts int           `json:"conflicts"`
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
}

// sync envía memorias locales y recibe las del servidor para un proyecto.
func (c *client) sync(ctx context.Context, token, project string,
	toSend []*models.Memory, lastSync *time.Time) (*syncResponse, error) {

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
		})
	}

	reqBody, err := json.Marshal(syncRequest{
		Project:  project,
		Memories: payloads,
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
