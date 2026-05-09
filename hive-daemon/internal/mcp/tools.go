package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/project"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/sanitize"
	hivesync "github.com/Thrasno/jarvis-dev/hive-daemon/internal/sync"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MaxObservationLength is the maximum allowed content size in runes (not bytes).
// Unicode-safe: a Japanese character counts as 1 rune even though it is 3 bytes.
const MaxObservationLength = 50_000

// MaxRecentPrompts is the maximum number of recent user prompts to include in mem_context.
const MaxRecentPrompts = 10

func registerTools(s *sdkmcp.Server, store MemoryStore, syncStore hivesync.SyncStore, syncer SyncRunner, cfg *hivesync.Config, activity *ActivityTracker, prompts PromptStore) {
	s.AddTool(&sdkmcp.Tool{
		Name:        "mem_session_start",
		Description: "Start a new named session to track tool calls and memory saves under a single lifecycle.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["id", "project", "directory", "dev_id", "client"],
			"properties": {
				"id":        {"type": "string", "description": "Caller-provided session ID (UUID recommended)"},
				"project":   {"type": "string", "description": "Project identifier"},
				"directory": {"type": "string", "description": "Working directory for this session"},
				"dev_id":    {"type": "string", "description": "Developer identity (e.g. from HIVE_DEV_ID env var)"},
				"client":    {"type": "string", "description": "Client identifier (e.g. 'claude-code', 'opencode')"},
				"session_id": {"type": "string", "description": "Unused — present for schema symmetry only"}
			}
		}`),
	}, memSessionStartHandler(store, activity))

	s.AddTool(&sdkmcp.Tool{
		Name:        "mem_session_end",
		Description: "End an active session. Records ended_at and optional summary.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["id"],
			"properties": {
				"id":      {"type": "string", "description": "Session ID to close"},
				"summary": {"type": "string", "description": "Optional final session summary"}
			}
		}`),
	}, memSessionEndHandler(store, activity))

	s.AddTool(&sdkmcp.Tool{
		Name:        "mem_save",
		Description: "Save a memory observation to Hive persistent storage",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["title", "content", "type", "project"],
			"properties": {
				"title":         {"type": "string", "description": "Short searchable title"},
				"content":       {"type": "string", "description": "Full memory content (markdown OK)"},
				"type":          {"type": "string", "description": "Category: architecture, decision, bugfix, pattern, discovery, config, preference, session_summary"},
				"project":       {"type": "string", "description": "Project identifier"},
				"topic_key":     {"type": "string", "description": "Stable key for upsert (e.g. 'arch/auth-model')"},
				"tags":          {"type": "array", "items": {"type": "string"}},
				"files_affected":{"type": "array", "items": {"type": "string"}},
				"session_id":    {"type": "string", "description": "Optional session ID; absent triggers lazy manual-save fallback"}
			}
		}`),
	}, memSaveHandler(store, syncer, cfg, activity))

	s.AddTool(&sdkmcp.Tool{
		Name:        "mem_search",
		Description: "Search memories using full-text search with BM25 ranking",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["query"],
			"properties": {
				"query":   {"type": "string", "description": "Search terms"},
				"project": {"type": "string", "description": "Filter by project (omit for all projects)"},
				"type":    {"type": "string", "description": "Filter by category (architecture, decision, bugfix, pattern, discovery, config, preference, session_summary)"},
				"limit":   {"type": "integer", "description": "Max results (default 10, max 50)"}
			}
		}`),
	}, memSearchHandler(store, activity))

	s.AddTool(&sdkmcp.Tool{
		Name:        "mem_get_observation",
		Description: "Retrieve a specific memory observation by its numeric ID",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["id"],
			"properties": {
				"id": {"type": "integer", "description": "Observation ID"}
			}
		}`),
	}, memGetObservationHandler(store, activity))

	s.AddTool(&sdkmcp.Tool{
		Name:        "mem_session_summary",
		Description: "Save a session summary memory. Title is auto-extracted from first line.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["content", "project"],
			"properties": {
				"content":    {"type": "string", "description": "Session summary in markdown"},
				"project":    {"type": "string", "description": "Project identifier"},
				"session_id": {"type": "string", "description": "Optional session ID; absent triggers lazy manual-save fallback"}
			}
		}`),
	}, memSessionSummaryHandler(store, activity))

	s.AddTool(&sdkmcp.Tool{
		Name:        "mem_context",
		Description: "Get recent memory context for a project, ordered by recency",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"project": {"type": "string", "description": "Filter by project (omit for all)"},
				"limit":   {"type": "integer", "description": "Max results (default 20)"}
			}
		}`),
	}, memContextHandler(store, prompts, activity))

	s.AddTool(&sdkmcp.Tool{
		Name:        "mem_sync",
		Description: "Sync local memories with the hive-api cloud server. Pushes unsynced local memories and pulls new ones from the server. Requires HIVE_API_URL, HIVE_API_EMAIL, HIVE_API_PASSWORD env vars or ~/.jarvis/sync.json config file.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["project"],
			"properties": {
				"project": {"type": "string", "description": "Project to sync (e.g. 'jarvis-dev')"}
			}
		}`),
	}, memSyncHandler(syncStore, syncer))

	s.AddTool(&sdkmcp.Tool{
		Name:        "mem_save_prompt",
		Description: "Persist a user prompt to the local Hive database for future recall and analysis",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["content", "project"],
			"properties": {
				"content":    {"type": "string", "description": "The user prompt text to persist"},
				"project":    {"type": "string", "description": "Project identifier"},
				"session_id": {"type": "string", "description": "Optional session ID; absent triggers lazy manual-save fallback"}
			}
		}`),
	}, memSavePromptHandler(store, prompts, activity))
}

// ─── Handlers ──────────────────────────────────────────────────────────────

func memSessionStartHandler(store MemoryStore, activity *ActivityTracker) sdkmcp.ToolHandler {
	return func(_ context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var p struct {
			ID        string `json:"id"`
			Project   string `json:"project"`
			Directory string `json:"directory"`
			DevID     string `json:"dev_id"`
			Client    string `json:"client"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}
		if p.ID == "" {
			return toolError(fmt.Errorf("id is required")), nil
		}
		if p.DevID == "" {
			return toolError(fmt.Errorf("dev_id is required")), nil
		}
		if p.Client == "" {
			return toolError(fmt.Errorf("client is required")), nil
		}

		if err := store.CreateSession(p.ID, p.Project, p.Directory, p.DevID, p.Client); err != nil {
			return toolError(fmt.Errorf("create session failed: %w", err)), nil
		}

		// CRIT-6: record activity in BOTH namespaces — per-session (real attribution)
		// and per-project (legacy nudge logic depends on project-keyed counters).
		activity.RecordToolCallForSession(p.ID)
		activity.RecordToolCall(p.Project)

		return toolJSON(map[string]any{
			"session_id": p.ID,
			"started_at": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func memSessionEndHandler(store MemoryStore, activity *ActivityTracker) sdkmcp.ToolHandler {
	return func(_ context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var p struct {
			ID      string `json:"id"`
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}
		if p.ID == "" {
			return toolError(fmt.Errorf("id is required")), nil
		}

		// Validate session exists and is open before ending it.
		sess, err := store.GetSession(p.ID)
		if err != nil {
			return toolError(fmt.Errorf("session %q not found", p.ID)), nil
		}
		if sess.EndedAt != nil {
			return toolError(fmt.Errorf("session %q already ended at %s", p.ID, sess.EndedAt.UTC().Format(time.RFC3339))), nil
		}

		if err := store.EndSession(p.ID, p.Summary); err != nil {
			return toolError(fmt.Errorf("end session failed: %w", err)), nil
		}

		activity.ClearSession(p.ID)

		endedAt := time.Now().UTC().Format(time.RFC3339)
		return toolJSON(map[string]any{
			"session_id": p.ID,
			"ended_at":   endedAt,
		})
	}
}

func memSaveHandler(store MemoryStore, syncer SyncRunner, cfg *hivesync.Config, activity *ActivityTracker) sdkmcp.ToolHandler {
	return func(_ context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var p struct {
			Title         string   `json:"title"`
			Content       string   `json:"content"`
			Type          string   `json:"type"`
			Project       string   `json:"project"`
			TopicKey      *string  `json:"topic_key"`
			Tags          []string `json:"tags"`
			FilesAffected []string `json:"files_affected"`
			SessionID     string   `json:"session_id"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}
		if p.Title == "" || p.Content == "" || p.Project == "" {
			return toolError(fmt.Errorf("title, content, and project are required")), nil
		}

		resolved, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{Project: p.Project, SessionID: p.SessionID})
		if err != nil {
			return toolValidationError(err), nil
		}
		p.Project = resolved.Project

		// Lazy session fallback: when session_id is absent, resolve via EnsureManualSaveSession.
		sessionID, err := resolveSessionID(p.SessionID, p.Project, store)
		if err != nil {
			return toolError(fmt.Errorf("resolve session: %w", err)), nil
		}

		// Guard: reject content exceeding MaxObservationLength runes (Unicode-safe).
		if runeCount := utf8.RuneCountInString(p.Content); runeCount > MaxObservationLength {
			return toolError(fmt.Errorf(
				"content too long: %d runes (max %d). Summarize or split into multiple observations",
				runeCount, MaxObservationLength,
			)), nil
		}

		// Strip private tags from title and content at the handler boundary.
		// TopicKey is intentionally excluded — see design ADR-3.
		titleRes := sanitize.Strip(p.Title)
		contentRes := sanitize.Strip(p.Content)
		strippedCount := titleRes.Count + contentRes.Count

		mem := &models.Memory{
			Title:         titleRes.Clean,
			Content:       contentRes.Clean,
			Category:      p.Type,
			Project:       p.Project,
			TopicKey:      p.TopicKey,
			Tags:          p.Tags,
			FilesAffected: p.FilesAffected,
			SessionID:     sessionID,
		}

		id, err := store.SaveMemory(mem)
		if err != nil {
			return toolError(fmt.Errorf("save failed: %w", err)), nil
		}

		// CRIT-6: per-session save counter mirrors the project counter so callers
		// querying SessionSaves(sessionID) see real attribution post-fix.
		activity.RecordSaveForSession(sessionID)
		activity.RecordSave(p.Project)

		// Auto-sync: spawn background goroutine if enabled
		if cfg != nil && cfg.AutoSync && syncer != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if _, err := syncer.Sync(ctx, p.Project); err != nil && !isQuietSyncBlocker(err) {
					logger.Log.Printf("warn: autosync %s: %v", p.Project, err)
				}
			}()
		}

		return toolJSON(map[string]any{
			"id":             id,
			"status":         "saved",
			"stripped":       strippedCount > 0,
			"stripped_count": strippedCount,
		})
	}
}

func memSearchHandler(store MemoryStore, activity *ActivityTracker) sdkmcp.ToolHandler {
	return func(_ context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var p struct {
			Query    string `json:"query"`
			Project  string `json:"project"`
			Category string `json:"type"` // JSON "type" maps to Category to avoid Go reserved word
			Limit    int    `json:"limit"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}
		if p.Limit <= 0 {
			p.Limit = 10
		}
		if p.Limit > 50 {
			p.Limit = 50
		}

		activity.RecordToolCall(p.Project)

		results, err := store.Search(p.Query, p.Project, p.Category, p.Limit)
		if err != nil {
			return toolError(fmt.Errorf("search failed: %w", err)), nil
		}
		if results == nil {
			results = []*models.Memory{}
		}

		formatted := formatSearchResults(results, p.Query)
		formatted += activity.NudgeIfNeeded(p.Project)

		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: formatted}},
		}, nil
	}
}

func memGetObservationHandler(store MemoryStore, activity *ActivityTracker) sdkmcp.ToolHandler {
	return func(_ context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var p struct {
			ID *float64 `json:"id"` // JSON numbers decode as float64
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}
		if p.ID == nil {
			return toolError(fmt.Errorf("id is required")), nil
		}

		mem, err := store.GetMemory(int64(*p.ID))
		if err != nil {
			return toolError(err), nil
		}

		// Record tool call after successful fetch — project is only known from the memory itself.
		activity.RecordToolCall(mem.Project)

		return toolJSON(mem)
	}
}

func memSessionSummaryHandler(store MemoryStore, activity *ActivityTracker) sdkmcp.ToolHandler {
	return func(_ context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var p struct {
			Content   string `json:"content"`
			Project   string `json:"project"`
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}
		if p.Content == "" {
			return toolError(fmt.Errorf("content is required")), nil
		}
		if p.Project == "" {
			return toolError(fmt.Errorf("project is required")), nil
		}

		resolved, err := project.ValidateWriteProject(context.Background(), store, project.WriteInput{Project: p.Project, SessionID: p.SessionID})
		if err != nil {
			return toolValidationError(err), nil
		}
		p.Project = resolved.Project

		// Guard: same 50K rune limit as memSaveHandler.
		if runeCount := utf8.RuneCountInString(p.Content); runeCount > MaxObservationLength {
			return toolError(fmt.Errorf(
				"content too long: %d runes (max %d). Summarize or split into multiple observations",
				runeCount, MaxObservationLength,
			)), nil
		}

		// When session_id is explicit: validate the session is open before saving.
		// When absent: lazy fallback to manual-save-{project}.
		var effectiveSessionID string
		if p.SessionID != "" {
			sess, err := store.GetSession(p.SessionID)
			if err != nil {
				return toolError(fmt.Errorf("session %q not found", p.SessionID)), nil
			}
			if sess.EndedAt != nil {
				return toolError(fmt.Errorf("session %q already ended at %s", p.SessionID, sess.EndedAt.UTC().Format(time.RFC3339))), nil
			}
			effectiveSessionID = p.SessionID
		} else {
			var err error
			effectiveSessionID, err = store.EnsureManualSaveSession(p.Project)
			if err != nil {
				return toolError(fmt.Errorf("resolve session: %w", err)), nil
			}
		}

		// Strip private tags from content. Title is derived from stripped content (ADR-6).
		contentRes := sanitize.Strip(p.Content)

		mem := &models.Memory{
			Title:     titleFromContent(contentRes.Clean),
			Content:   contentRes.Clean,
			Category:  "session_summary",
			Project:   p.Project,
			SessionID: effectiveSessionID,
		}

		id, err := store.SaveMemory(mem)
		if err != nil {
			return toolError(fmt.Errorf("save failed: %w", err)), nil
		}

		// CRIT-6: per-session save counter for mem_session_summary.
		activity.RecordSaveForSession(effectiveSessionID)
		activity.RecordSave(p.Project)

		jsonBytes, err := json.Marshal(map[string]any{
			"id":             id,
			"status":         "saved",
			"stripped":       contentRes.Count > 0,
			"stripped_count": contentRes.Count,
		})
		if err != nil {
			return toolError(fmt.Errorf("marshal response: %w", err)), nil
		}
		responseText := string(jsonBytes) + activity.SessionStats(p.Project)

		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: responseText}},
		}, nil
	}
}

func memContextHandler(store MemoryStore, prompts PromptStore, activity *ActivityTracker) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var p struct {
			Project string `json:"project"`
			Limit   int    `json:"limit"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}
		if p.Limit <= 0 {
			p.Limit = 20
		}

		activity.RecordToolCall(p.Project)

		results, err := store.ListMemories(p.Project, p.Limit)
		if err != nil {
			return toolError(fmt.Errorf("list failed: %w", err)), nil
		}
		if results == nil {
			results = []*models.Memory{}
		}

		var recentPrompts []*models.Prompt
		if prompts != nil {
			recentPrompts, err = prompts.ListRecentPrompts(ctx, p.Project, MaxRecentPrompts)
			if err != nil {
				// Log and continue — prompt fetch failure should not break mem_context
				logger.Log.Printf("warn: list recent prompts: %v", err)
				recentPrompts = nil
			}
		}

		formatted := formatContext(recentPrompts, results, p.Project)
		formatted += activity.NudgeIfNeeded(p.Project)

		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: formatted}},
		}, nil
	}
}

func memSavePromptHandler(store MemoryStore, prompts PromptStore, activity *ActivityTracker) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		if prompts == nil {
			return toolError(fmt.Errorf("prompts store not configured")), nil
		}
		var p struct {
			Content   string `json:"content"`
			Project   string `json:"project"`
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}
		if strings.TrimSpace(p.Content) == "" {
			return toolError(fmt.Errorf("content is required")), nil
		}
		if strings.TrimSpace(p.Project) == "" {
			return toolError(fmt.Errorf("project is required")), nil
		}
		if runeCount := utf8.RuneCountInString(p.Content); runeCount > MaxObservationLength {
			return toolError(fmt.Errorf(
				"content too long: %d runes (max %d). Summarize the prompt before saving",
				runeCount, MaxObservationLength,
			)), nil
		}

		resolved, err := project.ValidateWriteProject(ctx, store, project.WriteInput{Project: p.Project, SessionID: p.SessionID})
		if err != nil {
			return toolValidationError(err), nil
		}
		p.Project = resolved.Project

		// Lazy session fallback — same pattern as memSaveHandler.
		sessionID, err := resolveSessionID(p.SessionID, p.Project, store)
		if err != nil {
			return toolError(fmt.Errorf("resolve session: %w", err)), nil
		}

		// Strip private tags from content at the handler boundary.
		contentRes := sanitize.Strip(p.Content)

		prompt, err := prompts.SavePrompt(ctx, p.Project, contentRes.Clean)
		if err != nil {
			return toolError(fmt.Errorf("save failed: %w", err)), nil
		}
		// CRIT-6: per-session prompt capture so CurrentPromptForSession works.
		activity.RecordPromptForSession(sessionID, contentRes.Clean)
		activity.RecordSaveForSession(sessionID)
		return toolJSON(map[string]any{
			"id":             prompt.ID,
			"created_at":     prompt.CreatedAt.Format(time.RFC3339),
			"stripped":       contentRes.Count > 0,
			"stripped_count": contentRes.Count,
		})
	}
}

// ─── Formatters ────────────────────────────────────────────────────────────

// formatContext renders recent prompts (if any) followed by memories as compact markdown.
// Prompts are prepended as a "### Recent User Prompts" section.
// Returns a footer with count and hint to use mem_get_observation.
func formatContext(recentPrompts []*models.Prompt, memories []*models.Memory, project string) string {
	var b strings.Builder

	// Prepend recent user prompts section when available
	if len(recentPrompts) > 0 {
		b.WriteString("### Recent User Prompts\n")
		for _, p := range recentPrompts {
			content := truncateRunes(p.Content, 200)
			fmt.Fprintf(&b, "- %s — %s\n", p.CreatedAt.Format("2006-01-02 15:04"), content)
		}
		b.WriteString("\n")
	}

	if len(memories) == 0 {
		b.WriteString(fmt.Sprintf("No memories found for project %q.", project))
		return b.String()
	}

	for _, m := range memories {
		// Header: ### [ID] Title (category)
		fmt.Fprintf(&b, "### [%d] %s (%s)\n", m.ID, m.Title, m.Category)

		// Metadata line: _project | created_by | YYYY-MM-DD_
		fmt.Fprintf(&b, "_%s | %s | %s_\n", m.Project, m.CreatedBy, m.CreatedAt.Format("2006-01-02"))

		// Truncated content preview
		b.WriteString(truncateRunes(m.Content, 300))
		b.WriteByte('\n')

		// Tags line — omitted when empty
		if len(m.Tags) > 0 {
			fmt.Fprintf(&b, "\nTags: %s\n", strings.Join(m.Tags, ", "))
		}

		b.WriteString("---\n\n")
	}

	fmt.Fprintf(&b, "📋 %d memories shown. Use mem_get_observation(id) for full content.\n", len(memories))
	return b.String()
}

// formatSearchResults renders search results as compact markdown with truncated previews.
// query is included in the footer for context.
func formatSearchResults(memories []*models.Memory, query string) string {
	if len(memories) == 0 {
		return fmt.Sprintf("No results found for %q.", query)
	}

	var b strings.Builder
	for _, m := range memories {
		// Header with impact score if non-zero
		if m.ImpactScore > 0 {
			fmt.Fprintf(&b, "### [%d] %s (%s) ⭐%d\n", m.ID, m.Title, m.Category, m.ImpactScore)
		} else {
			fmt.Fprintf(&b, "### [%d] %s (%s)\n", m.ID, m.Title, m.Category)
		}

		// Metadata: _project | YYYY-MM-DD_
		fmt.Fprintf(&b, "_%s | %s_\n", m.Project, m.CreatedAt.Format("2006-01-02"))

		// Content preview
		b.WriteString(truncateRunes(m.Content, 300))
		b.WriteByte('\n')

		b.WriteString("---\n\n")
	}

	fmt.Fprintf(&b, "🔍 %d results for %q. Use mem_get_observation(id) for full content.\n", len(memories), query)
	return b.String()
}

// truncateRunes returns the first maxRunes runes of s.
// If truncation occurs, appends "..." to the result.
// Uses range-based iteration — Unicode-safe, single pass with early exit.
func truncateRunes(s string, maxRunes int) string {
	count := 0
	for i := range s {
		if count >= maxRunes {
			return s[:i] + "..."
		}
		count++
	}
	return s // no truncation needed
}

// ─── Helpers ───────────────────────────────────────────────────────────────

func toolError(err error) *sdkmcp.CallToolResult {
	r := &sdkmcp.CallToolResult{}
	r.SetError(err)
	return r
}

func toolValidationError(err error) *sdkmcp.CallToolResult {
	var validationErr *project.ValidationError
	if !errors.As(err, &validationErr) {
		return toolError(err)
	}
	result, marshalErr := toolJSON(validationErr)
	if marshalErr != nil {
		return toolError(marshalErr)
	}
	result.IsError = true
	return result
}

func toolJSON(v any) (*sdkmcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError(fmt.Errorf("marshal response: %w", err)), nil
	}
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: string(b)}},
	}, nil
}

func memSyncHandler(syncStore hivesync.SyncStore, syncer SyncRunner) sdkmcp.ToolHandler {
	// syncer se captura por referencia — la inicialización lazy persiste entre llamadas.
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		// Lazy init: si el daemon arrancó sin las vars (proceso en caché, env tardío),
		// intentamos cargarlas ahora en cada llamada hasta que estén disponibles.
		if syncer == nil && syncStore != nil {
			cfg, err := hivesync.Load()
			if err != nil {
				return toolError(fmt.Errorf("sync config error: %w", err)), nil
			}
			if cfg != nil {
				syncer = hivesync.New(cfg, syncStore)
			}
		}
		if syncer == nil {
			return toolError(fmt.Errorf(
				"sync not configured — set HIVE_API_URL, HIVE_API_EMAIL, HIVE_API_PASSWORD env vars or create ~/.jarvis/sync.json (chmod 600)",
			)), nil
		}

		var p struct {
			Project string `json:"project"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}
		if p.Project == "" {
			return toolError(fmt.Errorf("project es requerido")), nil
		}

		result, err := syncer.Sync(ctx, p.Project)
		if err != nil {
			if errors.Is(err, hivesync.ErrSyncInFlight) {
				return toolJSON(map[string]any{
					"project": p.Project,
					"status":  "in_flight",
				})
			}

			var backoffErr *hivesync.BackoffError
			if errors.As(err, &backoffErr) {
				return toolJSON(map[string]any{
					"project":  p.Project,
					"retry_at": backoffErr.RetryAt.UTC().Format(time.RFC3339),
					"status":   "backoff",
				})
			}

			return toolError(fmt.Errorf("sync failed: %w", err)), nil
		}

		return toolJSON(map[string]any{
			"pushed":    result.Pushed,
			"pulled":    result.Pulled,
			"conflicts": result.Conflicts,
			"project":   result.Project,
			"status":    "ok",
		})
	}
}

// resolveSessionID returns the effective session ID for a save operation.
// When explicit is non-empty it is used directly; otherwise EnsureManualSaveSession
// is called to lazily create (or return) the always-open 'manual-save-{project}' session.
// This mirrors engram's handleSave lazy-dispatch pattern (internal/mcp/mcp.go:765-792).
func resolveSessionID(explicit, project string, store MemoryStore) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	return store.EnsureManualSaveSession(project)
}

func isQuietSyncBlocker(err error) bool {
	if errors.Is(err, hivesync.ErrSyncInFlight) {
		return true
	}
	var backoffErr *hivesync.BackoffError
	return errors.As(err, &backoffErr)
}

// titleFromContent extracts the first non-empty line from markdown content,
// stripping the leading '#' if present. Falls back to a timestamp-based title.
func titleFromContent(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "## ")
		line = strings.TrimPrefix(line, "# ")
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return "Session Summary " + time.Now().Format("2006-01-02 15:04")
}
