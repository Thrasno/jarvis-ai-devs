# Hive API Operator Guide

Hive API is the shared team memory backend. Operate it as a server-side service with PostgreSQL, strong secrets, HTTPS termination, and optional dashboard assets.

## Quick path

1. Prepare PostgreSQL and deployment secrets.
2. Configure `hive-api` environment variables.
3. Run the API behind a reverse proxy with HTTPS.
4. Enable the dashboard only when compiled assets are available and intended.
5. Confirm health, authentication, and memory/sync behavior.

## Core settings

| Setting | Required | Purpose |
|---------|----------|---------|
| `DATABASE_URL` | Yes | PostgreSQL connection string. |
| `JWT_SECRET` | Yes | Signs sessions; must be at least 32 characters. |
| `PORT` | No | API port. |
| `GIN_MODE` | No | Use `release` for production. |
| `CORS_ALLOWED_ORIGINS` | No | Browser origins allowed to call the API. |
| `DASHBOARD_ASSETS_DIR` | Dashboard only | Directory with compiled dashboard `index.html` and assets. |

## Expected deployment shape

```text
Users / local daemons
        |
        v
HTTPS reverse proxy
        |
        v
hive-api ---- PostgreSQL
   |
   +-- /dashboard when enabled/configured
```

## Operator responsibilities

- Keep secrets out of the repository.
- Rotate `JWT_SECRET` and credentials through the deployment process.
- Ensure production traffic uses HTTPS.
- Monitor API health and database availability.
- Decide which projects are allowed to share memory.
- Document whether dashboard access is enabled and who can use admin features.

## Dashboard enablement

If `DASHBOARD_ASSETS_DIR` is empty, dashboard routes are not registered and API routes continue normally. If enabled, the directory must contain a readable `index.html` from the built `hive-dashboard` app.

## Checklist

- [ ] `DATABASE_URL` points to the intended PostgreSQL instance.
- [ ] `JWT_SECRET` is strong and stored outside git.
- [ ] Reverse proxy terminates HTTPS.
- [ ] Dashboard assets are built and configured only when intended.
- [ ] Operational logs do not expose credentials.

## Next step

For dashboard-specific behavior, read [`dashboard-guide.md`](dashboard-guide.md) and [`../hive-api-dashboard.md`](../hive-api-dashboard.md).
