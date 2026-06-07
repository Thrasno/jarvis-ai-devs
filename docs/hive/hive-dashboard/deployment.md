# Deploy the Hive Dashboard

This guide explains how to publish the Hive Dashboard on the same server as `hive-api`, using a public URL such as `https://hivemem.dev/dashboard/`.

The dashboard is not a separate backend service. It is a static web app built from `hive-dashboard/` and served by `hive-api` under `/dashboard/`.

## Quick path

1. Point the domain to the server that runs `hive-api`.
2. Configure `hive-api` with a strong `JWT_SECRET`.
3. Build/deploy the `hive-api` container, which also builds the dashboard assets.
4. Expose the app through HTTPS at `https://hivemem.dev/dashboard/`.
5. Open the dashboard and sign in with the same credentials/token flow used by `hive-api`.

Use the trailing slash in documentation and bookmarks:

```text
https://hivemem.dev/dashboard/
```

`https://hivemem.dev/dashboard` may redirect, but the trailing slash avoids avoidable routing problems with frontend assets.

## What runs where

| Piece | Where it runs | Notes |
|-------|---------------|-------|
| `hive-api` | Server | Serves the API and the dashboard static files. |
| Hive Dashboard | Built into static files | No separate Node process is needed in production. |
| PostgreSQL | Server or managed database | Used by `hive-api`, not by the dashboard directly. |
| Reverse proxy | Server edge | Terminates HTTPS and forwards traffic to `hive-api`. |

The production dashboard does **not** run `vite`, `npm`, or a Node server. Those tools are only used while building the static files.

## Prerequisites

Before deploying, confirm you have:

- A Linux server where `hive-api` can run.
- A domain or subdomain, for example `hivemem.dev`.
- DNS access for that domain.
- Docker and Docker Compose installed on the server.
- A production `JWT_SECRET` value.
- Database connection settings for `hive-api`.

## 1. Point DNS to the server

Create or update an `A` record:

| DNS field | Value |
|-----------|-------|
| Name | `hivemem.dev` or the selected subdomain |
| Type | `A` |
| Value | Public IP address of the server |

If you use IPv6, also add an `AAAA` record.

After saving DNS, wait for propagation. You can check from your machine:

```bash
dig hivemem.dev
```

The returned IP should be the server public IP.

## 2. Configure environment variables

On the server, keep secrets outside the Git repository. A common option is a deployment `.env` file next to the Compose file.

Example:

```env
JWT_SECRET=replace-with-a-long-random-production-secret
GIN_MODE=release

# Example only. Use the real values for the server/database.
DATABASE_URL=postgres://hive:replace-me@postgres:5432/hive?sslmode=disable
```

Important rules:

- `JWT_SECRET` is required. Do not use a public default.
- Use a long random value for `JWT_SECRET`.
- Do not commit the `.env` file.
- Keep database credentials private.

Generate a strong secret with:

```bash
openssl rand -base64 48
```

## 3. Build and start `hive-api`

From the repository root on the server, use the deployment files for `hive-api`.

Typical flow:

```bash
cd /opt/jarvis-dev
git fetch --all
git checkout master
git pull --ff-only
docker compose -f hive-api/deploy/docker-compose.yml up -d --build
```

What this should do:

1. Build the dashboard from `hive-dashboard/`.
2. Copy the compiled dashboard files into the `hive-api` runtime image.
3. Start `hive-api` with `DASHBOARD_ASSETS_DIR` pointing at those compiled files.
4. Serve the dashboard under `/dashboard/`.

If your deployment uses a different Compose project or custom paths, keep the same principle: the runtime `hive-api` process must know where the compiled dashboard assets are through `DASHBOARD_ASSETS_DIR`.

## 4. Put HTTPS in front of `hive-api`

Users should access the dashboard through HTTPS:

```text
https://hivemem.dev/dashboard/
```

The reverse proxy receives public traffic on ports `80` and `443`, then forwards it to the internal `hive-api` port.

### Option A: Caddy example

Caddy is the simplest option because it can manage TLS certificates automatically.

Example `Caddyfile`:

```caddyfile
hivemem.dev {
    reverse_proxy 127.0.0.1:8080
}
```

Use the actual local port where `hive-api` listens. If Compose maps `hive-api` to a different host port, replace `8080`.

### Option B: Nginx example

Example Nginx server block:

```nginx
server {
    listen 80;
    server_name hivemem.dev;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Then configure TLS with Certbot or your standard certificate process.

## 5. Verify the deployment

Run these checks from your local machine.

### API health

Use the health endpoint configured for `hive-api`:

```bash
curl -i https://hivemem.dev/health
```

Expected result:

- HTTP `200`, or the documented healthy response for `hive-api`.

### Dashboard HTML

```bash
curl -I https://hivemem.dev/dashboard/
```

Expected result:

- HTTP `200`.
- A content type compatible with HTML.

### Dashboard assets

Open the dashboard in a browser:

```text
https://hivemem.dev/dashboard/
```

Expected result:

- The login shell loads.
- No missing JavaScript or CSS files in the browser console.
- Login failures show a visible error instead of a blank page.
- After login, read-only dashboard views load using `hive-api` endpoints.

## Common problems

### `404` at `/dashboard/`

Likely causes:

- The dashboard assets were not built.
- `DASHBOARD_ASSETS_DIR` points to the wrong directory.
- The container image was not rebuilt after pulling the dashboard changes.

Try:

```bash
docker compose -f hive-api/deploy/docker-compose.yml up -d --build
```

### Dashboard loads, but JS/CSS files return `404`

Likely causes:

- The URL is missing the trailing slash.
- The reverse proxy rewrites paths incorrectly.
- The dashboard was built with an unexpected base path.

Use:

```text
https://hivemem.dev/dashboard/
```

Then verify the reverse proxy forwards the full path to `hive-api` without stripping `/dashboard`.

### Login works locally but not on the server

Likely causes:

- Wrong `JWT_SECRET` or the server was restarted with a different secret.
- API base URL/proxy headers are wrong.
- HTTPS is not configured correctly.

Check:

- `JWT_SECRET` is stable across restarts.
- The browser is using `https://hivemem.dev/dashboard/`.
- The reverse proxy sets `X-Forwarded-Proto`.

### Container starts locally but fails in production

Likely causes:

- Missing required environment variables.
- Database is not reachable from the container.
- File permissions prevent the runtime process from reading dashboard assets.

Check logs:

```bash
docker compose -f hive-api/deploy/docker-compose.yml logs hive-api
```

## Deployment checklist

- [ ] DNS points `hivemem.dev` to the server.
- [ ] HTTPS is configured.
- [ ] `JWT_SECRET` is set to a strong private value.
- [ ] Database settings are configured.
- [ ] `hive-api` container builds successfully.
- [ ] Dashboard assets are included in the runtime image.
- [ ] `https://hivemem.dev/dashboard/` returns the dashboard HTML.
- [ ] Login screen loads in a browser.
- [ ] Authenticated read-only views load.

## Follow-up security work

The dashboard feature can run in production without Node runtime dependencies. However, development/build tooling should still be kept healthy.

Track dependency audit remediation in:

```text
https://github.com/Thrasno/jarvis-ai-devs/issues/70
```

That follow-up should be done from a fresh branch or worktree based on `master`.
