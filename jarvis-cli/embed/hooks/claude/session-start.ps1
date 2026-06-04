# Hive — SessionStart hook for Claude Code on Windows.
# Injects memory protocol instructions as additionalContext before the first prompt.
# Always exits 0 and emits valid JSON — never blocks Claude Code.

# Primary injection path: this hook fires unconditionally at session start, pre-creates
# .first-prompt-done, and injects additionalContext before the first user prompt.
# The first-prompt branch in user-prompt-submit.{ps1,sh} acts as a fallback when this
# hook is NOT installed (e.g. older Claude Code builds without SessionStart support).

$ErrorActionPreference = 'SilentlyContinue'

$stateFile = Join-Path $PSScriptRoot '.first-prompt-done'
try { [System.IO.File]::WriteAllText($stateFile, [datetime]::UtcNow.ToString('o')) } catch {}

$context = "## Hive Memory Protocol — ACTIVE`n`nMANDATORY FIRST ACTION: call mem_context to recover memory from previous sessions before responding to the user.`n`nIf mem_context is not available, call ToolSearch with query ""mem_context"" to load it first.`n`nDo not respond to the user until memory context has been loaded."

Write-Output (@{ additionalContext = $context } | ConvertTo-Json -Compress)
exit 0
