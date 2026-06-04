#!/bin/bash
# Hive — UserPromptSubmit hook for Claude Code.
# Reads {"prompt": "..."} from stdin and POSTs it to the local hive-daemon HTTP server.
# Always exits 0 and emits {} — never blocks Claude Code.

HIVE_HTTP_PORT="${HIVE_HTTP_PORT:-7438}"

INPUT=$(cat)
PROMPT=$(printf '%s' "$INPUT" | jq -r '.prompt // empty' 2>/dev/null)

if [ -n "$PROMPT" ]; then
  BODY=$(jq -n --arg c "$PROMPT" '{content: $c}' 2>/dev/null)
  if [ -n "$BODY" ]; then
    curl -s --max-time 1 \
      -X POST "http://127.0.0.1:${HIVE_HTTP_PORT}/prompts" \
      -H 'Content-Type: application/json' \
      -d "$BODY" >/dev/null 2>&1 &
  fi
fi

STATE_FILE="$(dirname "$0")/.first-prompt-done"
if [ ! -f "$STATE_FILE" ]; then
    date -u +%Y-%m-%dT%H:%M:%SZ > "$STATE_FILE" 2>/dev/null || true
    jq -n '{"systemMessage": "Memory protocol is active. FIRST ACTION: call mem_context to load session memory before responding to the user."}'
else
    printf '{}\n'
fi
exit 0
