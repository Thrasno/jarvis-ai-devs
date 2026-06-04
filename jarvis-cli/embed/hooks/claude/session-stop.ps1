# Hive — Stop hook for Claude Code on Windows.
# Cleans up session state so the next session gets fresh memory injection.

$ErrorActionPreference = 'SilentlyContinue'
$stateFile = Join-Path $PSScriptRoot '.first-prompt-done'
try { Remove-Item $stateFile -Force -ErrorAction SilentlyContinue } catch {}
Write-Output '{}'
exit 0
