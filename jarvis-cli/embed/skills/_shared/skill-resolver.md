# Skill Resolver Protocol — Jarvis Version

## Purpose

The orchestrator uses this protocol to find project-specific coding standards and inject them into sub-agent prompts BEFORE task-specific instructions. Sub-agents receive rules pre-digested — they do NOT read SKILL.md files or the registry themselves.

## Step 1 — Find Registry

First, search Hive for the registry:
1. Call `mcp__hive__mem_search(query: "skill-registry", project: "{project}")`.
2. Call `mcp__hive__mem_get_observation(id)` to get the full registry content.

If Hive is unavailable or the registry is not found in Hive:
- Fallback: read `.jarvis/skill-registry.md` from the project root.

If neither source is available:
- Warn the user: "No skill registry found. Sub-agents will not receive project-specific standards."
- Proceed without injecting standards.

## Step 2 — Match Skills

From the registry's Compact Rules section, match relevant rules by:

- **Code context**: file extensions or paths the sub-agent will touch (e.g., `.go`, `internal/agent/`, `.md`)
- **Task context**: what actions it will perform (e.g., "review", "PR creation", "Go testing", "writing specs")

Select all matching compact rule blocks.

## Step 3 — Inject Compact Rules

Copy matching compact rule blocks into the sub-agent prompt as a `## Project Standards (auto-resolved)` section. Place this section BEFORE the sub-agent's task-specific instructions.

The sub-agent receives rules as text, not file paths. It does NOT read SKILL.md files. It does NOT read the registry. Rules arrive pre-digested. This is compaction-safe because each delegation re-reads the registry if the cache is lost.

## Skill Resolution Feedback

Every sub-agent return value MUST include a `skill_resolution` field:
- `injected` — compact rules were successfully passed in the prompt
- `fallback-registry` — rules came from `.jarvis/skill-registry.md` (Hive was unavailable)
- `fallback-path` — rules came from a direct file path fallback
- `none` — no rules were found or injected

If the orchestrator receives `fallback-*` or `none` in any return value: re-read the registry immediately and inject compact rules in all subsequent delegations. This is a self-correction mechanism — do NOT ignore fallback reports.

## Registry Location

`.jarvis/skill-registry.md` in the project root — NOT `.atl/` or any other path.

The registry is written by `sdd-init` during project initialization. It is infrastructure, not an SDD artifact.
