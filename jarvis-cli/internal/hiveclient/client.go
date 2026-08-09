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

	// MaskedSecret is the sentinel value that clients send when they want to
	// preserve the currently stored secret without transmitting or revealing it.
	// The daemon detects this sentinel and reuses the stored credential.
	MaskedSecret = "********"
)

var ErrNotAvailable = errors.New("governance endpoint is not available")
var ErrDeletedMemoryNotFound = errors.New("deleted memory not found")

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
	TopicKey     *string    `json:"topic_key"`
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
	ID             int64
	IncludeDeleted bool
	DeletedOnly    bool
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

	// RetainUntil is when the daemon reclaims this archive. A migration backup
	// is kept 24h and is the ONLY rollback artifact for that migration, so an
	// operator who cannot see the deadline discovers it by finding the backup
	// gone. The zero value means no deadline (an operator-created backup).
	RetainUntil time.Time `json:"retain_until,omitempty"`
}

// RestoreResult is what the daemon actually did with a restore request.
//
// The two outcomes are genuinely different actions, not two descriptions of one:
// with the migration gate BLOCKED the daemon schedules the restore and stops
// itself, while with the gate ready it only validates the archive and answers
// coordination_required — nothing is scheduled and nothing is replaced.
type RestoreResult struct {
	BackupID              string    `json:"backup_id"`
	DBPath                string    `json:"db_path"`
	RestoredAt            time.Time `json:"restored_at,omitempty"`
	ArchivePath           string    `json:"archive_path"`
	Status                string    `json:"status,omitempty"`
	RequiresDaemonRestart bool      `json:"requires_daemon_restart,omitempty"`
	Message               string    `json:"message,omitempty"`
}

// RestoreStatusRestartRequested is the one status that means the daemon took
// ownership of the restart. Every other status leaves the operator holding it.
const RestoreStatusRestartRequested = "restart-requested"

// MigrationIdentityStatus is the recovery state exposed while normal Hive
// operations are blocked by canonical project identity migration.
// BackupID is present only when the blocked migration itself created that
// backup. PlanFingerprint identifies the exact plan the operator was shown and
// is the guard an identity resolution must echo back.
type MigrationIdentityStatus struct {
	State           string   `json:"state"`
	Reason          string   `json:"reason,omitempty"`
	Continuation    string   `json:"continuation,omitempty"`
	BackupID        string   `json:"backup_id,omitempty"`
	PlanFingerprint string   `json:"plan_fingerprint,omitempty"`
	Conflicts       []string `json:"conflicts,omitempty"`
	Variants        []string `json:"variants,omitempty"`
}

type IdentityResolutionRequest struct {
	SourceProject   string `json:"source_project"`
	TargetProject   string `json:"target_project"`
	BackupID        string `json:"backup_id,omitempty"`
	PlanFingerprint string `json:"plan_fingerprint"`
	Confirmation    string `json:"confirmation"`
}

// Capabilities is the daemon's advertised guarded-mutation safety contract.
// The UI must require all fields before enabling delete or restore.
type Capabilities struct {
	DeleteRestore    bool `json:"delete_restore"`
	ExpectedIdentity bool `json:"expected_identity"`
	RequestReceipts  bool `json:"request_receipts"`
	MutationSyncV2   bool `json:"mutation_sync_v2"`
}

func (c Capabilities) SupportsGuardedDeleteRestore() bool {
	return c.DeleteRestore && c.ExpectedIdentity && c.RequestReceipts && c.MutationSyncV2
}

type GuardRequest struct {
	Operation       string `json:"operation"`
	TargetType      string `json:"target_type"`
	TargetID        int64  `json:"target_id"`
	BackupID        string `json:"backup_id"`
	Confirmation    string `json:"confirmation"`
	ActorID         string `json:"actor_id,omitempty"`
	Reason          string `json:"reason,omitempty"`
	ExpectedProject string `json:"expected_project,omitempty"`
	ExpectedSyncID  string `json:"expected_sync_id,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
}

type GuardResult struct {
	Operation  string           `json:"operation"`
	TargetType string           `json:"target_type"`
	TargetID   int64            `json:"target_id"`
	BackupID   string           `json:"backup_id"`
	Mutated    bool             `json:"mutated"`
	Receipt    *MutationReceipt `json:"receipt,omitempty"`
}

// MutationReceipt is the durable outcome for one guarded local mutation.
type MutationReceipt struct {
	RequestID    string `json:"request_id"`
	Operation    string `json:"operation"`
	TargetID     int64  `json:"target_id"`
	Project      string `json:"project"`
	EntitySyncID string `json:"entity_sync_id"`
	EventID      string `json:"event_id"`
	LocalStatus  string `json:"local_status"`
	SharedStatus string `json:"shared_status"`
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

type EngramImportJobKind string

const (
	EngramImportJobKindPreview EngramImportJobKind = "preview"
	EngramImportJobKindExecute EngramImportJobKind = "execute"
)

type EngramImportPhase string

const (
	EngramImportPhaseQueued       EngramImportPhase = "queued"
	EngramImportPhaseDiscovery    EngramImportPhase = "discovery"
	EngramImportPhaseAnalysis     EngramImportPhase = "analysis"
	EngramImportPhaseBackup       EngramImportPhase = "backup"
	EngramImportPhaseImport       EngramImportPhase = "import"
	EngramImportPhaseFinalization EngramImportPhase = "finalization"
	EngramImportPhaseCompleted    EngramImportPhase = "completed"
	EngramImportPhaseFailed       EngramImportPhase = "failed"
)

type EngramImportRequest struct {
	Source    string `json:"source,omitempty"`
	PreviewID string `json:"preview_id,omitempty"`
}

type EngramImportEntityCounts struct {
	Sessions     int `json:"sessions"`
	Prompts      int `json:"prompts"`
	Observations int `json:"observations"`
}

type EngramImportCounts struct {
	Imported  int `json:"imported"`
	Reused    int `json:"reused"`
	Ambiguous int `json:"ambiguous"`
}

type EngramImportProjectImpact struct {
	Project   string                   `json:"project"`
	Projected EngramImportEntityCounts `json:"projected"`
}

type EngramImportInvalidRow struct {
	Table    string `json:"table"`
	SourceID string `json:"source_id"`
	Reason   string `json:"reason"`
}

type EngramImportAmbiguousDuplicate struct {
	SourceID string `json:"source_id"`
	Project  string `json:"project"`
	Title    string `json:"title"`
	Reason   string `json:"reason"`
}

type EngramImportReport struct {
	PreviewID           string                           `json:"preview_id,omitempty"`
	SourcePath          string                           `json:"source_path"`
	SourceFingerprint   string                           `json:"source_fingerprint"`
	Projects            []string                         `json:"projects,omitempty"`
	Projected           EngramImportEntityCounts         `json:"projected"`
	ProjectedByProject  []EngramImportProjectImpact      `json:"projected_by_project,omitempty"`
	Imported            EngramImportCounts               `json:"imported"`
	AmbiguousDuplicates []EngramImportAmbiguousDuplicate `json:"ambiguous_duplicates,omitempty"`
	SkippedRelations    int                              `json:"skipped_relations"`
	InvalidRows         []EngramImportInvalidRow         `json:"invalid_rows,omitempty"`
	BackupID            string                           `json:"backup_id,omitempty"`
}

type EngramImportJob struct {
	ID           string              `json:"id"`
	Kind         EngramImportJobKind `json:"kind"`
	Phase        EngramImportPhase   `json:"phase"`
	Message      string              `json:"message"`
	Processed    int                 `json:"processed"`
	Total        int                 `json:"total"`
	Percent      int                 `json:"percent"`
	Done         bool                `json:"done"`
	Error        string              `json:"error,omitempty"`
	Report       *EngramImportReport `json:"report,omitempty"`
	PhaseHistory []string            `json:"phase_history,omitempty"`
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
	Configured      bool     `json:"configured"`
	Source          string   `json:"source"`
	APIURL          string   `json:"api_url"`
	Email           string   `json:"email"`
	PasswordSet     bool     `json:"password_set"`
	PasswordMasked  string   `json:"password_masked"`
	AutoSync        bool     `json:"auto_sync"`
	EnvActive       bool     `json:"env_active"`
	RestartHint     string   `json:"restart_hint,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	RestartRequired bool     `json:"restart_required"`
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

// SyncSummary is the client-side view of the daemon's aggregate sync health.
// It mirrors the HealthSummaryResponse DTO from the daemon's
// GET /governance/health/summary endpoint.
// All time fields use the zero value when the daemon returns null or a zero timestamp.
type SyncSummary struct {
	Reachable           bool      `json:"reachable"`
	AuthOK              bool      `json:"auth_ok"`
	AutoSync            bool      `json:"auto_sync"`
	LastSuccessAt       time.Time `json:"last_success_at"`
	LastFailureAt       time.Time `json:"last_failure_at"`
	LastError           string    `json:"last_error"`
	UnsyncedMemories    int       `json:"unsynced_memories"`
	UnsyncedPrompts     int       `json:"unsynced_prompts"`
	UnsyncedSessions    int       `json:"unsynced_sessions"`
	BackoffUntil        time.Time `json:"backoff_until"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
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
	if filter.ID > 0 {
		query.Set("id", strconv.FormatInt(filter.ID, 10))
	}
	if filter.IncludeDeleted {
		query.Set("include_deleted", "true")
	}
	if filter.DeletedOnly {
		query.Set("deleted_only", "true")
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

func (c *Client) DeletedMemoryByID(ctx context.Context, id int64, project string) (Memory, error) {
	memories, err := c.Memories(ctx, MemoryFilter{Project: project, ID: id, DeletedOnly: true})
	if err != nil {
		return Memory{}, err
	}
	for _, memory := range memories {
		if memory.ID == id {
			return memory, nil
		}
	}
	return Memory{}, fmt.Errorf("%w: id=%d", ErrDeletedMemoryNotFound, id)
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

func (c *Client) MigrationIdentityStatus(ctx context.Context) (MigrationIdentityStatus, error) {
	var status MigrationIdentityStatus
	if err := c.get(ctx, "/governance/project-identity/status", nil, &status, false); err != nil {
		return MigrationIdentityStatus{}, err
	}
	return status, nil
}

// RequestMigrationRetry asks the running daemon to shut down cleanly. Its MCP
// lifecycle owner starts the single replacement process, which replans the
// complete migration during normal startup.
func (c *Client) RequestMigrationRetry(ctx context.Context) error {
	return c.post(ctx, "/governance/project-identity/retry", struct{}{}, &struct{}{})
}

func (c *Client) ResolveMigrationIdentity(ctx context.Context, req IdentityResolutionRequest) error {
	return c.post(ctx, "/governance/project-identity/resolve", req, &struct{}{})
}

func (c *Client) RestoreMigrationBackup(ctx context.Context, backupID, confirmation string) (RestoreResult, error) {
	var body struct {
		Restore RestoreResult `json:"restore"`
	}
	if err := c.post(ctx, "/governance/restores", map[string]string{"backup_id": backupID, "confirmation": confirmation}, &body); err != nil {
		return RestoreResult{}, err
	}
	return body.Restore, nil
}

func (c *Client) CreateBackup(ctx context.Context) (Backup, error) {
	var body struct {
		Backup Backup `json:"backup"`
	}
	if err := c.post(ctx, "/governance/backups", struct{}{}, &body); err != nil {
		return Backup{}, err
	}
	return body.Backup, nil
}

func (c *Client) Capabilities(ctx context.Context) (Capabilities, error) {
	var body struct {
		Capabilities Capabilities `json:"capabilities"`
	}
	if err := c.get(ctx, "/governance/capabilities", nil, &body, true); err != nil {
		return Capabilities{}, err
	}
	return body.Capabilities, nil
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

func (c *Client) MutationReceipt(ctx context.Context, requestID string, targetID int64, project, syncID string) (MutationReceipt, error) {
	query := url.Values{"target_id": {strconv.FormatInt(targetID, 10)}, "project": {project}, "sync_id": {syncID}}
	var body struct {
		Receipt MutationReceipt `json:"receipt"`
	}
	if err := c.get(ctx, "/governance/mutations/"+url.PathEscape(strings.TrimSpace(requestID)), query, &body, false); err != nil {
		return MutationReceipt{}, err
	}
	return body.Receipt, nil
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

// TimelineResult is the structured result of Timeline.
// Truncated is true when the daemon hit the 500-entry hard limit.
type TimelineResult struct {
	Memories  []Memory
	Truncated bool
}

// Timeline fetches the category-filtered, ASC-ordered timeline for a project
// via GET /governance/projects/{name}/timeline.
// It returns an APIError (with StatusCode 404) when the project does not exist.
// TimelineResult.Truncated is true when the response signals the 500-entry limit was hit.
func (c *Client) Timeline(ctx context.Context, project string) (TimelineResult, error) {
	var body struct {
		Memories  []Memory `json:"memories"`
		Truncated bool     `json:"truncated"`
	}
	path := "/governance/projects/" + url.PathEscape(project) + "/timeline"
	if err := c.get(ctx, path, nil, &body, false); err != nil {
		return TimelineResult{}, err
	}
	return TimelineResult{Memories: body.Memories, Truncated: body.Truncated}, nil
}

// GetSyncSummary fetches the aggregate sync health summary from
// GET /governance/health/summary. A Go error is returned on transport failures
// and non-2xx daemon responses. On 404 (old daemon without T14), the error is
// ErrNotAvailable — callers should treat this as a nil-summary situation rather
// than a fatal error and use errors.Is(err, ErrNotAvailable) to detect it.
func (c *Client) GetSyncSummary(ctx context.Context) (SyncSummary, error) {
	var summary SyncSummary
	if err := c.get(ctx, "/governance/health/summary", nil, &summary, true); err != nil {
		return SyncSummary{}, err
	}
	return summary, nil
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

func (c *Client) StartEngramImportPreview(ctx context.Context, req EngramImportRequest) (EngramImportJob, error) {
	var body struct {
		Job EngramImportJob `json:"job"`
	}
	if err := c.post(ctx, "/governance/imports/engram/preview", req, &body); err != nil {
		return EngramImportJob{}, err
	}
	return body.Job, nil
}

func (c *Client) StartEngramImportExecute(ctx context.Context, req EngramImportRequest) (EngramImportJob, error) {
	var body struct {
		Job EngramImportJob `json:"job"`
	}
	if err := c.post(ctx, "/governance/imports/engram/execute", req, &body); err != nil {
		return EngramImportJob{}, err
	}
	return body.Job, nil
}

func (c *Client) GetEngramImportJob(ctx context.Context, id string) (EngramImportJob, error) {
	var body struct {
		Job EngramImportJob `json:"job"`
	}
	path := "/governance/imports/engram/jobs/" + url.PathEscape(strings.TrimSpace(id))
	if err := c.get(ctx, path, nil, &body, false); err != nil {
		return EngramImportJob{}, err
	}
	return body.Job, nil
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
