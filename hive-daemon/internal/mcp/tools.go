package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/logger"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/project"
	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sanitize"
	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
	"github.com/Thrasno/jarvis-ai-devs/hivederive"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MaxObservationLength is the maximum allowed content size in runes (not bytes).
// Unicode-safe: a Japanese character counts as 1 rune even though it is 3 bytes.
const MaxObservationLength = 50_000

// MaxRecentPrompts is the maximum number of recent user prompts to include in mem_context.
const MaxRecentPrompts = 10

func registerTools(s *sdkmcp.Server, store MemoryStore, syncRuntime *syncRuntime, activity *ActivityTracker, prompts PromptStore) {
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
				"directory":     {"type": "string", "description": "Working directory; when project is empty, the daemon derives the canonical project name from this path"},
				"topic_key":     {"type": "string", "description": "Grouping/context key for related memories (e.g. 'arch/auth-model'). Every save creates a new row; topic_key groups and aids retrieval — saving twice with the same key creates two distinct rows."},
				"tags":          {"type": "array", "items": {"type": "string"}},
				"files_affected":{"type": "array", "items": {"type": "string"}},
				"session_id":    {"type": "string", "description": "Optional session ID; absent triggers lazy manual-save fallback"},
				"capture_prompt":{"type": "boolean", "description": "Whether to best-effort link this memory to the latest prompt for the same project/session. Defaults to true; automated artifacts should pass false."},
				"recovery_token": {"type": "string", "description": "Recovery token returned by an ambiguous project response"},
				"project_choice_reason": {"type": "string", "description": "Original ambiguous project/context used when retrying with recovery_token"}
			}
		}`),
	}, memSaveHandler(store, syncRuntime, activity, prompts))

	s.AddTool(&sdkmcp.Tool{
		Name:        "mem_suggest_topic_key",
		Description: "Suggest a deterministic topic_key for a memory title and supported type without persisting anything.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["title", "type"],
			"properties": {
				"title": {"type": "string", "description": "Memory title to normalize into a slug"},
				"type":  {"type": "string", "enum": ["architecture", "bugfix", "decision", "pattern", "config", "discovery", "preference"], "description": "Supported category: architecture, bugfix, decision, pattern, config, discovery, preference"}
			}
		}`),
	}, memSuggestTopicKeyHandler())

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
				"directory":  {"type": "string", "description": "Working directory; the daemon derives the canonical project name from this path to self-heal an unknown/stale project. The filesystem-derived name wins over the supplied project on conflict."},
				"session_id": {"type": "string", "description": "Optional session ID; absent triggers lazy manual-save fallback"},
				"recovery_token": {"type": "string", "description": "Recovery token returned by an ambiguous project response"},
				"project_choice_reason": {"type": "string", "description": "Original ambiguous project/context used when retrying with recovery_token"}
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
	}, memSyncHandler(syncRuntime))

	s.AddTool(&sdkmcp.Tool{
		Name:        "mem_save_prompt",
		Description: "Persist a user prompt to the local Hive database for future recall and analysis",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["content", "project"],
			"properties": {
				"content":    {"type": "string", "description": "The user prompt text to persist"},
				"project":    {"type": "string", "description": "Project identifier"},
				"session_id": {"type": "string", "description": "Optional session ID; absent triggers lazy manual-save fallback"},
				"recovery_token": {"type": "string", "description": "Recovery token returned by an ambiguous project response"},
				"project_choice_reason": {"type": "string", "description": "Original ambiguous project/context used when retrying with recovery_token"}
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

		// Derive the effective project from directory when project is empty.
		p.Project, _ = project.ResolveEffectiveProject(p.Project, p.Directory)

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

func memSaveHandler(store MemoryStore, syncRuntime *syncRuntime, activity *ActivityTracker, prompts PromptStore) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var p struct {
			Title               string   `json:"title"`
			Content             string   `json:"content"`
			Type                string   `json:"type"`
			Project             string   `json:"project"`
			Directory           string   `json:"directory"`
			TopicKey            *string  `json:"topic_key"`
			Tags                []string `json:"tags"`
			FilesAffected       []string `json:"files_affected"`
			SessionID           string   `json:"session_id"`
			CapturePrompt       *bool    `json:"capture_prompt"`
			RecoveryToken       string   `json:"recovery_token"`
			ProjectChoiceReason string   `json:"project_choice_reason"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}

		// T-05: derive-then-validate with provenance-gated project_unknown escape.
		// When project is empty and directory is provided, derive the canonical
		// name from the real filesystem (git remote → basename → "default").
		// The provenance bool tracks whether the name came from derivation (true)
		// or was supplied by the caller (false). Only a derived name may bypass
		// the project_unknown gate; an assistant-supplied name never does.
		effective, derived := project.ResolveEffectiveProject(p.Project, p.Directory)
		if effective != "" {
			p.Project = effective
		}

		if p.Title == "" || p.Content == "" || p.Project == "" {
			return toolError(fmt.Errorf("title, content, and project are required")), nil
		}

		resolved, err := project.ValidateWriteProject(ctx, store, project.WriteInput{Project: p.Project, SessionID: p.SessionID, RecoveryToken: p.RecoveryToken, ProjectChoiceReason: p.ProjectChoiceReason})
		if err != nil {
			var validationErr *project.ValidationError
			if errors.As(err, &validationErr) &&
				validationErr.Code == project.CodeProjectUnknown &&
				derived &&
				p.Project != "default" {
				// Provenance-gated escape: the name came from real git/filesystem
				// derivation, not from the assistant. Allow the write — the memory
				// row itself registers the derived project in KnownProjects.
				// "default" is explicitly excluded: it is a sentinel for "could not
				// derive a real name" and must never auto-register as a pooling target.
				resolved = project.Result{Project: p.Project}
			} else {
				return toolValidationError(err), nil
			}
		}
		p.Project = resolved.Project

		sessionID := p.SessionID
		if sessionID == "" {
			sessionID = "manual-save-" + p.Project
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

		var promptID int64
		if shouldCapturePrompt(p.CapturePrompt) && prompts != nil {
			prompt, err := prompts.LatestPromptForSession(ctx, p.Project, sessionID)
			if err != nil {
				logger.Log.Printf("warn: latest prompt lookup failed for project=%q session_id=%q: %v", p.Project, sessionID, err)
			} else if prompt != nil {
				promptID = prompt.ID
			}
		}

		mem := &models.Memory{
			Title:         titleRes.Clean,
			Content:       contentRes.Clean,
			Category:      p.Type,
			Project:       p.Project,
			TopicKey:      p.TopicKey,
			Tags:          p.Tags,
			FilesAffected: p.FilesAffected,
			SessionID:     sessionID,
			PromptID:      promptID,
		}

		var id int64
		if p.SessionID == "" {
			id, err = store.SaveMemoryWithManualSession(mem)
		} else {
			id, err = store.SaveMemory(mem)
		}
		if err != nil {
			return toolError(fmt.Errorf("save failed: %w", err)), nil
		}

		// CRIT-6: per-session save counter mirrors the project counter so callers
		// querying SessionSaves(sessionID) see real attribution post-fix.
		activity.RecordSaveForSession(sessionID)
		activity.RecordSave(p.Project)

		autosyncStatus := "disabled"
		syncer, syncStatus, syncConfigErr := syncRuntime.current()
		if syncConfigErr != nil {
			autosyncStatus = "config_error"
		} else if syncStatus.AutoSync && syncer != nil {
			autosyncStatus = "queued"
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if _, err := syncer.Sync(ctx, p.Project); err != nil && !isQuietSyncBlocker(err) {
					logger.Log.Printf("warn: autosync %s: %v", p.Project, err)
				}
			}()
		}

		return toolJSON(map[string]any{
			"id":                     id,
			"status":                 "saved",
			"stripped":               strippedCount > 0,
			"stripped_count":         strippedCount,
			"autosync_status":        autosyncStatus,
			"autosync_config_source": syncStatus.Source,
			"auto_sync":              syncStatus.AutoSync,
			"autosync_warnings":      syncStatus.Warnings,
		})
	}
}

func memSuggestTopicKeyHandler() sdkmcp.ToolHandler {
	return func(_ context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var p struct {
			Title string `json:"title"`
			Type  string `json:"type"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}

		title := strings.TrimSpace(p.Title)
		if title == "" {
			return toolError(fmt.Errorf("title is required")), nil
		}

		topicType := strings.TrimSpace(p.Type)
		if !isSupportedTopicKeyType(topicType) {
			return toolError(fmt.Errorf("type must be one of: architecture, bugfix, decision, pattern, config, discovery, preference")), nil
		}

		slug := slugFromTitle(title)
		if slug == "" {
			return toolError(fmt.Errorf("title must contain at least one alphanumeric character")), nil
		}

		return toolJSON(map[string]any{
			"topic_key": topicType + "/" + slug,
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
			Content             string `json:"content"`
			Project             string `json:"project"`
			Directory           string `json:"directory"`
			SessionID           string `json:"session_id"`
			RecoveryToken       string `json:"recovery_token"`
			ProjectChoiceReason string `json:"project_choice_reason"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}
		if p.Content == "" {
			return toolError(fmt.Errorf("content is required")), nil
		}

		ctx := context.Background()

		// Validate the caller-supplied project FIRST. A valid, known project is
		// authoritative and is never overridden by the working directory — this
		// mirrors memSaveHandler / ResolveEffectiveProject, where a non-empty
		// caller project short-circuits derivation.
		//
		// Filesystem derivation is a SELF-HEAL fallback that runs ONLY when the
		// caller project is unknown (or empty) AND a usable directory is present,
		// outside a recovery retry. In that escape the filesystem-derived name
		// wins — recovering an unknown/stale project instead of failing the
		// summary — because the caller could not name a project the daemon knows.
		resolved, err := project.ValidateWriteProject(ctx, store, project.WriteInput{Project: p.Project, SessionID: p.SessionID, RecoveryToken: p.RecoveryToken, ProjectChoiceReason: p.ProjectChoiceReason})
		if err != nil {
			var validationErr *project.ValidationError
			canSelfHeal := errors.As(err, &validationErr) &&
				validationErr.Code == project.CodeProjectUnknown &&
				p.RecoveryToken == "" &&
				strings.TrimSpace(p.Directory) != ""
			if !canSelfHeal {
				return toolValidationError(err), nil
			}

			// Derive directly from the real filesystem (not the
			// "default"-collapsing DeriveFromDirectory adapter) so a failure
			// surfaces as a typed error rather than masquerading as a derived
			// name.
			name, derr := hivederive.Derive(p.Directory)
			if derr != nil {
				// Surface a structured project_unknown carrying the typed derive
				// reason instead of a generic uncoded "project is required", so
				// the caller keeps the recovery_token contract and gets an
				// actionable message.
				return toolValidationError(&project.ValidationError{
					Code:    project.CodeProjectUnknown,
					Message: fmt.Sprintf("project is required: could not derive a project name from directory %q: %v", p.Directory, derr),
				}), nil
			}
			if name == "" || name == "default" {
				// "default" is the reserved pooling sentinel: it must never
				// auto-register as a project. Keep the original project_unknown.
				return toolValidationError(err), nil
			}

			// Derived name wins on conflict: the caller project was unknown, so
			// the session-summary row registers the derived project instead.
			p.Project = name
		} else {
			p.Project = resolved.Project
		}

		// Guard: same 50K rune limit as memSaveHandler.
		if runeCount := utf8.RuneCountInString(p.Content); runeCount > MaxObservationLength {
			return toolError(fmt.Errorf(
				"content too long: %d runes (max %d). Summarize or split into multiple observations",
				runeCount, MaxObservationLength,
			)), nil
		}

		// Explicit sessions retain lifecycle validation. The store creates manual
		// fallback sessions atomically with the memory below.
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
			effectiveSessionID = "manual-save-" + p.Project
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

		var id int64
		if p.SessionID == "" {
			id, err = store.SaveMemoryWithManualSession(mem)
		} else {
			id, err = store.SaveMemory(mem)
		}
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
			Content             string `json:"content"`
			Project             string `json:"project"`
			SessionID           string `json:"session_id"`
			RecoveryToken       string `json:"recovery_token"`
			ProjectChoiceReason string `json:"project_choice_reason"`
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

		resolved, err := project.ValidateWriteProject(ctx, store, project.WriteInput{Project: p.Project, SessionID: p.SessionID, RecoveryToken: p.RecoveryToken, ProjectChoiceReason: p.ProjectChoiceReason})
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

		prompt, err := prompts.SavePromptForSession(ctx, p.Project, sessionID, contentRes.Clean)
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
		// Header: ### [ID] Title (category)
		fmt.Fprintf(&b, "### [%d] %s (%s)\n", m.ID, m.Title, m.Category)

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

const maxTopicKeySlugLength = 60

var supportedTopicKeyTypes = map[string]struct{}{
	"architecture": {},
	"bugfix":       {},
	"decision":     {},
	"pattern":      {},
	"config":       {},
	"discovery":    {},
	"preference":   {},
}

func isSupportedTopicKeyType(topicType string) bool {
	_, ok := supportedTopicKeyTypes[topicType]
	return ok
}

func slugFromTitle(title string) string {
	text := splitCamelCaseBoundaries(title)
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, "n+1", "n plus one")

	var b strings.Builder
	lastWasHyphen := true
	for _, r := range text {
		switch {
		case r == '+':
			writeSlugWord(&b, "plus", &lastWasHyphen)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastWasHyphen = false
		default:
			writeSlugHyphen(&b, &lastWasHyphen)
		}
	}

	slug := strings.Trim(b.String(), "-")
	return truncateSlugRunes(slug, maxTopicKeySlugLength)
}

func truncateSlugRunes(slug string, maxRunes int) string {
	if utf8.RuneCountInString(slug) <= maxRunes {
		return slug
	}

	runeCount := 0
	for i := range slug {
		if runeCount == maxRunes {
			return strings.TrimRight(slug[:i], "-")
		}
		runeCount++
	}
	return strings.TrimRight(slug, "-")
}

func splitCamelCaseBoundaries(s string) string {
	var b strings.Builder
	var prev rune
	for i, r := range s {
		if i > 0 && unicode.IsLower(prev) && unicode.IsUpper(r) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}

func writeSlugWord(b *strings.Builder, word string, lastWasHyphen *bool) {
	writeSlugHyphen(b, lastWasHyphen)
	b.WriteString(word)
	*lastWasHyphen = false
}

func writeSlugHyphen(b *strings.Builder, lastWasHyphen *bool) {
	if !*lastWasHyphen {
		b.WriteByte('-')
		*lastWasHyphen = true
	}
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

func memSyncHandler(syncRuntime *syncRuntime) sdkmcp.ToolHandler {
	// syncRuntime lazy-loads config and recreates the syncer when credentials change.
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		syncer, syncStatus, err := syncRuntime.current()
		if err != nil {
			return toolError(fmt.Errorf("sync config error: %w", err)), nil
		}
		if syncer == nil {
			return toolError(errors.New(syncNotConfiguredMessage(runtime.GOOS))), nil
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

		// mem_sync drives a manual, user-triggered sync: drain the backlog
		// across as many batches as needed instead of the single-step
		// TriggerAuto policy used by the background/auto-sync path (design
		// §2.1, §4.3, PR 1b-ii). PR 3 (task 3.2) surfaces the full
		// DrainOutcome in the JSON response instead of discarding it.
		result, outcome, err := syncer.Drain(ctx, p.Project, hivesync.TriggerManual)
		if err != nil {
			if errors.Is(err, hivesync.ErrSyncInFlight) {
				return toolJSON(map[string]any{
					"project":       p.Project,
					"status":        "in_flight",
					"config_source": syncStatus.Source,
					"auto_sync":     syncStatus.AutoSync,
				})
			}

			var backoffErr *hivesync.BackoffError
			if errors.As(err, &backoffErr) {
				return toolJSON(map[string]any{
					"project":       p.Project,
					"retry_at":      backoffErr.RetryAt.UTC().Format(time.RFC3339),
					"status":        "backoff",
					"config_source": syncStatus.Source,
					"auto_sync":     syncStatus.AutoSync,
				})
			}

			wrapped := fmt.Errorf("sync failed: %w", err)
			if result != nil {
				response, marshalErr := toolJSON(map[string]any{
					"status": "error",
					"error":  wrapped.Error(),
					"partial_result": map[string]any{
						"batches_done":      outcome.BatchesDone,
						"pushed":            result.Pushed,
						"pulled":            result.Pulled,
						"marked":            0,
						"conflicts":         result.Conflicts,
						"drain_state":       drainStateJSON(outcome.State),
						"drain_reason":      string(outcome.Reason),
						"remaining_push":    outcome.RemainingPush,
						"remaining_pull":    0,
						"remaining_mark":    0,
						"batches_remaining": outcome.BatchesRemaining,
					},
				})
				if marshalErr != nil {
					return toolError(marshalErr), nil
				}
				response.IsError = true
				return response, nil
			}
			return toolError(wrapped), nil
		}

		// DrainExpectedPending (either reason: auto-single-step, no-progress,
		// or iteration-cap) is a SUCCESS response — backlog remaining after a
		// manual drain is not, by itself, a tool-level error. A caller that
		// wants to distinguish "normal bounded remainder" from "stuck" reads
		// drain_reason (task 3.1/3.3).
		//
		// DrainDegradedFailure surfaces here (drain_state + error populated)
		// ONLY when Drain returned it WITHOUT a top-level err — the common
		// case (a mid-loop batch step failing) already returns a non-nil err
		// above and takes the toolError branch instead, exactly as before
		// this PR. This success-path branch exists so a caller never has to
		// poll separately to learn about a failure that Drain chose to report
		// alongside a nil error.
		errText := ""
		if outcome.Err != nil {
			errText = outcome.Err.Error()
		}
		return toolJSON(map[string]any{
			"pushed":            result.Pushed,
			"pulled":            result.Pulled,
			"conflicts":         result.Conflicts,
			"project":           result.Project,
			"status":            "ok",
			"config_source":     syncStatus.Source,
			"auto_sync":         syncStatus.AutoSync,
			"config_warnings":   syncStatus.Warnings,
			"drain_state":       drainStateJSON(outcome.State),
			"drain_reason":      string(outcome.Reason),
			"batches_done":      outcome.BatchesDone,
			"batches_remaining": outcome.BatchesRemaining,
			"remaining_push":    outcome.RemainingPush,
			"error":             errText,
		})
	}
}

// drainStateJSON maps a hivesync.DrainState to the mem_sync/health wire
// vocabulary (PR 3, tasks 3.2/3.3). Kept as a single source of truth so both
// surfaces stay in sync.
func drainStateJSON(state hivesync.DrainState) string {
	switch state {
	case hivesync.DrainFullySynced:
		return "fully_synced"
	case hivesync.DrainExpectedPending:
		return "expected_pending"
	case hivesync.DrainDegradedFailure:
		return "degraded_failure"
	default:
		return "unknown"
	}
}

func syncNotConfiguredMessage(goos string) string {
	if goos == "windows" {
		return "sync not configured — set HIVE_API_URL, HIVE_API_EMAIL, HIVE_API_PASSWORD env vars or create ~/.jarvis/sync.json. On Windows, secure the config file with your user account permissions."
	}

	return "sync not configured — set HIVE_API_URL, HIVE_API_EMAIL, HIVE_API_PASSWORD env vars or create ~/.jarvis/sync.json (chmod 600)"
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

func shouldCapturePrompt(value *bool) bool {
	return value == nil || *value
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
