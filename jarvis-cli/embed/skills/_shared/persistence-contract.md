# Persistence Contract — 4-Mode Backend Contract

## The Four Modes

| Mode | When to use | Backend |
|------|-------------|---------|
| `hive` | Default — always unless overridden | `mcp__hive__*` MCP tool calls |
| `openspec` | User says "use openspec", "save to files", or "file-based" | Files under `openspec/changes/{change-name}/` |
| `hybrid` | User says "both" or "hybrid" | Both Hive AND openspec |
| `none` | User says "inline only", "no persistence", or "don't save" | Return results only — call no persistence tools |

## Canonical Step 0 Template

Every SDD skill MUST begin with this exact Step 0 section. Copy it verbatim:

```markdown
## Step 0 — Resolve Persistence Mode

1. **Default**: Hive (`mcp__hive__*` tools)
2. **Override**: openspec or hybrid — if user explicitly requests it
3. **Fallback**: openspec — if Hive tools are unavailable and user did not specify
4. **None**: only if user explicitly requests it

Carry this decision through all steps. Do not re-evaluate mid-skill.
```

## Mode Behavior Rules

### Hive mode
- Use `mcp__hive__mem_save` for all artifact saves
- Use `mcp__hive__mem_search` + `mcp__hive__mem_get_observation` for all reads
- Do NOT create any `openspec/` directories or files
- `project` field is ALWAYS required in every save call

### openspec mode
- Write artifact files under `openspec/changes/{change-name}/`
- Do NOT call any `mcp__hive__*` tools for artifact saves (only for context reads if needed)
- Read existing specs from `openspec/specs/`

### hybrid mode
- Perform both: save to Hive AND write to filesystem
- Retrieve dependencies from Hive (primary) with filesystem fallback
- Both backends stay in sync

### none mode
- Call NO persistence tools at all
- Return results inline in the response
- Never create or modify any project files
- Explicitly state in the return that no artifacts were saved

## Fallback Behavior

If Hive tools return an error or are unavailable AND the user has not specified a mode:
1. Log a warning: "Hive unavailable, falling back to openspec"
2. Switch to openspec mode for the remainder of the skill
3. Report `status: complete` but include a note about the fallback in `risks`
