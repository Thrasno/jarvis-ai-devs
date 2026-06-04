# Hive — UserPromptSubmit hook for Claude Code on Windows.
# Reads {"prompt":"..."} from stdin and POSTs it to the local hive-daemon HTTP server.
# Always exits 0 and emits {} — never blocks Claude Code.

$ErrorActionPreference = 'SilentlyContinue'

$port = if ($env:HIVE_HTTP_PORT) { $env:HIVE_HTTP_PORT } else { '7438' }
$inputJson = [Console]::In.ReadToEnd()
$prompt = ''

function Resolve-PowerShellExecutable {
    try {
        $currentProcessPath = (Get-Process -Id $PID).Path
        if (-not [string]::IsNullOrWhiteSpace($currentProcessPath)) {
            return $currentProcessPath
        }
    } catch {
    }

    foreach ($candidate in @('pwsh', 'powershell')) {
        try {
            $command = Get-Command -Name $candidate -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($null -ne $command) {
                if (-not [string]::IsNullOrWhiteSpace($command.Path)) {
                    return $command.Path
                }
                if (-not [string]::IsNullOrWhiteSpace($command.Source)) {
                    return $command.Source
                }
            }
        } catch {
        }
    }

    return $null
}

try {
    if (-not [string]::IsNullOrWhiteSpace($inputJson)) {
        $payload = $inputJson | ConvertFrom-Json
        if ($null -ne $payload.prompt) {
            $prompt = [string]$payload.prompt
        }
    }
} catch {
    $prompt = ''
}

if (-not [string]::IsNullOrWhiteSpace($prompt)) {
    $body = @{ content = $prompt } | ConvertTo-Json -Compress
    $uri = "http://127.0.0.1:$port/prompts"

    try {
        $uriEncoded = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($uri))
        $bodyEncoded = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($body))
        $worker = @"
`$TargetUri = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('$uriEncoded'))
`$JsonBody = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('$bodyEncoded'))
try {
    Invoke-RestMethod -Uri `$TargetUri -Method Post -ContentType 'application/json' -Body `$JsonBody -TimeoutSec 1 | Out-Null
} catch {
}
"@
        $encodedCommand = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($worker))
        $powerShellPath = Resolve-PowerShellExecutable
        if ([string]::IsNullOrWhiteSpace($powerShellPath)) {
            throw 'PowerShell executable unavailable'
        }
        $processInfo = [Diagnostics.ProcessStartInfo]::new()
        $processInfo.FileName = $powerShellPath
        $processInfo.Arguments = "-NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand $encodedCommand"
        $processInfo.UseShellExecute = $true
        $processInfo.WindowStyle = [Diagnostics.ProcessWindowStyle]::Hidden
        [Diagnostics.Process]::Start($processInfo) | Out-Null
    } catch {
        try {
            $powerShellPath = Resolve-PowerShellExecutable
            if (-not [string]::IsNullOrWhiteSpace($powerShellPath)) {
                $nullDevice = if ($IsWindows -or $env:OS -eq 'Windows_NT') { 'NUL' } else { '/dev/null' }
                Start-Process -FilePath $powerShellPath -WindowStyle Hidden -ArgumentList '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-EncodedCommand', $encodedCommand -RedirectStandardInput $nullDevice -RedirectStandardOutput $nullDevice -RedirectStandardError $nullDevice | Out-Null
            }
        } catch {
        }
    }
}

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

# First-prompt injection design:
# When SessionStart is installed, session-start.{ps1,sh} creates this session-scoped
# marker in temp storage before any user prompt, so this branch is skipped. When
# SessionStart is unavailable, this branch creates the marker for the current session
# and acts as the fallback injection mechanism.
$stateFile = Get-HiveSessionStateFile -Payload $payload
$created = $false
try {
    $stream = [System.IO.File]::Open($stateFile, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
    $writer = [System.IO.StreamWriter]::new($stream)
    $writer.Write([datetime]::UtcNow.ToString('o'))
    $writer.Close()
    $stream.Close()
    $created = $true
} catch {}
if ($created) {
    $msg = 'Memory protocol is active. FIRST ACTION: call mem_context to load session memory before responding to the user.'
    Write-Output (@{ systemMessage = $msg } | ConvertTo-Json -Compress)
} else {
    Write-Output '{}'
}
exit 0
