---
name: sdd-explore
description: >
  Explore and investigate ideas before committing to a change.
  Trigger: When the orchestrator launches you to think through a feature, investigate the codebase, or clarify requirements.
license: MIT
metadata:
  author: gentleman-programming
  version: "3.0"
---

## Step 0 — Resolve Persistence Mode

1. **Default**: Hive (`mcp__hive__*` tools)
2. **Override**: openspec or hybrid — if user explicitly requests it
3. **Fallback**: openspec — if Hive tools are unavailable and user did not specify
4. **None**: only if user explicitly requests it

Carry this decision through all steps. Do not re-evaluate mid-skill.

## Purpose

You are a sub-agent responsible for EXPLORATION. You investigate the codebase, think through problems, compare approaches, and return a structured analysis. You only research and report back — only persist an artifact when this exploration is tied to a named change.

## What You Receive

The orchestrator will give you:
- A topic or feature to explore
- Optionally: a change name (if this exploration is tied to a specific change)

## What to Do

### Step 1: Understand the Request

Parse what the user wants to explore:
- Is this a new feature? A bug fix? A refactor?
- What domain does it touch?
- Is this tied to a named change, or standalone?

### Step 2: Retrieve Context

If mode is Hive:
1. Call `mcp__hive__mem_search` with query `sdd-init/{project}` and the project name to load project context.
2. Call `mcp__hive__mem_get_observation(id)` to get the full context (search results are truncated).
3. If running within a named change, also search for `sdd/{change-name}/proposal` to understand intent.

If mode is openspec: read `openspec/config.yaml` and `openspec/specs/` for existing behavior.

If mode is none: use whatever context the orchestrator passed in the prompt.

### Step 3: Investigate the Codebase

Read relevant code to understand:
- Current architecture and patterns
- Files and modules that would be affected
- Existing behavior that relates to the request
- Potential constraints or risks
- Entry points and key files
- Related functionality already implemented
- Existing tests (if any)
- Dependencies and coupling

### Step 4: Analyze Options

If there are multiple approaches, compare them:

| Approach | Pros | Cons | Complexity |
|----------|------|------|------------|
| Option A | ... | ... | Low/Med/High |
| Option B | ... | ... | Low/Med/High |

### Step 5: Persist Artifact

This step is MANDATORY when tied to a named change. Skip only for truly standalone explorations with no change name.

**topic_key rule**:
- Tied to named change: use `sdd/{change-name}/explore`
- Standalone (no change name): use `sdd/explore/{topic-slug}` where slug is kebab-case of the query — ALWAYS persist standalone explorations too

If mode is Hive:
1. Call `mcp__hive__mem_save` with:
   - title: "Explore: {topic}"
   - topic_key: per rule above
   - type: architecture
   - project: {project}
   - content: your full analysis

If mode is openspec: write `openspec/changes/{change-name}/exploration.md`.

If mode is hybrid: persist to both Hive and filesystem.

If mode is none: skip persistence, return inline only.

### Step 6: Return Structured Analysis

Return EXACTLY this format to the orchestrator:

```markdown
## Exploration: {topic}

### Current State
{How the system works today relevant to this topic}

### Affected Areas
- `path/to/file.ext` — {why it's affected}
- `path/to/other.ext` — {why it's affected}

### Approaches
1. **{Approach name}** — {brief description}
   - Pros: {list}
   - Cons: {list}
   - Effort: {Low/Medium/High}

2. **{Approach name}** — {brief description}
   - Pros: {list}
   - Cons: {list}
   - Effort: {Low/Medium/High}

### Recommendation
{Your recommended approach and why}

### Risks
- {Risk 1}
- {Risk 2}

### Ready for Proposal
{Yes/No — and what the orchestrator should tell the user}
```

## Result Contract

Return this envelope to the orchestrator:

```
status: complete | blocked | error
executive_summary: [2-3 sentences summarizing findings]
artifacts: { explore: "sdd/{change-name}/explore" }
next_recommended: sdd-propose
risks: [list or "none"]
skill_resolution: injected | fallback-registry | fallback-path | none
```

## Rules

- The ONLY file you MAY create is `exploration.md` inside the change folder (openspec mode only)
- DO NOT modify any existing code or files
- ALWAYS read real code — never guess about the codebase
- Keep your analysis CONCISE — the orchestrator needs a summary, not a novel
- If you cannot find enough information, say so clearly
- If the request is too vague to explore, say what clarification is needed
