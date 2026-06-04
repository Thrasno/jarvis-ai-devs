#!/bin/bash
# Hive — SessionStart hook for Claude Code.
# Injects memory protocol instructions as additionalContext before the first prompt.
# Always exits 0 and emits valid JSON — never blocks Claude Code.

# Primary injection path: this hook fires at session start, records a session-scoped
# first-prompt marker in temp storage, and injects additionalContext before the first
# user prompt. The first-prompt branch in user-prompt-submit.{ps1,sh} is the fallback
# when this hook is not installed (e.g. older Claude Code builds without SessionStart).

INPUT=$(cat)

resolve_session_id() {
  if [ -n "${HIVE_CLAUDE_SESSION_ID:-}" ]; then
    printf '%s' "$HIVE_CLAUDE_SESSION_ID"
    return
  fi
  if [ -n "${CLAUDE_SESSION_ID:-}" ]; then
    printf '%s' "$CLAUDE_SESSION_ID"
    return
  fi
  if [ -n "${SESSION_ID:-}" ]; then
    printf '%s' "$SESSION_ID"
    return
  fi
  if command -v jq >/dev/null 2>&1; then
    SESSION_FROM_INPUT=$(printf '%s' "$INPUT" | jq -r '.session_id // .sessionId // .session.id // .transcript_path // .transcriptPath // empty' 2>/dev/null)
    if [ -n "$SESSION_FROM_INPUT" ] && [ "$SESSION_FROM_INPUT" != "null" ]; then
      printf '%s' "$SESSION_FROM_INPUT"
      return
    fi
  fi
  printf 'ppid-%s' "${PPID:-$$}"
}

state_file_for_session() {
  SESSION_ID_VALUE=$(resolve_session_id)
  SAFE_SESSION_ID=$(printf '%s' "$SESSION_ID_VALUE" | tr -c 'A-Za-z0-9_.-' '_' | cut -c 1-160)
  if [ -z "$SAFE_SESSION_ID" ]; then
    SAFE_SESSION_ID="unknown"
  fi
  STATE_ROOT="${XDG_RUNTIME_DIR:-${TMPDIR:-${TEMP:-${TMP:-/tmp}}}}/jarvis-hive/claude-hooks"
  mkdir -p "$STATE_ROOT" 2>/dev/null || true
  printf '%s/first-prompt-%s.done' "$STATE_ROOT" "$SAFE_SESSION_ID"
}

STATE_FILE=$(state_file_for_session)
date -u +%Y-%m-%dT%H:%M:%SZ > "$STATE_FILE" 2>/dev/null || true

CONTEXT="## Hive Memory Protocol — ACTIVE

MANDATORY FIRST ACTION: call mem_context to recover memory from previous sessions before responding to the user.

If mem_context is not available, call ToolSearch with query \"mem_context\" to load it first.

Do not respond to the user until memory context has been loaded."

jq -n --arg ctx "$CONTEXT" '{"additionalContext": $ctx}' 2>/dev/null || \
  printf '{"additionalContext":"## Hive Memory Protocol — ACTIVE\\n\\nMANDATORY FIRST ACTION: call mem_context to recover memory from previous sessions before responding to the user.\\n\\nIf mem_context is not available, call ToolSearch with query \\"mem_context\\" to load it first.\\n\\nDo not respond to the user until memory context has been loaded."}\n'
exit 0
