#!/bin/bash
# =============================================================================
# Jarvis Installer — Linux / macOS
# =============================================================================
# Uso: curl -sSL https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/master/scripts/install.sh | bash
# Overrides opcionales:
#   JARVIS_INSTALL_REPO=owner/repo       (default: Thrasno/jarvis-ai-devs)
#   JARVIS_INSTALL_VERSION=vX.Y.Z        (si se define, no consulta releases/latest)
# =============================================================================

set -euo pipefail

DEFAULT_REPO="Thrasno/jarvis-ai-devs"
REPO="${JARVIS_INSTALL_REPO:-$DEFAULT_REPO}"
INSTALL_DIR="/usr/local/bin"
VERSION_OVERRIDE="${JARVIS_INSTALL_VERSION:-}"
RETRY_MAX=4
RETRY_BASE_DELAY=1

# Colores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

is_retryable_status() {
    case "$1" in
        000|429|500|502|503|504) return 0 ;;
        *) return 1 ;;
    esac
}

get_content_type() {
    local headers_file=$1
    grep -i '^content-type:' "$headers_file" | tail -n 1 | sed -E 's/[Cc]ontent-[Tt]ype:[[:space:]]*([^;[:space:]]+).*/\1/' | tr '[:upper:]' '[:lower:]' || true
}

download_with_retry() {
    local url=$1
    local output_file=$2
    local label=$3

    local attempt=1
    local delay=$RETRY_BASE_DELAY

    while [ "$attempt" -le "$RETRY_MAX" ]; do
        local headers_file
        headers_file=$(mktemp)

        local http_code
        http_code=$(curl -sS -L --connect-timeout 10 --max-time 180 -D "$headers_file" -o "$output_file" -w "%{http_code}" "$url" || true)
        local content_type
        content_type=$(get_content_type "$headers_file")

        if [ "$http_code" = "200" ]; then
            if [[ "$content_type" == text/html* ]] || [[ "$content_type" == application/json* ]]; then
                rm -f "$headers_file"
                error "Descarga inválida para ${label}: el servidor devolvió content-type=${content_type:-unknown} en lugar del artefacto esperado. URL: ${url}"
            fi
            rm -f "$headers_file"
            return 0
        fi

        if is_retryable_status "$http_code" && [ "$attempt" -lt "$RETRY_MAX" ]; then
            warn "Descarga de ${label} falló con HTTP ${http_code} (transitorio). Reintentando en ${delay}s (backoff, intento ${attempt}/${RETRY_MAX})..."
            rm -f "$headers_file"
            sleep "$delay"
            delay=$((delay * 2))
            attempt=$((attempt + 1))
            continue
        fi

        rm -f "$headers_file"

        if is_retryable_status "$http_code"; then
            error "No se pudo descargar ${label}. GitHub/CDN respondió HTTP ${http_code} repetidamente. Parece un fallo transitorio; intentá nuevamente en unos minutos."
        fi

        if [ "$http_code" = "404" ]; then
            error "No se encontró el artefacto ${label} (HTTP 404). Verificá repo/version: JARVIS_INSTALL_REPO=${REPO} JARVIS_INSTALL_VERSION=${VERSION}."
        fi

        error "No se pudo descargar ${label} desde ${url} (HTTP ${http_code})."
    done

    error "No se pudo descargar ${label} desde ${url}."
}

# -----------------------------------------------------------------------------
# Detectar OS y arquitectura
# -----------------------------------------------------------------------------
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        linux)  OS="linux" ;;
        darwin) OS="darwin" ;;
        *)      error "OS no soportado: $OS" ;;
    esac

    case "$ARCH" in
        x86_64)  ARCH="amd64" ;;
        amd64)   ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        arm64)   ARCH="arm64" ;;
        *)       error "Arquitectura no soportada: $ARCH" ;;
    esac

    info "Plataforma detectada: ${OS}/${ARCH}"
}

# -----------------------------------------------------------------------------
# Obtener última versión desde GitHub API
# -----------------------------------------------------------------------------
get_latest_version() {
    if [ -n "$VERSION_OVERRIDE" ]; then
        VERSION="$VERSION_OVERRIDE"
        info "Usando versión explícita: ${VERSION}"
        return
    fi

    local api_url="https://api.github.com/repos/${REPO}/releases/latest"
    local response_file
    response_file=$(mktemp)
    local headers_file
    headers_file=$(mktemp)
    local attempt=1
    local delay=$RETRY_BASE_DELAY

    while [ "$attempt" -le "$RETRY_MAX" ]; do
        local http_code
        http_code=$(curl -sS -L --connect-timeout 10 --max-time 60 -D "$headers_file" -o "$response_file" -w "%{http_code}" "$api_url" || true)

        if [ "$http_code" = "200" ]; then
            break
        fi

        if [ "$http_code" = "404" ]; then
            rm -f "$response_file" "$headers_file"
            error "No releases found en ${REPO} (el endpoint releases/latest devolvió 404).
Prueba una versión explícita: JARVIS_INSTALL_VERSION=vX.Y.Z
O usa otro repositorio: JARVIS_INSTALL_REPO=owner/repo
Si todavía no hay artefactos públicos, instala desde el código fuente en este repositorio."
        fi

        if is_retryable_status "$http_code" && [ "$attempt" -lt "$RETRY_MAX" ]; then
            warn "GitHub API devolvió HTTP ${http_code} al consultar latest release. Reintentando en ${delay}s (backoff, intento ${attempt}/${RETRY_MAX})..."
            sleep "$delay"
            delay=$((delay * 2))
            attempt=$((attempt + 1))
            continue
        fi

        if is_retryable_status "$http_code"; then
            rm -f "$response_file" "$headers_file"
            error "No se pudo obtener la última versión: GitHub API devolvió HTTP ${http_code} de forma repetida (falla transitoria). Intentá nuevamente en unos minutos."
        fi

        rm -f "$response_file" "$headers_file"
        error "No se pudo obtener la última versión desde ${api_url} (HTTP ${http_code})."
    done

    VERSION=$(grep '"tag_name":' "$response_file" | sed -E 's/.*"([^"]+)".*/\1/')
    rm -f "$response_file" "$headers_file"

    if [ -z "$VERSION" ] || [ "$VERSION" = "null" ]; then
        error "La respuesta de GitHub no incluyó tag_name válido para ${REPO}."
    fi

    info "Última versión: ${VERSION}"
}

# -----------------------------------------------------------------------------
# Descargar y extraer binario
# -----------------------------------------------------------------------------
download_binary() {
    local name=$1
    local url="https://github.com/${REPO}/releases/download/${VERSION}/${name}_${VERSION#v}_${OS}_${ARCH}.tar.gz"
    local tmp_dir=$(mktemp -d)
    
    info "Descargando ${name}..."

    download_with_retry "$url" "${tmp_dir}/${name}.tar.gz" "${name}"

    if ! tar -tzf "${tmp_dir}/${name}.tar.gz" > /dev/null 2>&1; then
        rm -rf "$tmp_dir"
        error "El archivo descargado para ${name} no es un tar.gz válido (posible HTML/error del CDN)."
    fi

    tar -xzf "${tmp_dir}/${name}.tar.gz" -C "$tmp_dir"

    if [ ! -f "${tmp_dir}/${name}" ]; then
        rm -rf "$tmp_dir"
        error "El artefacto de ${name} se extrajo pero no contiene el binario esperado (${name})."
    fi
    
    # Mover binario al directorio de instalación
    if [ -w "$INSTALL_DIR" ]; then
        mv "${tmp_dir}/${name}" "${INSTALL_DIR}/"
    else
        info "Se requiere sudo para instalar en ${INSTALL_DIR}"
        sudo mv "${tmp_dir}/${name}" "${INSTALL_DIR}/"
    fi
    
    chmod +x "${INSTALL_DIR}/${name}"
    rm -rf "$tmp_dir"
    
    info "${name} instalado en ${INSTALL_DIR}/${name}"
}

# -----------------------------------------------------------------------------
# Verificar instalación
# -----------------------------------------------------------------------------
verify_installation() {
    info "Verificando instalación..."
    
    if command -v jarvis &> /dev/null; then
        echo -e "${GREEN}✓${NC} jarvis: $(which jarvis)"
    else
        warn "jarvis no está en el PATH"
    fi
    
    if command -v hive-daemon &> /dev/null; then
        echo -e "${GREEN}✓${NC} hive-daemon: $(which hive-daemon)"
    else
        warn "hive-daemon no está en el PATH"
    fi
}

# -----------------------------------------------------------------------------
# Main
# -----------------------------------------------------------------------------
main() {
    echo ""
    echo "=============================================="
    echo "       Jarvis Installer"
    echo "=============================================="
    echo ""
    
    detect_platform
    get_latest_version
    
    download_binary "jarvis"
    download_binary "hive-daemon"
    
    echo ""
    verify_installation
    
    echo ""
    echo "=============================================="
    echo -e "${GREEN}Instalación completada!${NC}"
    echo "=============================================="
    echo ""
    echo "Siguiente paso: ejecuta 'jarvis' para configurar o reconfigurar este equipo"
    echo ""
}

main "$@"
