# =============================================================================
# Jarvis Installer — Windows PowerShell
# =============================================================================
# Uso: irm https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.ps1 | iex
# Overrides opcionales:
#   $env:JARVIS_INSTALL_REPO = "owner/repo"   (default: Thrasno/jarvis-ai-devs)
#   $env:JARVIS_INSTALL_VERSION = "vX.Y.Z"    (si se define, no consulta releases/latest)
# =============================================================================

$ErrorActionPreference = "Stop"

$DEFAULT_REPO = "Thrasno/jarvis-ai-devs"
$REPO = if ($env:JARVIS_INSTALL_REPO) { $env:JARVIS_INSTALL_REPO } else { $DEFAULT_REPO }
$INSTALL_DIR = "$env:LOCALAPPDATA\Programs\jarvis"
$VERSION_OVERRIDE = $env:JARVIS_INSTALL_VERSION
$RETRY_MAX = 4
$RETRY_BASE_DELAY_SECONDS = 1

function Write-Info { param($msg) Write-Host "[INFO] $msg" -ForegroundColor Green }
function Write-Warn { param($msg) Write-Host "[WARN] $msg" -ForegroundColor Yellow }
function Write-Err { param($msg) Write-Host "[ERROR] $msg" -ForegroundColor Red; exit 1 }

function Is-RetryableStatusCode {
    param([int]$StatusCode)
    return $StatusCode -in @(0, 429, 500, 502, 503, 504)
}

function Get-ContentTypeFromHeaders {
    param($Headers)
    if (-not $Headers) { return "" }
    if ($Headers["Content-Type"]) {
        return ($Headers["Content-Type"].ToString().Split(";")[0].Trim().ToLower())
    }
    return ""
}

function Invoke-WebRequestWithRetry {
    param(
        [string]$Uri,
        [string]$OutFile,
        [string]$Label
    )

    $attempt = 1
    $delay = $RETRY_BASE_DELAY_SECONDS

    while ($attempt -le $RETRY_MAX) {
        try {
            if ($OutFile) {
                $response = Invoke-WebRequest -Uri $Uri -OutFile $OutFile -UseBasicParsing -PassThru
            } else {
                $response = Invoke-WebRequest -Uri $Uri -UseBasicParsing
            }

            $contentType = Get-ContentTypeFromHeaders $response.Headers
            if ($response.StatusCode -eq 200) {
                if ($contentType -like "text/html*" -or $contentType -like "application/json*") {
                    Write-Err "Descarga invalida para $Label: el servidor devolvio content-type=$contentType en lugar del artefacto esperado. URI: $Uri"
                }
                return $response
            }

            if (Is-RetryableStatusCode $response.StatusCode -and $attempt -lt $RETRY_MAX) {
                Write-Warn "$Label devolvio HTTP $($response.StatusCode). Reintentando en ${delay}s (backoff, intento $attempt/$RETRY_MAX)..."
                Start-Sleep -Seconds $delay
                $delay = $delay * 2
                $attempt++
                continue
            }

            if (Is-RetryableStatusCode $response.StatusCode) {
                Write-Err "$Label fallo por HTTP $($response.StatusCode) de forma repetida (transitorio). Reintenta en unos minutos."
            }

            Write-Err "$Label fallo con HTTP $($response.StatusCode). URI: $Uri"
        } catch {
            $statusCode = 0
            if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
                $statusCode = [int]$_.Exception.Response.StatusCode.value__
            }

            if ($statusCode -eq 404) {
                Write-Err "$Label no encontrado (HTTP 404). Verifica repo/version: JARVIS_INSTALL_REPO=$REPO JARVIS_INSTALL_VERSION=$VERSION_OVERRIDE"
            }

            if (Is-RetryableStatusCode $statusCode -and $attempt -lt $RETRY_MAX) {
                Write-Warn "$Label fallo con HTTP $statusCode. Reintentando en ${delay}s (backoff, intento $attempt/$RETRY_MAX)..."
                Start-Sleep -Seconds $delay
                $delay = $delay * 2
                $attempt++
                continue
            }

            if (Is-RetryableStatusCode $statusCode) {
                Write-Err "$Label fallo por HTTP $statusCode de forma repetida (transitorio). Reintenta en unos minutos."
            }

            Write-Err "$Label fallo al descargar desde $Uri"
        }
    }

    Write-Err "$Label fallo al descargar desde $Uri"
}

function Test-ZipArchive {
    param([string]$ZipPath)

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    try {
        $zip = [System.IO.Compression.ZipFile]::OpenRead($ZipPath)
        $zip.Dispose()
        return $true
    } catch {
        return $false
    }
}

# -----------------------------------------------------------------------------
# Detectar arquitectura
# -----------------------------------------------------------------------------
function Get-Architecture {
    $arch = $env:PROCESSOR_ARCHITECTURE
    switch ($arch) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { Write-Err "Arquitectura no soportada: $arch" }
    }
}

# -----------------------------------------------------------------------------
# Obtener última versión desde GitHub API
# -----------------------------------------------------------------------------
function Get-LatestVersion {
    if ($VERSION_OVERRIDE) {
        Write-Info "Usando version explicita: $VERSION_OVERRIDE"
        return $VERSION_OVERRIDE
    }

    $latestUrl = "https://api.github.com/repos/$REPO/releases/latest"

    $attempt = 1
    $delay = $RETRY_BASE_DELAY_SECONDS
    while ($attempt -le $RETRY_MAX) {
        try {
            $response = Invoke-WebRequest -Uri $latestUrl -UseBasicParsing
            if ($response.StatusCode -eq 200) {
                $json = $response.Content | ConvertFrom-Json
                $version = $json.tag_name
                if (-not $version) {
                    Write-Err "La respuesta de GitHub no incluyo tag_name valido para $REPO"
                }

                Write-Info "Ultima version: $version"
                return $version
            }

            if (Is-RetryableStatusCode $response.StatusCode -and $attempt -lt $RETRY_MAX) {
                Write-Warn "GitHub API devolvio HTTP $($response.StatusCode) al consultar latest release. Reintentando en ${delay}s (backoff, intento $attempt/$RETRY_MAX)..."
                Start-Sleep -Seconds $delay
                $delay = $delay * 2
                $attempt++
                continue
            }

            if (Is-RetryableStatusCode $response.StatusCode) {
                Write-Err "No se pudo obtener la ultima version: GitHub API devolvio HTTP $($response.StatusCode) de forma repetida (transitorio). Reintenta en unos minutos."
            }

            Write-Err "No se pudo obtener la ultima version desde $latestUrl (HTTP $($response.StatusCode))"
        } catch {
            $statusCode = 0
            if ($_.Exception.Response -and $_.Exception.Response.StatusCode) {
                $statusCode = [int]$_.Exception.Response.StatusCode.value__
            }

            if ($statusCode -eq 404) {
                Write-Err "No releases found en $REPO (releases/latest devolvio 404). Usa `\$env:JARVIS_INSTALL_VERSION='vX.Y.Z'` o `\$env:JARVIS_INSTALL_REPO='owner/repo'`. Si todavia no hay artefactos publicos, instala desde el codigo fuente en este repositorio."
            }

            if (Is-RetryableStatusCode $statusCode -and $attempt -lt $RETRY_MAX) {
                Write-Warn "GitHub API fallo con HTTP $statusCode. Reintentando en ${delay}s (backoff, intento $attempt/$RETRY_MAX)..."
                Start-Sleep -Seconds $delay
                $delay = $delay * 2
                $attempt++
                continue
            }

            if (Is-RetryableStatusCode $statusCode) {
                Write-Err "No se pudo obtener la ultima version: GitHub API devolvio HTTP $statusCode de forma repetida (transitorio). Reintenta en unos minutos."
            }

            Write-Err "No se pudo obtener la ultima version desde $latestUrl (HTTP $statusCode)"
        }
    }

    Write-Err "No se pudo obtener la ultima version desde $latestUrl"
}

# -----------------------------------------------------------------------------
# Descargar y extraer binario
# -----------------------------------------------------------------------------
function Install-Binary {
    param($Name, $Version, $Arch)
    
    $versionNumber = $Version.TrimStart("v")
    $url = "https://github.com/$REPO/releases/download/$Version/${Name}_${versionNumber}_windows_${Arch}.zip"
    $tmpDir = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory -Path $_ }
    $zipPath = Join-Path $tmpDir "$Name.zip"
    
    Write-Info "Descargando $Name..."

    $response = Invoke-WebRequestWithRetry -Uri $url -OutFile $zipPath -Label $Name
    $contentType = Get-ContentTypeFromHeaders $response.Headers
    if ($contentType -like "text/html*" -or $contentType -like "application/json*") {
        Write-Err "Descarga invalida para $Name: content-type=$contentType (probable pagina HTML o error de API)."
    }

    if (-not (Test-ZipArchive -ZipPath $zipPath)) {
        Write-Err "El archivo descargado para $Name no es un zip valido (posible HTML/error del CDN)."
    }

    Expand-Archive -Path $zipPath -DestinationPath $tmpDir -Force
    
    # Crear directorio de instalación si no existe
    if (-not (Test-Path $INSTALL_DIR)) {
        New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null
    }
    
    $binaryPath = Join-Path $tmpDir "$Name.exe"
    if (-not (Test-Path $binaryPath)) {
        Write-Err "El artefacto de $Name se extrajo pero no contiene $Name.exe"
    }

    Move-Item -Path $binaryPath -Destination $INSTALL_DIR -Force
    Remove-Item -Path $tmpDir -Recurse -Force
    
    Write-Info "$Name instalado en $INSTALL_DIR\$Name.exe"
}

# -----------------------------------------------------------------------------
# Agregar al PATH si no está
# -----------------------------------------------------------------------------
function Add-ToPath {
    $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    
    if ($currentPath -notlike "*$INSTALL_DIR*") {
        Write-Info "Agregando $INSTALL_DIR al PATH..."
        [Environment]::SetEnvironmentVariable("PATH", "$currentPath;$INSTALL_DIR", "User")
        $env:PATH = "$env:PATH;$INSTALL_DIR"
        Write-Info "PATH actualizado. Reinicia la terminal para que tome efecto."
    } else {
        Write-Info "$INSTALL_DIR ya esta en el PATH"
    }
}

# -----------------------------------------------------------------------------
# Main
# -----------------------------------------------------------------------------
function Main {
    Write-Host ""
    Write-Host "==============================================" -ForegroundColor Cyan
    Write-Host "       Jarvis Installer (Windows)" -ForegroundColor Cyan
    Write-Host "==============================================" -ForegroundColor Cyan
    Write-Host ""
    
    $arch = Get-Architecture
    Write-Info "Arquitectura detectada: windows/$arch"
    
    $version = Get-LatestVersion
    
    Install-Binary -Name "jarvis" -Version $version -Arch $arch
    Install-Binary -Name "hive-daemon" -Version $version -Arch $arch
    
    Add-ToPath
    
    Write-Host ""
    Write-Host "==============================================" -ForegroundColor Green
    Write-Host "Instalacion completada!" -ForegroundColor Green
    Write-Host "==============================================" -ForegroundColor Green
    Write-Host ""
    Write-Host "Siguiente paso: ejecuta 'jarvis' para configurar o reconfigurar este equipo"
    Write-Host ""
}

Main
