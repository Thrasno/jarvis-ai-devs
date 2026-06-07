# ============================================================
# Stage 1: Dashboard builder
# ============================================================
# The build context is the repository root. Dashboard source remains in the
# separate hive-dashboard/ module; only compiled assets are copied into the API
# runtime image.
FROM node:22-alpine AS dashboard-builder

WORKDIR /app/hive-dashboard

COPY hive-dashboard/package.json hive-dashboard/package-lock.json ./
RUN npm ci

COPY hive-dashboard/ ./
RUN npm run build

# ============================================================
# Stage 2: Hive API builder
# ============================================================
# Use the official Go Alpine image to produce a static Linux binary.
FROM golang:1.26-alpine AS api-builder

WORKDIR /app/hive-api

# Copy module files before source so Docker can cache dependency downloads.
COPY hive-api/go.mod hive-api/go.sum ./
RUN go mod download

COPY hive-api/ ./

# -ldflags="-w -s" strips debug/symbol tables for a smaller runtime binary.
# CGO_ENABLED=0 keeps the binary static for the distroless runtime image.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o hive-api ./cmd/server/

# ============================================================
# Stage 3: Runtime
# ============================================================
# distroless/static-debian12 es una imagen sin shell, sin package manager,
# sin ninguna herramienta — solo el runtime mínimo para ejecutar Go estático.
# Es ~2MB. Tiene ventajas de seguridad enormes: si alguien logra RCE,
# no tiene nada disponible (ni sh, ni curl, ni ls).
FROM gcr.io/distroless/static-debian12

WORKDIR /app

# Copiamos solo el binario compilado desde la etapa builder.
# La imagen final no tiene el código fuente, ni las herramientas de Go.
# Copy only the API binary and compiled dashboard assets.
COPY --from=api-builder /app/hive-api/hive-api .
COPY --from=dashboard-builder /app/hive-dashboard/dist ./dashboard

# Enables the existing hive-api static serving configuration.
ENV DASHBOARD_ASSETS_DIR=/app/dashboard

# Puerto en el que escucha el servidor (configurable por env var PORT).
EXPOSE 8080

# El binario es el único proceso del contenedor.
# Usamos la forma JSON (exec form) para que el proceso sea PID 1 —
# esto garantiza que reciba las señales del OS (SIGTERM en kubectl delete pod).
ENTRYPOINT ["/app/hive-api"]
