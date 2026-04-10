package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

func registerTools(s *sdkmcp.Server, store MemoryStore) {
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
	}, memSaveHandler(store))

	s.AddTool(&sdkmcp.Tool{
		Name:        "mem_search",
		Description: "Search memories using full-text search with BM25 ranking",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"required": ["query"],
			"properties": {
				"query":   {"type": "string", "description": "Search terms"},
				"project": {"type": "string", "description": "Filter by project (omit for all projects)"},
				"limit":   {"type": "integer", "description": "Max results (default 10, max 50)"}
			}
		}`),
	}, memSearchHandler(store))

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
	}, memGetObservationHandler(store))

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
	}, memSessionSummaryHandler(store))

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
	}, memContextHandler(store))
}

// ─── Handlers ──────────────────────────────────────────────────────────────

func memSaveHandler(store MemoryStore) sdkmcp.ToolHandler {
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

		return toolJSON(map[string]any{"id": id, "status": "saved"})
	}
}

func memSearchHandler(store MemoryStore) sdkmcp.ToolHandler {
	return func(_ context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		var p struct {
			Query   string `json:"query"`
			Project string `json:"project"`
			Limit   int    `json:"limit"`
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

		results, err := store.Search(p.Query, p.Project, p.Limit)
		if err != nil {
			return toolError(fmt.Errorf("search failed: %w", err)), nil
		}
		if results == nil {
			results = []*models.Memory{}
		}
		return toolJSON(results)
	}
}

func memGetObservationHandler(store MemoryStore) sdkmcp.ToolHandler {
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
		return toolJSON(mem)
	}
}

func memSessionSummaryHandler(store MemoryStore) sdkmcp.ToolHandler {
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
		return toolJSON(map[string]any{"id": id, "status": "saved"})
	}
}

func memContextHandler(store MemoryStore) sdkmcp.ToolHandler {
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

		results, err := store.ListMemories(p.Project, p.Limit)
		if err != nil {
			return toolError(fmt.Errorf("list failed: %w", err)), nil
		}
		if results == nil {
			results = []*models.Memory{}
		}
		return toolJSON(results)
	}
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
