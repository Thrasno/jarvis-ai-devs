#!/bin/bash
# Hive — Stop hook for Claude Code.
# Cleans up only the current session marker so other active sessions keep their state.

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
  printf '%s/first-prompt-%s.done' "$STATE_ROOT" "$SAFE_SESSION_ID"
}

STATE_FILE=$(state_file_for_session)
rm -f "$STATE_FILE" 2>/dev/null || true
printf '{}\n'
exit 0
