package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

const mutationProtocolVersion = 2

var ErrProjectBlocked = errors.New("project is blocked")

// HTTPStatusError safely preserves an HTTP outcome without retaining a response body.
type HTTPStatusError struct {
	Operation  string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("%s failed (%d)", e.Operation, e.StatusCode)
}

// PullCursor is a sync-package alias for db.PullCursor (defined in
// internal/db alongside db.MutationCursor, PR 2b hive-sync-batched-drain).
// It cannot be declared here instead: internal/db needs the same type for
// GetPullCursor/SetPullCursor persistence, and internal/sync already
// imports internal/db, so internal/db cannot import back from
// internal/sync. Aliasing keeps call sites in this package (client.go,
// syncer.go, tests) short while there is exactly one underlying type.
type PullCursor = db.PullCursor

// pullOptions carries the daemon's opt-in bounded-pull request (PR 2b): a
// zero-value pullOptions{} sends no pull_limit/cursor fields at all
// (omitempty), which matches hive-api's own opt-in contract — an absent
// pull_limit means an unbounded legacy pull, exactly like pre-PR-2a
// behavior. Callers that want bounded, resumable pagination set Limit and
// forward the cursors persisted from the previous response's
// NextPullCursor/NextSessionCursor.
type pullOptions struct {
	Limit          int
	MemoriesCursor *PullCursor
	SessionsCursor *PullCursor
}

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

// Login is the exported thin wrapper over login that the ConfigService uses
// for connection tests. It discards the token; callers care only about success
// or failure (i.e. can the daemon reach the API with these credentials?).
func (c *client) Login(ctx context.Context) error {
	_, _, err := c.login(ctx)
	return err
}

// login obtiene un JWT del servidor y devuelve el token + su expiración.
func (c *client) login(ctx context.Context) (token string, expiresAt time.Time, err error) {
	body, _ := json.Marshal(map[string]string{
		"email":     c.cfg.Email,
		"password":  c.cfg.Password,
		"daemon_id": c.cfg.DaemonID,
		"client":    "hive-daemon",
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
		return "", time.Time{}, &HTTPStatusError{Operation: "login", StatusCode: resp.StatusCode}
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
	Project         string                `json:"project"`
	Sessions        []sessionPayload      `json:"sessions"`
	Memories        []memoryPayload       `json:"memories"`
	Prompts         []promptPayload       `json:"prompts,omitempty"`
	LastSync        *time.Time            `json:"last_sync,omitempty"`
	ProtocolVersion int                   `json:"protocol_version,omitempty"`
	MutationCursor  *db.MutationCursor    `json:"mutation_cursor,omitempty"`
	Mutations       []db.MutationEnvelope `json:"mutations,omitempty"`

	// Bounded legacy pull pagination (PR 2a/2b, hive-sync-batched-drain).
	// PullLimit is an explicit opt-in: omitted/0 means an unbounded legacy
	// pull, matching pre-PR-2a behavior exactly (see pullOptions doc).
	// PullCursor resumes the memories pull channel; PullSessionCursor
	// resumes the sessions pull channel — the two paginate independently.
	PullLimit         int         `json:"pull_limit,omitempty"`
	PullCursor        *PullCursor `json:"pull_cursor,omitempty"`
	PullSessionCursor *PullCursor `json:"pull_session_cursor,omitempty"`
}

// memoryPayload es el formato que espera hive-api para cada memoria.
type memoryPayload struct {
	SyncID        string    `json:"sync_id"`
	Project       string    `json:"project"`
	TopicKey      *string   `json:"topic_key,omitempty"`
	Category      string    `json:"category"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	Tags          []string  `json:"tags"`
	FilesAffected []string  `json:"files_affected"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	// SessionID enables explicit attribution end-to-end. Empty string is dropped
	// by omitempty so legacy daemons stay backward-compatible on the wire.
	SessionID string `json:"session_id,omitempty"`
}

// syncResponse es lo que devuelve hive-api tras el sync.
// PulledSessions se aplica ANTES de Pulled para satisfacer la FK.
type syncResponse struct {
	Pushed             int                   `json:"pushed"`
	Pulled             []apiMemory           `json:"pulled"`
	Conflicts          int                   `json:"conflicts"`
	PromptsPushed      int                   `json:"prompts_pushed"`
	PulledSessions     []sessionPayload      `json:"pulled_sessions,omitempty"`
	NextMutationCursor *db.MutationCursor    `json:"next_mutation_cursor,omitempty"`
	PulledMutations    []db.MutationEnvelope `json:"pulled_mutations,omitempty"`
	MutationResults    []mutationResult      `json:"mutation_results,omitempty"`
	CompatibilityMode  string                `json:"compatibility_mode,omitempty"`

	// Bounded legacy pull pagination (PR 2a/2b, hive-sync-batched-drain).
	// omitempty on all four fields preserves backward compat both ways: an
	// OLD hive-api server never sends them, so they decode to their zero
	// values (false / nil) — PulledHasMore=false and
	// PulledSessionsHasMore=false correctly signal "no more pages" for a
	// server that predates pagination entirely.
	PulledHasMore         bool        `json:"pulled_has_more,omitempty"`
	NextPullCursor        *PullCursor `json:"next_pull_cursor,omitempty"`
	PulledSessionsHasMore bool        `json:"pulled_sessions_has_more,omitempty"`
	NextSessionCursor     *PullCursor `json:"next_session_cursor,omitempty"`
}

type mutationResult struct {
	EventID   string `json:"event_id"`
	Applied   bool   `json:"applied"`
	Duplicate bool   `json:"duplicate"`
	Rejected  bool   `json:"rejected"`
}

type projectBlockCommand struct {
	CommandID           string    `json:"command_id"`
	AckToken            string    `json:"ack_token"`
	Project             string    `json:"project"`
	CanonicalProjectKey string    `json:"canonical_project_key"`
	Reason              string    `json:"reason"`
	BlockedAt           time.Time `json:"blocked_at"`
}

type projectBlockedErrorResponse struct {
	Error   string              `json:"error"`
	Command projectBlockCommand `json:"command"`
}

type ProjectBlockedError struct {
	Command projectBlockCommand
}

func (e *ProjectBlockedError) Error() string {
	return ErrProjectBlocked.Error()
}

func (e *ProjectBlockedError) Unwrap() error {
	return ErrProjectBlocked
}

type syncAttemptIngestRequest struct {
	Attempts []syncAttemptPayload `json:"attempts"`
}

type syncAttemptPayload struct {
	AttemptID    string            `json:"attempt_id"`
	DevID        string            `json:"dev_id"`
	Project      string            `json:"project"`
	Client       string            `json:"client"`
	DaemonID     string            `json:"daemon_id"`
	StartedAt    time.Time         `json:"started_at"`
	EndedAt      *time.Time        `json:"ended_at,omitempty"`
	Outcome      string            `json:"outcome"`
	HTTPStatus   *int              `json:"http_status,omitempty"`
	ErrorCode    *string           `json:"error_code,omitempty"`
	ErrorMessage *string           `json:"error_message,omitempty"`
	RequestID    string            `json:"request_id"`
	SyncCounts   map[string]int    `json:"sync_counts"`
	Metadata     map[string]string `json:"metadata"`
}

type syncAttemptRejected struct {
	AttemptID string `json:"attempt_id"`
	Error     string `json:"error"`
}

type syncAttemptIngestResponse struct {
	AcceptedIDs  []string              `json:"accepted_ids"`
	DuplicateIDs []string              `json:"duplicate_ids"`
	Rejected     []syncAttemptRejected `json:"rejected"`
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
	SessionID     string    `json:"session_id,omitempty"`
}

// sync envía sesiones, memorias y prompts locales, y recibe del servidor para un proyecto.
// sessions se serializa ANTES de memories (Decision 11: FK ordering).
// pullOpts opts into bounded legacy pull pagination (PR 2a/2b) — its zero
// value sends no pull_limit/cursor fields, preserving the pre-PR-2a
// unbounded-pull request shape exactly.
func (c *client) sync(ctx context.Context, token, project string,
	sessions []*models.Session, toSend []*models.Memory, prompts []*models.Prompt, lastSync *time.Time,
	mutations []db.MutationEnvelope, mutationCursor *db.MutationCursor, pullOpts pullOptions) (*syncResponse, error) {

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
		Project:           project,
		Sessions:          sessionPayloads,
		Memories:          payloads,
		Prompts:           promptPayloads,
		LastSync:          lastSync,
		ProtocolVersion:   mutationProtocolVersion,
		MutationCursor:    mutationCursor,
		Mutations:         mutations,
		PullLimit:         pullOpts.Limit,
		PullCursor:        pullOpts.MemoriesCursor,
		PullSessionCursor: pullOpts.SessionsCursor,
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

	if resp.StatusCode == http.StatusLocked {
		var blocked projectBlockedErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&blocked); err != nil {
			return nil, fmt.Errorf("decode blocked project response: %w", err)
		}
		return nil, &ProjectBlockedError{Command: blocked.Command}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{Operation: "sync", StatusCode: resp.StatusCode}
	}

	var result syncResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode sync response: %w", err)
	}

	return &result, nil
}

func (c *client) ackProjectBlock(ctx context.Context, token string, ack db.ProjectBlockAck) error {
	reqBody, err := json.Marshal(ack)
	if err != nil {
		return fmt.Errorf("marshal project block ack: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.APIURL+"/admin/project-blocks/ack", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("build project block ack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("project block ack request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &HTTPStatusError{Operation: "project block ack", StatusCode: resp.StatusCode}
	}
	return nil
}

func (c *client) syncAttempts(ctx context.Context, token string, attempts []db.SyncAttemptLog) (*syncAttemptIngestResponse, error) {
	payloads := make([]syncAttemptPayload, 0, len(attempts))
	for _, attempt := range attempts {
		payloads = append(payloads, syncAttemptPayload{
			AttemptID:    attempt.AttemptID,
			DevID:        attempt.DevID,
			Project:      attempt.Project,
			Client:       attempt.Client,
			DaemonID:     attempt.DaemonID,
			StartedAt:    attempt.StartedAt,
			EndedAt:      timePtr(attempt.EndedAt),
			Outcome:      string(attempt.Outcome),
			HTTPStatus:   intPtrIfNonZero(attempt.HTTPStatus),
			ErrorCode:    stringPtrIfNotEmpty(attempt.ErrorCode),
			ErrorMessage: stringPtrIfNotEmpty(attempt.ErrorMessage),
			RequestID:    attempt.RequestID,
			SyncCounts:   intMapFromJSON(attempt.SyncCountsJSON),
			Metadata:     stringMapFromJSON(attempt.MetadataJSON),
		})
	}

	reqBody, err := json.Marshal(syncAttemptIngestRequest{Attempts: payloads})
	if err != nil {
		return nil, fmt.Errorf("marshal sync attempts request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.APIURL+"/sync-attempts", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("build sync attempts request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sync attempts request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{Operation: "sync attempts", StatusCode: resp.StatusCode}
	}

	var result syncAttemptIngestResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode sync attempts response: %w", err)
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

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func intPtrIfNonZero(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func intMapFromJSON(raw string) map[string]int {
	if raw == "" {
		return map[string]int{}
	}
	var result map[string]int
	if err := json.Unmarshal([]byte(raw), &result); err != nil || result == nil {
		return map[string]int{}
	}
	return result
}

func stringMapFromJSON(raw string) map[string]string {
	if raw == "" {
		return map[string]string{}
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(raw), &result); err != nil || result == nil {
		return map[string]string{}
	}
	return result
}
