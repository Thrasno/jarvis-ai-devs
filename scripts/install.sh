#!/bin/bash
# =============================================================================
# Jarvis Installer — Linux / macOS
# =============================================================================
# Uso: curl -sSL https://raw.githubusercontent.com/Thrasno/jarvis-ai-devs/main/scripts/install.sh | bash
# Overrides opcionales:
#   JARVIS_INSTALL_REPO=owner/repo       (default: Thrasno/jarvis-ai-devs)
#   JARVIS_INSTALL_VERSION=vX.Y.Z        (si se define, no consulta releases/latest)
# =============================================================================

set -e

DEFAULT_REPO="Thrasno/jarvis-ai-devs"
REPO="${JARVIS_INSTALL_REPO:-$DEFAULT_REPO}"
INSTALL_DIR="/usr/local/bin"
VERSION_OVERRIDE="${JARVIS_INSTALL_VERSION:-}"

# Colores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

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
    local http_code
    http_code=$(curl -sS -o "$response_file" -w "%{http_code}" "$api_url" || true)

    if [ "$http_code" = "404" ]; then
        rm -f "$response_file"
        error "No hay releases publicadas en ${REPO} (el endpoint releases/latest devolvió 404).
Prueba una versión explícita: JARVIS_INSTALL_VERSION=vX.Y.Z
O usa otro repositorio: JARVIS_INSTALL_REPO=owner/repo
Si todavía no hay artefactos públicos, instala desde el código fuente en este repositorio."
    fi

    if [ "$http_code" != "200" ]; then
        rm -f "$response_file"
        error "No se pudo obtener la última versión desde ${api_url} (HTTP ${http_code})."
    fi

    VERSION=$(grep '"tag_name":' "$response_file" | sed -E 's/.*"([^"]+)".*/\1/')
    rm -f "$response_file"

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
    
    if ! curl -sSL "$url" -o "${tmp_dir}/${name}.tar.gz"; then
        error "Error descargando ${name} desde ${url}"
    fi
    
    tar -xzf "${tmp_dir}/${name}.tar.gz" -C "$tmp_dir"
    
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
