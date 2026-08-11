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
# Use the official Go Alpine image to produce a static binary for the target platform.
FROM golang:1.26-alpine AS api-builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /app/hive-api

# Copy module files before source so Docker can cache dependency downloads.
COPY hive-api/go.mod hive-api/go.sum ./
COPY hivederive/go.mod hivederive/go.sum /app/hivederive/
RUN go mod download

COPY hive-api/ ./
COPY hivederive/ /app/hivederive/

# -ldflags="-w -s" strips debug/symbol tables for a smaller runtime binary.
# CGO_ENABLED=0 keeps the binary static for the distroless runtime image.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-w -s" -o hive-api ./cmd/server/

# ============================================================
# Stage 3: Runtime
# ============================================================
# distroless/static-debian12:nonroot has no shell or package manager and runs as
# the built-in nonroot user by default.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copy only the API binary and compiled dashboard assets.
COPY --from=api-builder --chown=nonroot:nonroot --chmod=0755 /app/hive-api/hive-api .
COPY --from=dashboard-builder --chown=nonroot:nonroot --chmod=0755 /app/hive-dashboard/dist ./dashboard

# Enables the existing hive-api static serving configuration.
ENV DASHBOARD_ASSETS_DIR=/app/dashboard

# Port used by the API server. Override with PORT when needed.
EXPOSE 8080

# Keep the runtime explicit even though the image tag already defaults to nonroot.
USER nonroot:nonroot

# The binary is the only container process. Exec form keeps signal handling direct.
ENTRYPOINT ["/app/hive-api"]
