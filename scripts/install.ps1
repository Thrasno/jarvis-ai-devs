# =============================================================================
# Jarvis Installer — Windows PowerShell
# =============================================================================
# Uso: irm https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/main/scripts/install.ps1 | iex
# Overrides opcionales:
#   $env:JARVIS_INSTALL_REPO = "owner/repo"   (default: Thrasno/jarvis-ai-devs)
#   $env:JARVIS_INSTALL_VERSION = "vX.Y.Z"    (si se define, no consulta releases/latest)
# =============================================================================

$ErrorActionPreference = "Stop"

$DEFAULT_REPO = "Thrasno/jarvis-ai-devs"
$REPO = if ($env:JARVIS_INSTALL_REPO) { $env:JARVIS_INSTALL_REPO } else { $DEFAULT_REPO }
$INSTALL_DIR = "$env:LOCALAPPDATA\Programs\jarvis"
$VERSION_OVERRIDE = $env:JARVIS_INSTALL_VERSION

function Write-Info { param($msg) Write-Host "[INFO] $msg" -ForegroundColor Green }
function Write-Warn { param($msg) Write-Host "[WARN] $msg" -ForegroundColor Yellow }
function Write-Err { param($msg) Write-Host "[ERROR] $msg" -ForegroundColor Red; exit 1 }

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

    try {
        $response = Invoke-RestMethod -Uri $latestUrl -UseBasicParsing
    } catch {
        $statusCode = $_.Exception.Response.StatusCode.value__
        if ($statusCode -eq 404) {
            Write-Err "No hay releases publicadas en $REPO (releases/latest devolvio 404). Usa `\$env:JARVIS_INSTALL_VERSION='vX.Y.Z'` o `\$env:JARVIS_INSTALL_REPO='owner/repo'`. Si todavia no hay artefactos publicos, instala desde el codigo fuente en este repositorio."
        }
        Write-Err "No se pudo obtener la ultima version desde $latestUrl (HTTP $statusCode)"
    }

    $version = $response.tag_name
    if (-not $version) {
        Write-Err "La respuesta de GitHub no incluyo tag_name valido para $REPO"
    }
    
    Write-Info "Ultima version: $version"
    return $version
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
    
    try {
        Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing
    } catch {
        Write-Err "Error descargando $Name desde $url"
    }
    
    Expand-Archive -Path $zipPath -DestinationPath $tmpDir -Force
    
    # Crear directorio de instalación si no existe
    if (-not (Test-Path $INSTALL_DIR)) {
        New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null
    }
    
    Move-Item -Path (Join-Path $tmpDir "$Name.exe") -Destination $INSTALL_DIR -Force
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
