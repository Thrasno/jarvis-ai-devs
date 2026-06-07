package hiveclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "http://127.0.0.1:7438"
	defaultTimeout = 10 * time.Second
)

var ErrNotAvailable = errors.New("governance endpoint is not available")

type Client struct {
	baseURL *url.URL
	http    *http.Client
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("hive daemon returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("hive daemon returned status %d: %s", e.StatusCode, e.Message)
}

type Project struct {
	Name               string    `json:"name"`
	Directory          string    `json:"directory"`
	ActiveMemoryCount  int       `json:"active_memory_count"`
	DeletedMemoryCount int       `json:"deleted_memory_count"`
	SessionCount       int       `json:"session_count"`
	PromptCount        int       `json:"prompt_count"`
	LastActivityAt     time.Time `json:"last_activity_at"`
}

type Memory struct {
	ID        int64     `json:"id"`
	SyncID    string    `json:"sync_id"`
	Project   string    `json:"project"`
	Category  string    `json:"category"`
	Title     string    `json:"title"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	Deleted   bool      `json:"deleted"`
}

type MemoryFilter struct {
	Project        string
	IncludeDeleted bool
	Limit          int
}

type Health struct {
	Project             string    `json:"project"`
	LastAttemptAt       time.Time `json:"last_attempt_at"`
	LastSuccessAt       time.Time `json:"last_success_at"`
	LastFailureAt       time.Time `json:"last_failure_at"`
	BackoffUntil        time.Time `json:"backoff_until"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	LastError           string    `json:"last_error"`
}

type Warning struct {
	ID              int64      `json:"id"`
	CreatedAt       time.Time  `json:"created_at"`
	Severity        string     `json:"severity"`
	Source          string     `json:"source"`
	Message         string     `json:"message"`
	ResolutionState string     `json:"resolution_state"`
	ResolvedAt      *time.Time `json:"resolved_at"`
}

type Backup struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	DBPath       string    `json:"db_path"`
	ArchivePath  string    `json:"archive_path"`
	ManifestPath string    `json:"manifest_path"`
	Checksum     string    `json:"checksum"`
	SizeBytes    int64     `json:"size_bytes"`
}

type GuardRequest struct {
	Operation    string `json:"operation"`
	TargetType   string `json:"target_type"`
	TargetID     int64  `json:"target_id"`
	BackupID     string `json:"backup_id"`
	Confirmation string `json:"confirmation"`
	ActorID      string `json:"actor_id,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type GuardResult struct {
	Operation  string `json:"operation"`
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id"`
	BackupID   string `json:"backup_id"`
	Mutated    bool   `json:"mutated"`
}

func NewFromEnv() (*Client, error) {
	baseURL := strings.TrimSpace(os.Getenv("HIVE_DAEMON_URL"))
	if baseURL == "" {
		baseURL = defaultBaseURL
		if port := strings.TrimSpace(os.Getenv("HIVE_HTTP_PORT")); port != "" {
			baseURL = "http://127.0.0.1:" + port
		}
	}
	return New(baseURL)
}

func New(baseURL string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid hive daemon url %q", baseURL)
	}
	return &Client{baseURL: parsed, http: &http.Client{Timeout: defaultTimeout}}, nil
}

func (c *Client) Status(ctx context.Context) ([]Health, error) {
	var body struct {
		Projects []Health `json:"projects"`
	}
	if err := c.get(ctx, "/governance/health", nil, &body, false); err != nil {
		return nil, err
	}
	return body.Projects, nil
}

func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	var body struct {
		Projects []Project `json:"projects"`
	}
	if err := c.get(ctx, "/governance/projects", nil, &body, false); err != nil {
		return nil, err
	}
	return body.Projects, nil
}

func (c *Client) Memories(ctx context.Context, filter MemoryFilter) ([]Memory, error) {
	query := url.Values{"project": {filter.Project}}
	if filter.IncludeDeleted {
		query.Set("include_deleted", "true")
	}
	if filter.Limit > 0 {
		query.Set("limit", strconv.Itoa(filter.Limit))
	}
	var body struct {
		Memories []Memory `json:"memories"`
	}
	if err := c.get(ctx, "/governance/memories", query, &body, false); err != nil {
		return nil, err
	}
	return body.Memories, nil
}

func (c *Client) Warnings(ctx context.Context) ([]Warning, error) {
	var body struct {
		Warnings []Warning `json:"warnings"`
	}
	if err := c.get(ctx, "/governance/warnings", nil, &body, true); err != nil {
		return nil, err
	}
	return body.Warnings, nil
}

func (c *Client) Backups(ctx context.Context) ([]Backup, error) {
	var body struct {
		Backups []Backup `json:"backups"`
	}
	if err := c.get(ctx, "/governance/backups", nil, &body, false); err != nil {
		return nil, err
	}
	return body.Backups, nil
}

func (c *Client) ExecuteGuard(ctx context.Context, guard GuardRequest) (GuardResult, error) {
	var body struct {
		Result GuardResult `json:"result"`
	}
	if err := c.post(ctx, "/governance/guards/execute", guard, &body); err != nil {
		return GuardResult{}, err
	}
	return body.Result, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any, notAvailableOn404 bool) error {
	u := *c.baseURL
	u.Path = path
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound && notAvailableOn404 {
		return ErrNotAvailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var body struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return &APIError{StatusCode: resp.StatusCode, Message: body.Error}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(ctx context.Context, path string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	u := *c.baseURL
	u.Path = path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var body struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return &APIError{StatusCode: resp.StatusCode, Message: body.Error}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
