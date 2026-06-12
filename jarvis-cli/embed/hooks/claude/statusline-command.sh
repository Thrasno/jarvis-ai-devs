#!/bin/bash

PRIMARY='\033[38;5;110m'
ACCENT='\033[38;5;179m'
SECONDARY='\033[38;5;146m'
MUTED='\033[38;5;242m'
SUCCESS='\033[38;5;150m'
ERROR='\033[38;5;174m'
PURPLE='\033[38;5;183m'
BOLD='\033[1m'
NC='\033[0m'

input=$(cat)

MODEL=$(echo "$input" | jq -r '.model.display_name // "Claude"')
DIR=$(echo "$input" | jq -r '.workspace.current_dir // "~"')
ADDED=$(echo "$input" | jq -r '.cost.total_lines_added // 0')
REMOVED=$(echo "$input" | jq -r '.cost.total_lines_removed // 0')

CTX_SIZE=$(echo "$input" | jq -r '.context_window.context_window_size // 200000')
INPUT_TOKENS=$(echo "$input" | jq -r '.context_window.current_usage.input_tokens // 0')
CACHE_CREATE=$(echo "$input" | jq -r '.context_window.current_usage.cache_creation_input_tokens // 0')
CACHE_READ=$(echo "$input" | jq -r '.context_window.current_usage.cache_read_input_tokens // 0')

TOTAL_USED=$((INPUT_TOKENS + CACHE_CREATE + CACHE_READ))
if [ "$CTX_SIZE" -gt 0 ] 2>/dev/null; then
  CTX_PERCENT=$((TOTAL_USED * 100 / CTX_SIZE))
else
  CTX_PERCENT=0
fi
[ "$CTX_PERCENT" -gt 100 ] && CTX_PERCENT=100
[ "$CTX_PERCENT" -lt 0 ] && CTX_PERCENT=0

DIR_NAME=$(basename "$DIR")

BRANCH=""
GIT_DIRTY=""
if git rev-parse --git-dir > /dev/null 2>&1; then
  BRANCH=$(git branch --show-current 2>/dev/null)
  if [[ -n $(git status --porcelain 2>/dev/null) ]]; then
    GIT_DIRTY="*"
  fi
fi

MODEL_ICON="🤖"
case "$MODEL" in
  *Opus*) MODEL_ICON="🎭" ;;
  *Sonnet*) MODEL_ICON="📝" ;;
  *Haiku*) MODEL_ICON="🍃" ;;
esac

BAR_WIDTH=8
FILLED=$((CTX_PERCENT * BAR_WIDTH / 100))
EMPTY=$((BAR_WIDTH - FILLED))

if [ "$CTX_PERCENT" -ge 80 ]; then
  BAR_COLOR="$ERROR"
elif [ "$CTX_PERCENT" -ge 50 ]; then
  BAR_COLOR="$ACCENT"
else
  BAR_COLOR="$SUCCESS"
fi

BAR="${BAR_COLOR}["
for ((i=0; i<FILLED; i++)); do BAR+="="; done
for ((i=0; i<EMPTY; i++)); do BAR+="."; done
BAR+="]${NC}"

SEP="${MUTED}  ${NC}"

LINE="${BOLD}${PURPLE}${MODEL_ICON} ${MODEL}${NC}"
LINE+="${SEP}"
LINE+="${ACCENT}󰉋 ${DIR_NAME}${NC}"

if [ -n "$BRANCH" ]; then
  LINE+="${SEP}"
  LINE+="${SECONDARY} ${BRANCH}${GIT_DIRTY}${NC}"
fi

LINE+="${SEP}"
LINE+="${SUCCESS}+${ADDED}${NC} ${ERROR}-${REMOVED}${NC}"

LINE+="${SEP}"
LINE+="${MUTED}ctx${NC} ${BAR} ${MUTED}${CTX_PERCENT}%${NC}"

echo -e "${LINE}\033[K"
