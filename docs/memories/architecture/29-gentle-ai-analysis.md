---
id: 29
type: architecture
project: jarvis-dev
scope: project
session: manual-save-jarvis-dev
created_at: 2026-04-09 18:52:05
---

# gentle-ai ecosystem architecture analysis

**What**: Analyzed gentle-ai GitHub repository (https://github.com/Gentleman-Programming/gentle-ai) in depth — architecture, SDD workflow, skills system, PRD structure, Engram persistence, multi-agent support

**Why**: To understand the ecosystem as reference for creating jarvis-dev PRD, extract patterns for PRD structure, and learn SDD workflow implementation

**Where**: /tmp/gentle-ai/ (cloned repo for analysis)

**Key Discoveries**:

## Architecture
- **Language**: Go 1.24+, Bubbletea TUI, 18 internal packages
- **Pattern**: Agent adapters (8 agents implement common interface → extensibility without core changes)
- **Testing**: 26 test packages, 260+ test functions, 78 E2E tests (Docker: Ubuntu + Arch)
- **Backup System**: Compressed tar.gz, deduplicated, auto-pruned (keeps 5), pinnable via TUI
- **Distribution**: Homebrew, Scoop, Go install, GitHub Releases

## SDD Workflow (9 phases)
- **Dependency graph**: proposal → specs → tasks → apply → verify → archive; design branches from specs
- **Artifact store modes**: engram (memory, upserts overwrite), openspec (files, git history), hybrid (both, higher token cost), none (ephemeral)
- **Organic invocation**: Orchestrator offers SDD when task warrants it (not manual)
- **Strict TDD**: Auto-detected if test runner exists, cached during sdd-init
- **Per-phase models** (OpenCode only): opus for architectural (propose, design), sonnet for implementation (apply, verify), haiku for archive

## Skills System (16 skills)
- **Structure**: YAML frontmatter + markdown sections (Purpose, Execution Contract, Steps, Rules)
- **Shared resources**: `_shared/` dir with sdd-phase-common.md, engram-convention.md, openspec-convention.md, persistence-contract.md
- **Skill loading priority**: Injected compact rules (preferred) → fallback to registry search → fallback to skill path
- **Return envelope** (required): status, summary, artifacts, next, risks, skill_resolution

## Engram (Persistent Memory)
- **What**: MCP server (localhost:7437), SQLite + FTS5, cross-session memory
- **Topic keys**: Deterministic naming (sdd/{change-name}/{artifact}) enables upserts
- **Recovery protocol** (MANDATORY 2 steps): mem_search (returns 300-char previews) → mem_get_observation (full content)
- **Proactive saves**: Architecture decisions, bug fixes, non-obvious discoveries, patterns established
- **Session close**: mem_session_summary with Goal/Instructions/Discoveries/Accomplished/Next/Files

## Multi-Agent Support (8 agents)
- **Full delegation**: Claude Code (Task tool), OpenCode (multi-mode overlay), Gemini CLI (experimental), Cursor (native subagents), VSCode (runSubagent)
- **Solo-agent**: Codex, Windsurf, Antigravity (phases run inline, Engram provides persistence)
- **Ecosystem tiers**: Full (Claude/OpenCode) → Good (Cursor/VSCode) → Partial (Gemini/Codex/Windsurf) → Minimal (Antigravity)

## PRD Structure Patterns (2,366 lines total)
- **Sections**: Problem → Vision → Users → Platforms → Components → UX → Architecture → Agent-Specific → Success → Non-Goals
- **Tables**: Feature matrices, decision trees, requirement tracking with IDs (R-AGENT-01, R-DEP-02)
- **Mermaid diagrams**: Big Picture, Runtime Interaction, Installation Pipeline, Agent Config Matrix (4 total in main PRD)
- **"Before/After" storytelling**: Communicate value through user journey
- **Platform tables**: macOS/Linux/Windows differences explicitly called out
- **Explicit non-goals**: Set boundaries clearly

## Key Components
- **GGA** (Gentleman Guardian Angel): AI-powered code review as pre-commit hook, pure Bash, zero deps, smart SHA256 cache
- **Persona System**: Gentleman (Senior Architect mentor, Rioplatense Spanish, teaches concepts) | Neutral | Custom
- **MCP Servers**: Context7 (auto-configured), Notion/Jira (user configures tokens)

**Learned**: 
1. gentle-ai is 8-agent configurator (enterprise scale) — jarvis-dev should be simpler (personal env)
2. Use PRD structure/format, NOT scale (cap at ~800 lines vs 1,400+)
3. Borrow patterns (agent adapters, backup system, TUI) but simplify for single-user
4. SDD workflow is organic (not manual commands) — key for UX
5. Two-step recovery pattern (search → get_observation) is critical for Engram reliability
6. Skill loading has 3 fallback levels (injected → registry → path) for compaction survival
