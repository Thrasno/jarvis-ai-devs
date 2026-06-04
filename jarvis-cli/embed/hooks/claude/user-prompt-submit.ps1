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

# First-prompt injection design:
# When SessionStart hook is installed: session-start.{ps1,sh} pre-creates .first-prompt-done
# before any user prompt, so this branch is never reached — sessionStart is the primary
# injection path.
# When SessionStart is NOT installed (e.g. older Claude Code builds): this branch fires on
# the first prompt and acts as the fallback injection mechanism.
$stateFile = Join-Path $PSScriptRoot '.first-prompt-done'
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
