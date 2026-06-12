#!/bin/bash
# Hive — UserPromptSubmit hook for Claude Code.
# Reads {"prompt": "..."} from stdin and POSTs it to the local hive-daemon HTTP server.
# Always exits 0 and emits {} — never blocks Claude Code.

HIVE_HTTP_PORT="${HIVE_HTTP_PORT:-7438}"

INPUT=$(cat)

has_jq() {
  command -v jq >/dev/null 2>&1
}

json_unescape() {
  VALUE=$1
  OUT=""
  while [ -n "$VALUE" ]; do
    CHAR=${VALUE:0:1}
    VALUE=${VALUE:1}
    if [ "$CHAR" != "\\" ]; then
      OUT="$OUT$CHAR"
      continue
    fi

    if [ -z "$VALUE" ]; then
      OUT="$OUT\\"
      break
    fi

    ESC=${VALUE:0:1}
    VALUE=${VALUE:1}
    case "$ESC" in
      '"') OUT="$OUT\"" ;;
      "\\") OUT="$OUT\\" ;;
      '/') OUT="$OUT/" ;;
      'n') OUT="$OUT"$'\n' ;;
      'r') OUT="$OUT"$'\r' ;;
      't') OUT="$OUT"$'\t' ;;
      *) OUT="$OUT\\$ESC" ;;
    esac
  done
  printf '%s' "$OUT"
}

json_string_value() {
  KEY=$1
  REGEX='"'"$KEY"'"[[:space:]]*:[[:space:]]*"(([^"\\]|\\.)*)"'
  if [[ $INPUT =~ $REGEX ]]; then
    json_unescape "${BASH_REMATCH[1]}"
  fi
}

json_escape() {
  VALUE=$1
  VALUE=${VALUE//\\/\\\\}
  VALUE=${VALUE//\"/\\\"}
  VALUE=${VALUE//$'\n'/\\n}
  VALUE=${VALUE//$'\r'/\\r}
  VALUE=${VALUE//$'\t'/\\t}
  printf '%s' "$VALUE"
}

json_prompt() {
  if has_jq; then
    printf '%s' "$INPUT" | jq -r '.prompt // empty' 2>/dev/null
    return
  fi
  json_string_value "prompt"
}

PROMPT=$(json_prompt)

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
  if has_jq; then
    SESSION_FROM_INPUT=$(printf '%s' "$INPUT" | jq -r '.session_id // .sessionId // .session.id // .transcript_path // .transcriptPath // empty' 2>/dev/null)
    if [ -n "$SESSION_FROM_INPUT" ] && [ "$SESSION_FROM_INPUT" != "null" ]; then
      printf '%s' "$SESSION_FROM_INPUT"
      return
    fi
  else
    for key in session_id sessionId transcript_path transcriptPath; do
      SESSION_FROM_INPUT=$(json_string_value "$key")
      if [ -n "$SESSION_FROM_INPUT" ]; then
        printf '%s' "$SESSION_FROM_INPUT"
        return
      fi
    done
  fi
  printf 'ppid-%s' "${PPID:-$$}"
}

resolve_project() {
  if [ -n "${HIVE_PROJECT:-}" ]; then
    printf '%s' "$HIVE_PROJECT"
    return
  fi
  if [ -n "${JARVIS_PROJECT:-}" ]; then
    printf '%s' "$JARVIS_PROJECT"
    return
  fi
  if has_jq; then
    PROJECT_FROM_INPUT=$(printf '%s' "$INPUT" | jq -r '.project // .projectName // empty' 2>/dev/null)
    if [ -n "$PROJECT_FROM_INPUT" ] && [ "$PROJECT_FROM_INPUT" != "null" ]; then
      printf '%s' "$PROJECT_FROM_INPUT"
      return
    fi
  else
    for key in project projectName; do
      PROJECT_FROM_INPUT=$(json_string_value "$key")
      if [ -n "$PROJECT_FROM_INPUT" ]; then
        printf '%s' "$PROJECT_FROM_INPUT"
        return
      fi
    done
  fi
}

resolve_directory() {
  if [ -n "${HIVE_PROJECT_DIRECTORY:-}" ]; then
    printf '%s' "$HIVE_PROJECT_DIRECTORY"
    return
  fi
  if [ -n "${JARVIS_WORKSPACE_DIRECTORY:-}" ]; then
    printf '%s' "$JARVIS_WORKSPACE_DIRECTORY"
    return
  fi
  if has_jq; then
    DIRECTORY_FROM_INPUT=$(printf '%s' "$INPUT" | jq -r '.directory // .cwd // .workspace.directory // empty' 2>/dev/null)
    if [ -n "$DIRECTORY_FROM_INPUT" ] && [ "$DIRECTORY_FROM_INPUT" != "null" ]; then
      printf '%s' "$DIRECTORY_FROM_INPUT"
      return
    fi
  else
    for key in directory cwd; do
      DIRECTORY_FROM_INPUT=$(json_string_value "$key")
      if [ -n "$DIRECTORY_FROM_INPUT" ]; then
        printf '%s' "$DIRECTORY_FROM_INPUT"
        return
      fi
    done
  fi
  printf '%s' "${PWD:-}"
}

prompt_body() {
  if has_jq; then
    jq -n --arg c "$PROMPT" --arg s "$SESSION_ID_VALUE" --arg d "$DIRECTORY_VALUE" --arg p "$PROJECT_VALUE" '{content: $c, session_id: $s, directory: $d} + (if $p != "" then {project: $p} else {} end)' 2>/dev/null
    return
  fi

  ESCAPED_PROMPT=$(json_escape "$PROMPT")
  ESCAPED_SESSION=$(json_escape "$SESSION_ID_VALUE")
  ESCAPED_DIRECTORY=$(json_escape "$DIRECTORY_VALUE")
  if [ -n "$PROJECT_VALUE" ]; then
    ESCAPED_PROJECT=$(json_escape "$PROJECT_VALUE")
    printf '{"content":"%s","session_id":"%s","directory":"%s","project":"%s"}\n' "$ESCAPED_PROMPT" "$ESCAPED_SESSION" "$ESCAPED_DIRECTORY" "$ESCAPED_PROJECT"
    return
  fi
  printf '{"content":"%s","session_id":"%s","directory":"%s"}\n' "$ESCAPED_PROMPT" "$ESCAPED_SESSION" "$ESCAPED_DIRECTORY"
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
  SESSION_ID_VALUE=$(resolve_session_id)
  DIRECTORY_VALUE=$(resolve_directory)
  PROJECT_VALUE=$(resolve_project)
  BODY=$(prompt_body)
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
