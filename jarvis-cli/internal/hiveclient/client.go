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
	UnsyncedCount      int       `json:"unsynced_count"`
}

type Memory struct {
	ID           int64      `json:"id"`
	SyncID       string     `json:"sync_id"`
	Project      string     `json:"project"`
	Category     string     `json:"category"`
	Title        string     `json:"title"`
	Content      string     `json:"content"`
	CreatedBy    string     `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	Deleted      bool       `json:"deleted"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
	DeletedBy    string     `json:"deleted_by,omitempty"`
	DeleteReason string     `json:"delete_reason,omitempty"`
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

type ProjectArchiveRequest struct {
	Project      string `json:"project"`
	BackupID     string `json:"backup_id"`
	Confirmation string `json:"confirmation"`
	ActorID      string `json:"actor_id,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type ProjectArchiveResult struct {
	Operation        string `json:"operation"`
	TargetType       string `json:"target_type"`
	Project          string `json:"project"`
	BackupID         string `json:"backup_id"`
	Mutated          bool   `json:"mutated"`
	CloudHandoffNote string `json:"cloud_handoff_note"`
}

type ProjectMergeRequest struct {
	SourceProject string `json:"source_project"`
	TargetProject string `json:"target_project"`
	BackupID      string `json:"backup_id"`
	Confirmation  string `json:"confirmation"`
	ActorID       string `json:"actor_id,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type ProjectMergeResult struct {
	Operation        string `json:"operation"`
	TargetType       string `json:"target_type"`
	SourceProject    string `json:"source_project"`
	TargetProject    string `json:"target_project"`
	BackupID         string `json:"backup_id"`
	Mutated          bool   `json:"mutated"`
	CloudHandoffNote string `json:"cloud_handoff_note"`
}

// ProjectDeleteRequest is the input for DeleteProject.
type ProjectDeleteRequest struct {
	Project      string `json:"project"`
	BackupID     string `json:"backup_id"`
	Confirmation string `json:"confirmation"`
	ActorID      string `json:"actor_id,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// ProjectDeleteResult is the output of DeleteProject.
type ProjectDeleteResult struct {
	Operation        string `json:"operation"`
	TargetType       string `json:"target_type"`
	Project          string `json:"project"`
	BackupID         string `json:"backup_id"`
	RowsDeleted      int    `json:"rows_deleted"`
	Mutated          bool   `json:"mutated"`
	CloudHandoffNote string `json:"cloud_handoff_note"`
}

// ProjectMergeBatchRequest is the input for a multi-source batch merge.
type ProjectMergeBatchRequest struct {
	Sources      []string `json:"sources"`
	Target       string   `json:"target"`
	BackupID     string   `json:"backup_id"`
	Confirmation string   `json:"confirmation"`
	ActorID      string   `json:"actor_id,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

// MergeResult holds the outcome of a single source→target merge within a batch.
type MergeResult struct {
	Source        string `json:"source"`
	Target        string `json:"target"`
	AlreadyMerged bool   `json:"already_merged"`
	Mutated       bool   `json:"mutated"`
	ErrMsg        string `json:"error,omitempty"`
}

// ProjectMergeBatchResult is the output of a multi-source batch merge.
type ProjectMergeBatchResult struct {
	Operation        string        `json:"operation"`
	Target           string        `json:"target"`
	BackupID         string        `json:"backup_id"`
	Results          []MergeResult `json:"results"`
	HasSyncEvidence  bool          `json:"has_sync_evidence"`
	CloudHandoffNote string        `json:"cloud_handoff_note,omitempty"`
}

// ConfigStatus is the client-side view of the daemon sync config.
// The raw password is never present; PasswordMasked is "********" when set.
type ConfigStatus struct {
	Configured     bool     `json:"configured"`
	Source         string   `json:"source"`
	APIURL         string   `json:"api_url"`
	Email          string   `json:"email"`
	PasswordSet    bool     `json:"password_set"`
	PasswordMasked string   `json:"password_masked"`
	AutoSync       bool     `json:"auto_sync"`
	EnvActive      bool     `json:"env_active"`
	RestartHint    string   `json:"restart_hint,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

// ConfigUpdateRequest is the payload for POST /governance/config.
// Password may be "********" (the masked sentinel) to preserve the stored secret.
type ConfigUpdateRequest struct {
	APIURL   string `json:"api_url"`
	Email    string `json:"email"`
	Password string `json:"password"`
	AutoSync bool   `json:"auto_sync"`
}

// configUpdateWire mirrors the flat JSON the daemon emits for POST /governance/config.
// Keep in sync with hive-daemon/internal/httpapi/config.go ConfigStatusResponse.
// The daemon embeds ConfigStatusResponse directly and adds restart_required.
type configUpdateWire struct {
	Configured     bool     `json:"configured"`
	Source         string   `json:"source"`
	APIURL         string   `json:"api_url"`
	Email          string   `json:"email"`
	PasswordSet    bool     `json:"password_set"`
	PasswordMasked string   `json:"password_masked"`
	AutoSync       bool     `json:"auto_sync"`
	EnvActive      bool     `json:"env_active"`
	RestartHint    string   `json:"restart_hint,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
	RestartRequired bool    `json:"restart_required"`
}

// ConfigUpdateResponse is the structured result of UpdateConfig.
// RestartHint and EnvActive are promoted to the top level for convenient access.
// Status holds the full refreshed config state.
type ConfigUpdateResponse struct {
	RestartRequired bool
	RestartHint     string
	EnvActive       bool
	Status          ConfigStatus
}

// ConfigTestRequest is the payload for POST /governance/config/test.
type ConfigTestRequest struct {
	APIURL   string `json:"api_url"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ConfigTestResult is the outcome of TestConnection.
// OK is true when login succeeded. A Go error is only returned for transport failures.
type ConfigTestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
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

func (c *Client) MemoryByID(ctx context.Context, id int64) (Memory, error) {
	var body struct {
		Memory Memory `json:"memory"`
	}
	path := "/governance/memories/" + strconv.FormatInt(id, 10)
	if err := c.get(ctx, path, nil, &body, false); err != nil {
		return Memory{}, err
	}
	return body.Memory, nil
}

func (c *Client) Warnings(ctx context.Context) ([]Warning, error) {
	var body struct {
		Warnings []Warning `json:"warnings"`
	}
	if err := c.get(ctx, "/governance/warnings", nil, &body, false); err != nil {
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

func (c *Client) ArchiveProject(ctx context.Context, req ProjectArchiveRequest) (ProjectArchiveResult, error) {
	project := strings.TrimSpace(req.Project)
	req.Project = project
	var body struct {
		Result ProjectArchiveResult `json:"result"`
	}
	if err := c.post(ctx, "/governance/projects/"+url.PathEscape(project)+"/archive", req, &body); err != nil {
		return ProjectArchiveResult{}, err
	}
	return body.Result, nil
}

// DeleteProject sends a purge request for an archived project to
// POST /governance/projects/{name}/delete.
func (c *Client) DeleteProject(ctx context.Context, req ProjectDeleteRequest) (ProjectDeleteResult, error) {
	project := strings.TrimSpace(req.Project)
	req.Project = project
	var body struct {
		Result ProjectDeleteResult `json:"result"`
	}
	if err := c.post(ctx, "/governance/projects/"+url.PathEscape(project)+"/delete", req, &body); err != nil {
		return ProjectDeleteResult{}, err
	}
	return body.Result, nil
}

func (c *Client) MergeProject(ctx context.Context, req ProjectMergeRequest) (ProjectMergeResult, error) {
	source := strings.TrimSpace(req.SourceProject)
	target := strings.TrimSpace(req.TargetProject)
	req.SourceProject = source
	req.TargetProject = target
	var body struct {
		Result ProjectMergeResult `json:"result"`
	}
	path := "/governance/projects/" + url.PathEscape(source) + "/merge/" + url.PathEscape(target)
	if err := c.post(ctx, path, req, &body); err != nil {
		return ProjectMergeResult{}, err
	}
	return body.Result, nil
}

// GetConfigStatus fetches the current sync config state from GET /governance/config/status.
func (c *Client) GetConfigStatus(ctx context.Context) (ConfigStatus, error) {
	var status ConfigStatus
	if err := c.get(ctx, "/governance/config/status", nil, &status, false); err != nil {
		return ConfigStatus{}, err
	}
	return status, nil
}

// UpdateConfig saves new sync config via POST /governance/config and returns the
// refreshed status together with restart metadata.
func (c *Client) UpdateConfig(ctx context.Context, req ConfigUpdateRequest) (ConfigUpdateResponse, error) {
	var wire configUpdateWire
	if err := c.post(ctx, "/governance/config", req, &wire); err != nil {
		return ConfigUpdateResponse{}, err
	}
	return ConfigUpdateResponse{
		RestartRequired: wire.RestartRequired,
		RestartHint:     wire.RestartHint,
		EnvActive:       wire.EnvActive,
		Status: ConfigStatus{
			Configured:     wire.Configured,
			Source:         wire.Source,
			APIURL:         wire.APIURL,
			Email:          wire.Email,
			PasswordSet:    wire.PasswordSet,
			PasswordMasked: wire.PasswordMasked,
			AutoSync:       wire.AutoSync,
			EnvActive:      wire.EnvActive,
			RestartHint:    wire.RestartHint,
			Warnings:       wire.Warnings,
		},
	}, nil
}

// TestConnection calls POST /governance/config/test. A Go error is returned for
// transport failures AND non-2xx daemon responses. When the daemon responds 200
// with ok:false, the error is nil and the result carries the failure in OK and Message.
func (c *Client) TestConnection(ctx context.Context, req ConfigTestRequest) (ConfigTestResult, error) {
	var result ConfigTestResult
	if err := c.post(ctx, "/governance/config/test", req, &result); err != nil {
		return ConfigTestResult{}, err
	}
	return result, nil
}

// MergeProjects sends a multi-source batch merge request to POST /governance/projects/merge.
func (c *Client) MergeProjects(ctx context.Context, req ProjectMergeBatchRequest) (ProjectMergeBatchResult, error) {
	var body struct {
		Result ProjectMergeBatchResult `json:"result"`
	}
	if err := c.post(ctx, "/governance/projects/merge", req, &body); err != nil {
		return ProjectMergeBatchResult{}, err
	}
	return body.Result, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, out any, notAvailableOn404 bool) error {
	u := *c.baseURL
	setURLPath(&u, path)
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
	setURLPath(&u, path)
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

func setURLPath(u *url.URL, path string) {
	decodedPath, err := url.PathUnescape(path)
	if err != nil || decodedPath == path {
		u.Path = path
		return
	}
	u.Path = decodedPath
	u.RawPath = path
}
