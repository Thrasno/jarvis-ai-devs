#!/bin/bash
# Hive — Stop hook for Claude Code.
# Cleans up session state so the next session gets fresh memory injection.

STATE_FILE="$(dirname "$0")/.first-prompt-done"
rm -f "$STATE_FILE" 2>/dev/null || true
printf '{}\n'
exit 0
