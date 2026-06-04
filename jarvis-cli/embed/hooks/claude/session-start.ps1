# Hive — SessionStart hook for Claude Code on Windows.
# Injects memory protocol instructions as additionalContext before the first prompt.
# Always exits 0 and emits valid JSON — never blocks Claude Code.

# Primary injection path: this hook fires at session start, records a session-scoped
# first-prompt marker in temp storage, and injects additionalContext before the first
# user prompt. The first-prompt branch in user-prompt-submit.{ps1,sh} is the fallback
# when this hook is not installed (e.g. older Claude Code builds without SessionStart).

$ErrorActionPreference = 'SilentlyContinue'

function Resolve-HiveSessionId {
    param([object]$Payload)

    foreach ($name in @('HIVE_CLAUDE_SESSION_ID', 'CLAUDE_SESSION_ID', 'SESSION_ID')) {
        $value = [Environment]::GetEnvironmentVariable($name)
        if (-not [string]::IsNullOrWhiteSpace($value)) { return $value }
    }

    if ($null -ne $Payload) {
        foreach ($name in @('session_id', 'sessionId', 'transcript_path', 'transcriptPath')) {
            if ($Payload.PSObject.Properties.Name -contains $name) {
                $value = [string]$Payload.$name
                if (-not [string]::IsNullOrWhiteSpace($value)) { return $value }
            }
        }
        if ($Payload.PSObject.Properties.Name -contains 'session' -and $null -ne $Payload.session) {
            if ($Payload.session.PSObject.Properties.Name -contains 'id') {
                $value = [string]$Payload.session.id
                if (-not [string]::IsNullOrWhiteSpace($value)) { return $value }
            }
        }
    }

    return Resolve-HiveFallbackSessionId
}

function Resolve-HiveFallbackSessionId {
    try {
        $process = Get-CimInstance -ClassName Win32_Process -Filter "ProcessId = $PID"
        if ($null -ne $process -and $null -ne $process.ParentProcessId) {
            return "ppid-$($process.ParentProcessId)"
        }
    } catch {}

    return "ppid-$PID"
}

function Get-HiveSessionStateFile {
    param([object]$Payload)

    $sessionId = Resolve-HiveSessionId -Payload $Payload
    $safeSessionId = [regex]::Replace($sessionId, '[^A-Za-z0-9_.-]', '_')
    if ($safeSessionId.Length -gt 160) { $safeSessionId = $safeSessionId.Substring(0, 160) }
    if ([string]::IsNullOrWhiteSpace($safeSessionId)) { $safeSessionId = 'unknown' }
    $stateRoot = Join-Path ([IO.Path]::GetTempPath()) 'jarvis-hive/claude-hooks'
    try { [IO.Directory]::CreateDirectory($stateRoot) | Out-Null } catch {}
    return Join-Path $stateRoot "first-prompt-$safeSessionId.done"
}

$inputJson = [Console]::In.ReadToEnd()
$payload = $null
try {
    if (-not [string]::IsNullOrWhiteSpace($inputJson)) { $payload = $inputJson | ConvertFrom-Json }
} catch {}

$stateFile = Get-HiveSessionStateFile -Payload $payload
try { [System.IO.File]::WriteAllText($stateFile, [datetime]::UtcNow.ToString('o')) } catch {}

$context = "## Hive Memory Protocol — ACTIVE`n`nMANDATORY FIRST ACTION: call mem_context to recover memory from previous sessions before responding to the user.`n`nIf mem_context is not available, call ToolSearch with query ""mem_context"" to load it first.`n`nDo not respond to the user until memory context has been loaded."

Write-Output (@{ additionalContext = $context } | ConvertTo-Json -Compress)
exit 0
