# ============================================================
# Etapa 1: Builder
# ============================================================
# Usamos la imagen oficial de Go con Alpine (Linux minimalista).
# alpine = imagen muy pequeña (~5MB) sin las librerías glibc de Ubuntu.
# CGO_ENABLED=0 es CRÍTICO: desactiva CGO para producir un binario estático.
# Un binario estático no depende de ninguna librería del sistema operativo —
# funciona en cualquier Linux, incluso en una imagen scratch (sin OS).
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copiamos los archivos de módulo ANTES que el código fuente.
# Docker cachea capas por cambios. Si solo cambia el código (no las deps),
# Docker reutiliza la capa de "go mod download" — mucho más rápido.
COPY go.mod go.sum ./
RUN go mod download

# Copiamos el código fuente completo
COPY . .

# Compilamos el binario.
# -ldflags="-w -s" elimina la tabla de debug y el mapa de símbolos del binario.
# Resultado: binario ~30% más pequeño. Sin impacto en runtime, solo en debugging.
# CGO_ENABLED=0 garantiza que no haya dependencias dinámicas.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o hive-api ./cmd/server/

# ============================================================
# Etapa 2: Runtime
# ============================================================
# distroless/static-debian12 es una imagen sin shell, sin package manager,
# sin ninguna herramienta — solo el runtime mínimo para ejecutar Go estático.
# Es ~2MB. Tiene ventajas de seguridad enormes: si alguien logra RCE,
# no tiene nada disponible (ni sh, ni curl, ni ls).
FROM gcr.io/distroless/static-debian12

WORKDIR /app

# Copiamos solo el binario compilado desde la etapa builder.
# La imagen final no tiene el código fuente, ni las herramientas de Go.
COPY --from=builder /app/hive-api .

# Puerto en el que escucha el servidor (configurable por env var PORT).
EXPOSE 8080

# El binario es el único proceso del contenedor.
# Usamos la forma JSON (exec form) para que el proceso sea PID 1 —
# esto garantiza que reciba las señales del OS (SIGTERM en kubectl delete pod).
ENTRYPOINT ["/app/hive-api"]
