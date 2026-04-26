package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
	hivesync "github.com/Thrasno/jarvis-dev/hive-daemon/internal/sync"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MaxObservationLength is the maximum allowed content size in runes (not bytes).
// Unicode-safe: a Japanese character counts as 1 rune even though it is 3 bytes.
const MaxObservationLength = 50_000

func registerTools(s *sdkmcp.Server, store MemoryStore, syncStore hivesync.SyncStore, syncer SyncRunner, cfg *hivesync.Config, activity *ActivityTracker) {
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
				"files_affected":{"type": "array", "items": {"type": "string"}}
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
				"content": {"type": "string", "description": "Session summary in markdown"},
				"project": {"type": "string", "description": "Project identifier"}
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
	}, memContextHandler(store, activity))

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
}

// ─── Handlers ──────────────────────────────────────────────────────────────

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
		}
		if err := json.Unmarshal(req.Params.Arguments, &p); err != nil {
			return toolError(fmt.Errorf("invalid params: %w", err)), nil
		}
		if p.Title == "" || p.Content == "" || p.Project == "" {
			return toolError(fmt.Errorf("title, content, and project are required")), nil
		}

		// Guard: reject content exceeding MaxObservationLength runes (Unicode-safe).
		if runeCount := utf8.RuneCountInString(p.Content); runeCount > MaxObservationLength {
			return toolError(fmt.Errorf(
				"content too long: %d runes (max %d). Summarize or split into multiple observations",
				runeCount, MaxObservationLength,
			)), nil
		}

		mem := &models.Memory{
			Title:         p.Title,
			Content:       p.Content,
			Category:      p.Type,
			Project:       p.Project,
			TopicKey:      p.TopicKey,
			Tags:          p.Tags,
			FilesAffected: p.FilesAffected,
		}

		id, err := store.SaveMemory(mem)
		if err != nil {
			return toolError(fmt.Errorf("save failed: %w", err)), nil
		}

		activity.RecordSave(p.Project)

		// Auto-sync: spawn background goroutine if enabled
		if cfg != nil && cfg.AutoSync && syncer != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_, _ = syncer.Sync(ctx, p.Project) // fire-and-forget
			}()
		}

		return toolJSON(map[string]any{"id": id, "status": "saved"})
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
			Content string `json:"content"`
			Project string `json:"project"`
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

		// Guard: same 50K rune limit as memSaveHandler.
		if runeCount := utf8.RuneCountInString(p.Content); runeCount > MaxObservationLength {
			return toolError(fmt.Errorf(
				"content too long: %d runes (max %d). Summarize or split into multiple observations",
				runeCount, MaxObservationLength,
			)), nil
		}

		mem := &models.Memory{
			Title:    titleFromContent(p.Content),
			Content:  p.Content,
			Category: "session_summary",
			Project:  p.Project,
		}

		id, err := store.SaveMemory(mem)
		if err != nil {
			return toolError(fmt.Errorf("save failed: %w", err)), nil
		}

		activity.RecordSave(p.Project)

		responseText := fmt.Sprintf(`{"id":%d,"status":"saved"}`, id)
		responseText += activity.SessionStats(p.Project)

		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: responseText}},
		}, nil
	}
}

func memContextHandler(store MemoryStore, activity *ActivityTracker) sdkmcp.ToolHandler {
	return func(_ context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
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

		formatted := formatContext(results, p.Project)
		formatted += activity.NudgeIfNeeded(p.Project)

		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: formatted}},
		}, nil
	}
}

// ─── Formatters ────────────────────────────────────────────────────────────

// formatContext renders memories as compact markdown with truncated previews.
// Returns a footer with count and hint to use mem_get_observation.
func formatContext(memories []*models.Memory, project string) string {
	if len(memories) == 0 {
		return fmt.Sprintf("No memories found for project %q.", project)
	}

	var b strings.Builder
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
