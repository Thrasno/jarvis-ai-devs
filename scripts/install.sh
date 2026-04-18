#!/bin/bash
# =============================================================================
# Jarvis Installer — Linux / macOS
# =============================================================================
# Uso: curl -sSL https://raw.githubusercontent.com/Thrasno/jarvis-dev/main/scripts/install.sh | bash
# =============================================================================

set -e

REPO="Thrasno/jarvis-dev"
INSTALL_DIR="/usr/local/bin"

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
    VERSION=$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [ -z "$VERSION" ]; then
        error "No se pudo obtener la última versión. ¿Hay releases publicadas?"
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
    echo "Siguiente paso: ejecuta 'jarvis' para iniciar el wizard"
    echo ""
}

main "$@"
