#!/bin/bash
# Jarvis — project skill registry refresh hook for Claude Code.
# Non-fatal and quiet: refreshes only the active project/worktree cwd.

JARVIS_EXECUTABLE={{JARVIS_EXECUTABLE}}
PATH="/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
export PATH
INPUT=$(cat 2>/dev/null || true)

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
    FROM_INPUT=$(printf '%s' "$INPUT" | jq -r '.directory // .cwd // .worktree // .workspace.directory // empty' 2>/dev/null)
    if [ -n "$FROM_INPUT" ] && [ "$FROM_INPUT" != "null" ]; then
      printf '%s' "$FROM_INPUT"
      return
    fi
  else
    for key in directory cwd worktree; do
      FROM_INPUT=$(json_string_value "$key")
      if [ -n "$FROM_INPUT" ]; then
        printf '%s' "$FROM_INPUT"
        return
      fi
    done
  fi
  printf '%s' "${PWD:-}"
}

warn() {
  MESSAGE=$1
  if [ -n "$MESSAGE" ]; then
    printf 'Project skill registry warning: %s\n' "$MESSAGE" >&2
  fi
}

DIRECTORY=$(resolve_directory)
if [ -z "$DIRECTORY" ]; then
  warn "active project directory unavailable"
  exit 0
fi
if [ ! -x "$JARVIS_EXECUTABLE" ]; then
  warn "jarvis executable unavailable"
  exit 0
fi

ERR_FILE=$(mktemp 2>/dev/null || printf '')
cleanup() {
  if [ -n "$ERR_FILE" ]; then
    rm -f "$ERR_FILE" 2>/dev/null || true
  fi
}
trap cleanup EXIT

if command -v timeout >/dev/null 2>&1; then
  timeout {{JARVIS_REFRESH_TIMEOUT_SECONDS}}s "$JARVIS_EXECUTABLE" skill-registry refresh --quiet --cwd "$DIRECTORY" 2>"$ERR_FILE" >/dev/null || warn "refresh failed for $DIRECTORY"
else
  "$JARVIS_EXECUTABLE" skill-registry refresh --quiet --cwd "$DIRECTORY" 2>"$ERR_FILE" >/dev/null || warn "refresh failed for $DIRECTORY"
fi
if [ -s "$ERR_FILE" ]; then
  while IFS= read -r line; do
    warn "$line"
  done < "$ERR_FILE"
fi
exit 0
