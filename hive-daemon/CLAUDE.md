# hive-daemon — Project Instructions

## Memory System

This project uses **Hive** (hive-daemon MCP) as its persistent memory system, NOT Engram.

All `mem_save`, `mem_search`, `mem_context`, `mem_get_observation`, and `mem_session_summary`
calls MUST use the tools provided by the `hive` MCP server.

**project identifier for all saves:** `hive-daemon`

The global Engram protocol in `~/.claude/CLAUDE.md` is OVERRIDDEN here — do not call Engram tools
for this project. Engram remains available for other projects.

## Revert

To disable Hive and return to Engram: remove `~/.claude/mcp/hive.json`.
All Engram data is untouched in its original location.

## Development Rules

- NEVER build after changes (use existing test commands instead)
- Run tests: `go test ./...` from `hive-daemon/`
- Lint: `golangci-lint run ./...` from `hive-daemon/`
- Installer-managed binary path: `/usr/local/bin/hive-daemon` (Unix) or `%LOCALAPPDATA%\Programs\jarvis\hive-daemon.exe` (Windows)
- jarvis-cli runtime resolution order: installer-managed path → `PATH` → legacy `$GOPATH/bin` / `~/go/bin`
- DB in production: `~/.jarvis/memory.db`
- DB in tests: `HIVE_DB_PATH` env var (temp dir per test)

## Tools Available (Hive MCP)

| Tool | Description |
|------|-------------|
| `mem_save` | Save memory — title, content, type, project, topic_key (for upsert) |
| `mem_search` | FTS5 full-text search with BM25 ranking |
| `mem_context` | Recent memories ordered by recency |
| `mem_get_observation` | Get full content by ID |
| `mem_session_summary` | Save session summary (title auto-extracted) |
