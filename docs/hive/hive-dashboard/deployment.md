# Deploy the Hive Dashboard on a VPS

This guide explains how to deploy the Hive Dashboard on the same VPS that already runs `hive-api`.

The production URL should be:

```text
https://hivemem.dev/dashboard/
```

Use the trailing slash. It avoids avoidable frontend asset and route issues.

## What you are deploying

The dashboard is not a separate server process. It is a static frontend built from `hive-dashboard/` and served by `hive-api` under `/dashboard/`.

In production:

| Component | Role |
|-----------|------|
| `postgres` | Stores Hive API data. |
| `api` | Runs `hive-api` and serves `/dashboard/`. |
| Reverse proxy | Terminates HTTPS and forwards traffic to `127.0.0.1:8080`. |

The production container does not run Vite or a Node server. Node is used only during the Docker build to produce static dashboard files.

## Recommended production files

Use the production Compose file:

```text
hive-api/deploy/docker-compose.prod.yml
```

Use the example environment file as a template:

```text
hive-api/deploy/.env.prod.example
```

On the VPS, copy it to `.env` and replace every value:

```bash
cd /opt/hive-api/hive-api/deploy
cp .env.prod.example .env
```

Never commit the real `.env` file.

## 1. Prepare DNS

Point `hivemem.dev` to the VPS public IP.

Check it from your local machine:

```bash
dig hivemem.dev
```

The returned IP should match the VPS.

## 2. Prepare secrets

Edit the server `.env` file:

```bash
cd /opt/hive-api/hive-api/deploy
nano .env
```

Set at least:

```env
POSTGRES_USER=hive
POSTGRES_PASSWORD=replace-with-a-real-password
POSTGRES_DB=hivedb
DATABASE_URL=postgres://hive:replace-with-a-real-password@postgres:5432/hivedb?sslmode=disable
JWT_SECRET=replace-with-a-real-secret
PORT=8080
GIN_MODE=release
CORS_ALLOWED_ORIGINS=https://hivemem.dev
```

If the database password contains special URL characters, encode them in `DATABASE_URL`. Pay special attention to `@`, `:`, `/`, `?`, `#`, and `%`.

Generate a strong `JWT_SECRET` with:

```bash
openssl rand -base64 48
```

## 3. Pull the latest code

On the VPS:

```bash
cd /opt/hive-api
git fetch origin master
git switch master
git pull --ff-only origin master
```

This updates the repository to the version that contains the dashboard and production Compose file.

## 4. Validate Compose before starting

```bash
cd /opt/hive-api/hive-api/deploy
docker compose -f docker-compose.prod.yml config > /tmp/hive-compose-prod.yml
```

If this command exits successfully, Compose can read the file and interpolate the `.env` values.

## 5. Build and restart

```bash
cd /opt/hive-api/hive-api/deploy
docker compose -f docker-compose.prod.yml up -d --build
```

This builds `hive-api`, builds the dashboard assets, and starts the containers.

## 6. Verify containers

```bash
docker compose -f docker-compose.prod.yml ps
```

Expected result:

- `postgres` is healthy.
- `api` is running.
- `api` exposes `127.0.0.1:8080->8080/tcp`, not `0.0.0.0:8080`.

## 7. Configure HTTPS reverse proxy

The public domain should point to the reverse proxy, and the proxy should forward requests to `127.0.0.1:8080`.

### Caddy example

```caddyfile
hivemem.dev {
    reverse_proxy 127.0.0.1:8080
}
```

### Nginx example

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

After Nginx is configured, add TLS with Certbot or your normal certificate process.

## 8. Verify the dashboard

```bash
curl -I https://hivemem.dev/dashboard/
```

Expected result:

- HTTP `200`.
- `Content-Type` compatible with `text/html`.

Then open:

```text
https://hivemem.dev/dashboard/
```

Expected result:

- Login screen loads.
- JavaScript and CSS assets load without `404`.
- Authenticated dashboard views load after login.

## Troubleshooting

### `/dashboard/` returns 404

Rebuild the production Compose stack:

```bash
docker compose -f docker-compose.prod.yml up -d --build
```

Then check logs:

```bash
docker compose -f docker-compose.prod.yml logs api
```

### CSS or JavaScript returns 404

Use the URL with trailing slash:

```text
https://hivemem.dev/dashboard/
```

Also confirm the reverse proxy does not strip `/dashboard` from the forwarded path.

### Compose fails before starting

Run:

```bash
docker compose -f docker-compose.prod.yml config
```

Most failures here mean a required `.env` variable is missing.

## Follow-up

Issue #70 tracks dashboard development dependency audit remediation. It does not block this production deployment because the runtime image serves compiled static files and does not run Node.
