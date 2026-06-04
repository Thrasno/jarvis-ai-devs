#!/bin/bash
# Hive — UserPromptSubmit hook for Claude Code.
# Reads {"prompt": "..."} from stdin and POSTs it to the local hive-daemon HTTP server.
# Always exits 0 and emits {} — never blocks Claude Code.

HIVE_HTTP_PORT="${HIVE_HTTP_PORT:-7438}"

INPUT=$(cat)
PROMPT=$(printf '%s' "$INPUT" | jq -r '.prompt // empty' 2>/dev/null)

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

if [ -n "$PROMPT" ]; then
  BODY=$(jq -n --arg c "$PROMPT" '{content: $c}' 2>/dev/null)
  if [ -n "$BODY" ]; then
    curl -s --max-time 1 \
      -X POST "http://127.0.0.1:${HIVE_HTTP_PORT}/prompts" \
      -H 'Content-Type: application/json' \
      -d "$BODY" >/dev/null 2>&1 &
  fi
fi

# First-prompt injection design:
# When SessionStart is installed, session-start.{ps1,sh} creates this session-scoped
# marker in temp storage before any user prompt, so this branch is skipped. When
# SessionStart is unavailable, this branch creates the marker for the current session
# and acts as the fallback injection mechanism.
STATE_FILE=$(state_file_for_session)
if ( set -C; date -u +%Y-%m-%dT%H:%M:%SZ > "$STATE_FILE" ) 2>/dev/null; then
  jq -n '{"systemMessage": "Memory protocol is active. FIRST ACTION: call mem_context to load session memory before responding to the user."}' 2>/dev/null || printf '{"systemMessage":"Memory protocol is active. FIRST ACTION: call mem_context to load session memory before responding to the user."}\n'
else
  printf '{}\n'
fi
exit 0
