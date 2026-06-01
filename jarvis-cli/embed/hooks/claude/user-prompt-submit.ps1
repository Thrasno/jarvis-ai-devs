# Hive — UserPromptSubmit hook for Claude Code on Windows.
# Reads {"prompt":"..."} from stdin and POSTs it to the local hive-daemon HTTP server.
# Always exits 0 and emits {} — never blocks Claude Code.

$ErrorActionPreference = 'SilentlyContinue'

$port = if ($env:HIVE_HTTP_PORT) { $env:HIVE_HTTP_PORT } else { '7438' }
$inputJson = [Console]::In.ReadToEnd()
$prompt = ''

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
        $processInfo = [Diagnostics.ProcessStartInfo]::new()
        $processInfo.FileName = (Get-Process -Id $PID).Path
        $processInfo.Arguments = "-NoProfile -NonInteractive -ExecutionPolicy Bypass -EncodedCommand $encodedCommand"
        $processInfo.UseShellExecute = $true
        $processInfo.WindowStyle = [Diagnostics.ProcessWindowStyle]::Hidden
        [Diagnostics.Process]::Start($processInfo) | Out-Null
    } catch {
        try {
            $nullDevice = if ($IsWindows -or $env:OS -eq 'Windows_NT') { 'NUL' } else { '/dev/null' }
            Start-Process -FilePath 'powershell.exe' -WindowStyle Hidden -ArgumentList '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass', '-EncodedCommand', $encodedCommand -RedirectStandardInput $nullDevice -RedirectStandardOutput $nullDevice -RedirectStandardError $nullDevice | Out-Null
        } catch {
        }
    }
}

Write-Output '{}'
exit 0
