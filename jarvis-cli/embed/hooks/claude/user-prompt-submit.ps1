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

    Start-Job -ScriptBlock {
        param($TargetUri, $JsonBody)
        try {
            Invoke-RestMethod -Uri $TargetUri -Method Post -ContentType 'application/json' -Body $JsonBody -TimeoutSec 1 | Out-Null
        } catch {
        }
    } -ArgumentList $uri, $body | Out-Null
}

Write-Output '{}'
exit 0
