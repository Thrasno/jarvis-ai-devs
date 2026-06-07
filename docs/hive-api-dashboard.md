# Hive API dashboard deployment

The Hive API dashboard is a same-host admin UI served by `hive-api` from compiled static assets. Operators enable it by building `hive-dashboard/`, making its `dist` directory available to `hive-api`, and setting `DASHBOARD_ASSETS_DIR`.

## Quick path: Docker Compose

From the repository root:

```bash
export JWT_SECRET="$(openssl rand -base64 32)"
docker compose -f hive-api/deploy/docker-compose.yml up --build -d
```

Then open:

```text
http://<hive-api-host>:8080/dashboard
```

The Compose file builds the dashboard from `hive-dashboard/` and copies `dist` into the API container at `/app/dashboard`. The image sets `DASHBOARD_ASSETS_DIR=/app/dashboard`; Compose does not override it so the runtime image remains the single source of truth for the compiled asset path.

## Configuration

| Setting | Required | Purpose |
|---------|----------|---------|
| `DATABASE_URL` | Yes | PostgreSQL connection used by `hive-api`. |
| `JWT_SECRET` | Yes | Signs Hive API JWT sessions. Must be at least 32 characters. Compose fails if this is not provided. |
| `PORT` | No | API/dashboard port. Defaults to `8080`. |
| `GIN_MODE` | No | Use `release` for production and `debug` for local development. |
| `DASHBOARD_ASSETS_DIR` | Dashboard only | Directory containing compiled dashboard assets, including `index.html`. |

If `DASHBOARD_ASSETS_DIR` is empty, `hive-api` does not register `/dashboard` routes and API routes continue to behave normally.

Generate `JWT_SECRET` with a secret generator, keep it outside git, and rotate it through your deployment secret manager. A missing or shorter-than-32-character value prevents `hive-api` from starting.

## Manual same-host deployment

Use this path when building outside Docker:

```bash
cd hive-dashboard
npm ci
npm run build
```

Start `hive-api` with the compiled asset directory:

```bash
cd ../hive-api
export DASHBOARD_ASSETS_DIR="$(pwd)/../hive-dashboard/dist"
go run ./cmd/server
```

For production, copy `hive-dashboard/dist` to a stable path on the same host or inside the same container as `hive-api`, then set `DASHBOARD_ASSETS_DIR` to that path.

## Routing and authentication expectations

- Dashboard browser routes live under `/dashboard` and `/dashboard/*`.
- Unknown API routes under `/auth`, `/memories`, `/sync`, `/admin`, or `/health` return JSON 404 responses; they are not captured by the dashboard SPA shell.
- The dashboard uses existing Hive API auth/admin endpoints and stores the current browser JWT in `sessionStorage`.
- Production deployments should terminate TLS at the reverse proxy before forwarding traffic to `hive-api`.

## Explicit scope boundary

The dashboard observes and administers an active Hive API deployment. It does **not** start, stop, configure, monitor, or otherwise manage local `hive-daemon` processes.

GoReleaser currently packages the CLI and daemon binaries only. Dashboard delivery is handled by the Hive API container/deployment flow.
