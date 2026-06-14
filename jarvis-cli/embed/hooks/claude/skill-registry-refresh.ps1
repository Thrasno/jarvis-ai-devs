# Jarvis — project skill registry refresh hook for Claude Code on Windows.
# Non-fatal and quiet: runs `jarvis skill-registry refresh --quiet --cwd`
# only against the active project/worktree cwd.

$ErrorActionPreference = 'SilentlyContinue'
$JarvisExecutable = {{JARVIS_EXECUTABLE}}
$inputJson = [Console]::In.ReadToEnd()
$payload = $null
try {
    if (-not [string]::IsNullOrWhiteSpace($inputJson)) {
        $payload = $inputJson | ConvertFrom-Json
    }
} catch {}

function Resolve-Directory {
    param([object]$Payload)

    foreach ($name in @('HIVE_PROJECT_DIRECTORY', 'JARVIS_WORKSPACE_DIRECTORY')) {
        $value = [Environment]::GetEnvironmentVariable($name)
        if (-not [string]::IsNullOrWhiteSpace($value)) { return $value }
    }
    if ($null -ne $Payload) {
        foreach ($name in @('directory', 'cwd', 'worktree')) {
            if ($Payload.PSObject.Properties.Name -contains $name) {
                $value = [string]$Payload.$name
                if (-not [string]::IsNullOrWhiteSpace($value)) { return $value }
            }
        }
        if ($Payload.PSObject.Properties.Name -contains 'workspace' -and $null -ne $Payload.workspace) {
            if ($Payload.workspace.PSObject.Properties.Name -contains 'directory') {
                $value = [string]$Payload.workspace.directory
                if (-not [string]::IsNullOrWhiteSpace($value)) { return $value }
            }
        }
    }
    try { return [IO.Directory]::GetCurrentDirectory() } catch { return '' }
}

function Write-WarningLine {
    param([string]$Message)
    if (-not [string]::IsNullOrWhiteSpace($Message)) {
        [Console]::Error.WriteLine("Project skill registry warning: $Message")
    }
}

$directory = Resolve-Directory -Payload $payload
if ([string]::IsNullOrWhiteSpace($directory)) {
    Write-WarningLine 'active project directory unavailable'
    exit 0
}

if ([string]::IsNullOrWhiteSpace($JarvisExecutable) -or -not (Test-Path -LiteralPath $JarvisExecutable -PathType Leaf)) {
    Write-WarningLine 'jarvis executable unavailable'
    exit 0
}

$stderrPath = Join-Path ([IO.Path]::GetTempPath()) ("jarvis-skill-registry-$PID.err")
$stdoutPath = Join-Path ([IO.Path]::GetTempPath()) ("jarvis-skill-registry-$PID.out")
try {
    $process = Start-Process -FilePath $JarvisExecutable -ArgumentList @('skill-registry', 'refresh', '--quiet', '--cwd', $directory) -NoNewWindow -PassThru -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath
    if (-not $process.WaitForExit({{JARVIS_REFRESH_TIMEOUT_MILLIS}})) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
        Write-WarningLine "refresh timed out for $directory"
    } elseif ($process.ExitCode -ne 0) {
        Write-WarningLine "refresh failed for $directory"
    }
    if (Test-Path $stderrPath) {
        Get-Content -Path $stderrPath | ForEach-Object { Write-WarningLine $_ }
    }
} catch {
    Write-WarningLine "refresh failed for $directory"
} finally {
    Remove-Item -Path $stderrPath, $stdoutPath -Force -ErrorAction SilentlyContinue
}
exit 0
