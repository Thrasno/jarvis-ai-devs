# Hive — Stop hook for Claude Code on Windows.
# Cleans up only the current session marker so other active sessions keep their state.

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
    return Join-Path $stateRoot "first-prompt-$safeSessionId.done"
}

$inputJson = [Console]::In.ReadToEnd()
$payload = $null
try {
    if (-not [string]::IsNullOrWhiteSpace($inputJson)) { $payload = $inputJson | ConvertFrom-Json }
} catch {}

$stateFile = Get-HiveSessionStateFile -Payload $payload
try { Remove-Item $stateFile -Force -ErrorAction SilentlyContinue } catch {}
Write-Output '{}'
exit 0
