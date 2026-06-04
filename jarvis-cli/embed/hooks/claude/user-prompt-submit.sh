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

# First-prompt injection design:
# When SessionStart hook is installed: session-start.{ps1,sh} pre-creates .first-prompt-done
# before any user prompt, so this branch is never reached — sessionStart is the primary
# injection path.
# When SessionStart is NOT installed (e.g. older Claude Code builds): this branch fires on
# the first prompt and acts as the fallback injection mechanism.
STATE_FILE="$(dirname "$0")/.first-prompt-done"
if ( set -C; date -u +%Y-%m-%dT%H:%M:%SZ > "$STATE_FILE" ) 2>/dev/null; then
    jq -n '{"systemMessage": "Memory protocol is active. FIRST ACTION: call mem_context to load session memory before responding to the user."}' 2>/dev/null || printf '{"systemMessage":"Memory protocol is active. FIRST ACTION: call mem_context to load session memory before responding to the user."}\n'
else
    printf '{}\n'
fi
exit 0
