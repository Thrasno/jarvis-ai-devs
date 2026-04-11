# Hive Memory — Usage Protocol

## When to Save (MANDATORY — do not wait to be asked)

Call `mem_save` immediately after any of these events:
- Architecture or design decision made
- Bug fix completed (include root cause in content)
- Feature implemented with non-obvious approach
- Non-obvious discovery about the codebase
- Pattern established (naming, structure, convention)
- Team convention documented or updated

## mem_save Format

```
title: "Verb + what" (e.g. "Fixed race condition in sync loop")
type: bugfix | decision | architecture | discovery | pattern | config | preference
topic_key: stable-key/like-this  (use for evolving topics — enables upsert)
project: project-name
content: |
  **What**: One sentence.
  **Why**: What motivated it.
  **Where**: Files or paths affected.
  **Learned**: Gotchas, edge cases (omit if none).
```

## When to Search Memory

On any variation of "remember", "recall", "what did we do", "how did we solve":
1. `mem_context` — recent session history (fast, cheap)
2. `mem_search("keywords")` — full-text BM25 search
3. `mem_get_observation(id)` — full untruncated content (REQUIRED after search)

Also search PROACTIVELY when starting work that might have prior history.

## Session End (MANDATORY)

Before saying "done" or ending a session, call `mem_session_summary`:

```markdown
## Goal
[What we worked on]

## Discoveries
- [Technical findings, gotchas]

## Accomplished
- [Completed items with key details]

## Next Steps
- [What remains]

## Relevant Files
- path/to/file — [what changed]
```

## Topic Key Conventions

| Artifact | Key Pattern |
|----------|-------------|
| SDD spec | `sdd/{change}/spec` |
| SDD design | `sdd/{change}/design` |
| SDD tasks | `sdd/{change}/tasks` |
| Architecture | `architecture/{module}` |
| Bug fix | `bugfix/{component}/{issue}` |
| Pattern | `pattern/{name}` |
