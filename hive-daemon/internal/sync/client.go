package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/db"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

const mutationProtocolVersion = 2

// syncCapabilityReproject is the exact string hive-api matches on before it will
// send a reproject event. A near miss is worse than silence: the server withholds
// every reproject forever while both ends report a healthy sync — which is why
// the string is owned by the shared contract module rather than copied here.
const syncCapabilityReproject = projectidentity.CapabilityReproject

// clientSyncCapabilities names the optional mutation ops this build can actually
// apply. It is a promise about ApplyRemoteMutation, not a wish list: the server
// withholds any op absent from it, and the pull cursor advances past the
// withheld event either way, so declaring an op this build cannot apply would
// abort the batch and strand the cursor instead of merely missing the event.
func clientSyncCapabilities() []string {
	return []string{syncCapabilityReproject}
}

var ErrProjectBlocked = errors.New("project is blocked")
var ErrProjectIdentityIncompatible = errors.New("incompatible project identity contract")

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

	// serverCapabilities is what the server declared on the last response that
	// declared anything. It is learned, never assumed: the zero value means
	// "nothing known yet", which withholds every optional op. See
	// serverSupports.
	capabilitiesMu     sync.RWMutex
	serverCapabilities map[string]bool
}

// serverSupports reports whether the server has DECLARED it understands an
// optional mutation op.
//
// It fails closed on purpose. The daemon already sends sync_capabilities and
// hive-api already echoes back what it understands, but the response field was
// never read, so this daemon pushed reproject at any server. Against a server
// that predates the op that is unrecoverable: the push is a hard error, the
// mutations are never REJECTED, and only rejection drops them from the journal —
// so they resend forever and sync is dead with nothing saying why.
//
// The owner's deploy order (API first, always before a release is cut) makes
// that near-impossible going forward; this covers the residual case, an API
// rollback after a release is already out.
//
// Unknown-until-proven costs one sync cycle at daemon startup: the first batch
// withholds reprojects, learns the declaration from that batch's own response,
// and the next batch sends them. Withheld mutations stay in the journal
// untouched, so nothing is lost either way.
func (c *client) serverSupports(capability string) bool {
	c.capabilitiesMu.RLock()
	defer c.capabilitiesMu.RUnlock()
	return c.serverCapabilities[capability]
}

// learnServerCapabilities records a server's declaration.
//
// An empty declaration is ignored rather than treated as a withdrawal: hive-api
// sends the field with omitempty, so a response that simply carries nothing is
// not evidence the server stopped understanding the op, and forgetting on every
// quiet response would flap the daemon between pushing and withholding.
func (c *client) learnServerCapabilities(declared []string) {
	if len(declared) == 0 {
		return
	}
	capabilities := make(map[string]bool, len(declared))
	for _, capability := range declared {
		capabilities[capability] = true
	}
	c.capabilitiesMu.Lock()
	defer c.capabilitiesMu.Unlock()
	c.serverCapabilities = capabilities
}

// withheldUnsupportedMutations splits the pending journal page into what this
// server can be sent and how many rows were held back.
//
// Only reproject is optional today, so the rule is written as the one gate it
// needs rather than as a capability framework: every other op predates the
// handshake and every server understands it. Order is preserved — the caller
// correlates the response back to these exact rows.
func withheldUnsupportedMutations(pending []db.MutationEnvelope, reprojectSupported bool) ([]db.MutationEnvelope, int) {
	if reprojectSupported {
		return pending, 0
	}
	sendable := make([]db.MutationEnvelope, 0, len(pending))
	withheld := 0
	for _, mutation := range pending {
		if mutation.Op == db.MutationOpReproject {
			withheld++
			continue
		}
		sendable = append(sendable, mutation)
	}
	return sendable, withheld
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

	// FromProject is the same relocation precondition sessionPayload carries.
	FromProject string `json:"from_project,omitempty"`
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

	// FromProject names the project literal the server currently holds for this
	// row, and asks it to relocate that exact row to Project. It is sent only
	// after a local identity migration renamed the row here; omitempty keeps an
	// ordinary push a plain idempotent re-push, since an empty from_project
	// matches nothing server-side.
	FromProject string `json:"from_project,omitempty"`
}

// syncRequest es el payload que enviamos a POST /sync.
// Sessions precede a memories para satisfacer la FK memories.session_id → sessions(id).
type syncRequest struct {
	Project                string                `json:"project"`
	ProjectIdentityVersion string                `json:"project_identity_version,omitempty"`
	Sessions               []sessionPayload      `json:"sessions"`
	Memories               []memoryPayload       `json:"memories"`
	Prompts                []promptPayload       `json:"prompts,omitempty"`
	LastSync               *time.Time            `json:"last_sync,omitempty"`
	ProtocolVersion        int                   `json:"protocol_version,omitempty"`
	MutationCursor         *db.MutationCursor    `json:"mutation_cursor,omitempty"`
	Mutations              []db.MutationEnvelope `json:"mutations,omitempty"`

	// SyncCapabilities declares the optional mutation ops this client can
	// apply; see clientSyncCapabilities. A server that predates the field
	// ignores it, and a daemon that declares nothing keeps working and simply
	// never sees the gated ops.
	SyncCapabilities []string `json:"sync_capabilities,omitempty"`

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
	Pushed                 int                   `json:"pushed"`
	Pulled                 []apiMemory           `json:"pulled"`
	Conflicts              int                   `json:"conflicts"`
	PromptsPushed          int                   `json:"prompts_pushed"`
	PulledSessions         []sessionPayload      `json:"pulled_sessions,omitempty"`
	NextMutationCursor     *db.MutationCursor    `json:"next_mutation_cursor,omitempty"`
	PulledMutations        []db.MutationEnvelope `json:"pulled_mutations,omitempty"`
	MutationResults        []mutationResult      `json:"mutation_results,omitempty"`
	CompatibilityMode      string                `json:"compatibility_mode,omitempty"`
	ProjectIdentityVersion string                `json:"project_identity_version,omitempty"`

	// SyncCapabilities is the server's half of the handshake this client's
	// request opens: the optional mutation ops IT understands. Absent means an
	// API that predates the field, which understands none of them — see
	// client.serverSupports.
	SyncCapabilities []string `json:"sync_capabilities,omitempty"`

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
	Action              string    `json:"action"`
	Generation          int64     `json:"generation"`
	BlockedAt           time.Time `json:"blocked_at"`
}

type projectBlockInboxResponse struct {
	Commands []projectBlockCommand `json:"commands"`
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

// canonicalizedMutations returns the wire form of the caller's journal rows,
// with every project literal folded to the canonical spelling.
//
// The caller keeps its own envelopes untouched: it correlates the response back
// to the journal by the literals it read, and a retry must resend the same rows.
// Memory is a pointer, so the envelope has to be deep-copied to that depth —
// copying only the slice would leave the payload shared, and folding through it
// would rewrite the caller's row while the sibling write to Project stayed local
// to the copy. Tombstone and Reproject carry no project this folds, so they are
// shared by pointer and never written through.
//
// Pinned by TestSyncDoesNotCanonicalizeTheCallersMutations.
func canonicalizedMutations(mutations []db.MutationEnvelope) []db.MutationEnvelope {
	if len(mutations) == 0 {
		return nil
	}
	canonical := make([]db.MutationEnvelope, len(mutations))
	for i, mutation := range mutations {
		mutation.Project = projectidentity.Canonical(mutation.Project).String()
		if mutation.Memory != nil {
			memory := *mutation.Memory
			memory.Project = projectidentity.Canonical(memory.Project).String()
			mutation.Memory = &memory
		}
		canonical[i] = mutation
	}
	return canonical
}

// sync envía sesiones, memorias y prompts locales, y recibe del servidor para un proyecto.
// sessions se serializa ANTES de memories (Decision 11: FK ordering).
// pullOpts opts into bounded legacy pull pagination (PR 2a/2b) — its zero
// value sends no pull_limit/cursor fields, preserving the pre-PR-2a
// unbounded-pull request shape exactly.
func (c *client) sync(ctx context.Context, token, project string,
	sessions []*models.Session, toSend []*models.Memory, prompts []*models.Prompt, lastSync *time.Time,
	mutations []db.MutationEnvelope, mutationCursor *db.MutationCursor, pullOpts pullOptions) (*syncResponse, error) {

	project = projectidentity.Canonical(project).String()
	sessionPayloads := make([]sessionPayload, 0, len(sessions))
	for _, s := range sessions {
		sessionPayloads = append(sessionPayloads, sessionPayload{
			ID:        s.ID,
			SyncID:    s.SyncID,
			Project:   projectidentity.Canonical(s.Project).String(),
			Directory: s.Directory,
			DevID:     s.DevID,
			Client:    s.Client,
			StartedAt: s.StartedAt,
			EndedAt:   s.EndedAt,
			Summary:   nilStringPtr(s.Summary),
			// Deliberately NOT canonicalized: this is the literal the server
			// stores, and folding it would make it equal to Project and match
			// nothing.
			FromProject: s.SyncFromProject,
		})
	}

	payloads := make([]memoryPayload, 0, len(toSend))
	for _, m := range toSend {
		payloads = append(payloads, memoryPayload{
			SyncID:        m.SyncID,
			Project:       projectidentity.Canonical(m.Project).String(),
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
			Project:   projectidentity.Canonical(p.Project).String(),
			Content:   p.Content,
			CreatedAt: p.CreatedAt,
			// Verbatim, for the same reason as sessionPayload.FromProject.
			FromProject: p.SyncFromProject,
		})
	}

	canonicalMutations := canonicalizedMutations(mutations)
	reqBody, err := json.Marshal(syncRequest{
		Project:                project,
		ProjectIdentityVersion: projectidentity.ContractVersion,
		Sessions:               sessionPayloads,
		Memories:               payloads,
		Prompts:                promptPayloads,
		LastSync:               lastSync,
		ProtocolVersion:        mutationProtocolVersion,
		MutationCursor:         mutationCursor,
		Mutations:              canonicalMutations,
		SyncCapabilities:       clientSyncCapabilities(),
		PullLimit:              pullOpts.Limit,
		PullCursor:             pullOpts.MemoriesCursor,
		PullSessionCursor:      pullOpts.SessionsCursor,
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
	if result.ProjectIdentityVersion != "" && result.ProjectIdentityVersion != projectidentity.ContractVersion {
		return nil, ErrProjectIdentityIncompatible
	}
	c.learnServerCapabilities(result.SyncCapabilities)

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

func (c *client) projectBlockInbox(ctx context.Context, token string) ([]projectBlockCommand, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.APIURL+"/project-blocks/inbox", nil)
	if err != nil {
		return nil, fmt.Errorf("build project block inbox request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("project block inbox request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // mixed-version API without inbox support
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{Operation: "project block inbox", StatusCode: resp.StatusCode}
	}
	var result projectBlockInboxResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); errors.Is(err, io.EOF) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("decode project block inbox response: %w", err)
	}
	return result.Commands, nil
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
