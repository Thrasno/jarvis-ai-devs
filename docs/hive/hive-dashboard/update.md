# Deploying Dashboard Changes to Production

This guide covers how to push new dashboard code to a VPS that is already running the production stack.

For first-time setup, see [deployment.md](deployment.md) instead.

## When to use this guide

Use this guide whenever dashboard frontend code changes — TypeScript, CSS, components, routes, or any file under `hive-dashboard/` — need to go live on the VPS.

Do not use this guide to set up DNS, configure the reverse proxy, create `.env` secrets, or provision the server for the first time. Those steps are one-time operations covered in `deployment.md`.

## Why `--build` is always required

Dashboard assets are compiled inside the Docker image at build time. When `docker compose --build` runs, the `dashboard-builder` stage (Node 22) compiles the Vite/TypeScript frontend and the resulting static files are embedded in the final image.

At runtime, there is no Vite server and no mounted source directory. The container serves whatever was baked in during the build.

Running `docker compose up -d` without `--build` restarts the existing image unchanged. New dashboard code in the repository has no effect. Always pass `--build` when deploying dashboard changes.

## Pre-flight checklist

Before starting:

- [ ] Changes are merged to `master` on the remote.
- [ ] You have SSH access to the VPS.
- [ ] No critical operation is in progress on the dashboard (migrations, active user sessions during a disruptive change).

## Update procedure

Run these commands from the VPS, in order.

### 1. Pull latest master

```bash
cd /opt/hive-api
git fetch origin master
git switch master
git pull --ff-only origin master
```

Confirm you are on master and the pull succeeded:

```bash
git log --oneline -3
```

### 2. Rebuild and restart

```bash
cd /opt/hive-api/hive-api/deploy
docker compose -f docker-compose.prod.yml up -d --build
```

This rebuilds the image with the new dashboard assets and replaces the running container.

### 3. Verify containers

```bash
docker compose -f docker-compose.prod.yml ps
```

Expected result:

- `postgres` is healthy.
- `api` is running.

### 4. Verify the dashboard

```bash
curl -i https://hivemem.dev/dashboard/
```

Expected result:

- HTTP `200`.
- `Content-Type` compatible with `text/html`.

Do not use `curl -I` for this check. `curl -I` sends a `HEAD` request, while the dashboard routes are registered for browser-style `GET` requests; a `HEAD` check can return `404` even when the dashboard is working.

Then open `https://hivemem.dev/dashboard/` in a browser and confirm the updated UI loads correctly.

## What you do not need to redo

A code update does not change any infrastructure configuration. The following require no action unless the infrastructure itself changed:

| Item | Reason |
|------|--------|
| DNS records | Not affected by code changes. |
| Reverse proxy (Caddy or Nginx) | Configuration unchanged. |
| TLS certificates | Managed independently of the application. |
| `.env` secrets | Not modified during a code update. |
| PostgreSQL data | Persisted in a named Docker volume; survives container replacement. |

Only touch these if the deployment itself introduced a new environment variable, a new port, or a configuration change.

## Downtime expectation

Compose stops the old container before starting the rebuilt one. Expect a few seconds of downtime during the transition. This is normal behavior for a single-instance deployment without a load balancer or blue/green setup.

## Rollback

If the new build causes an issue, revert to the previous working commit and rebuild.

### Find the previous working commit

```bash
cd /opt/hive-api
git log --oneline -10
```

### Check out that commit and rebuild

```bash
git checkout <previous-commit-hash>
cd hive-api/deploy
docker compose -f docker-compose.prod.yml up -d --build
```

After confirming the rollback is stable, open a PR to fix the issue in master before deploying again. Do not leave the VPS on a detached commit for longer than necessary.

## Checking logs after the update

```bash
cd /opt/hive-api/hive-api/deploy
docker compose -f docker-compose.prod.yml logs api --tail=50
```

Look for startup errors, failed route registrations, or asset path issues in the log output.

## Common mistakes

| Mistake | Consequence |
|---------|-------------|
| Running `docker compose up -d` without `--build` | Old compiled assets remain in the image; the updated UI does not appear. |
| Deploying from a branch that is not master | Production runs an untested or incomplete version; `git branch` before deploying. |
| Running `docker compose` from the wrong directory | Compose cannot find `docker-compose.prod.yml`; always run from `/opt/hive-api/hive-api/deploy` or pass the full `-f` path. |
| Forgetting `git pull` before `--build` | The build compiles the previous version of the code already on disk. |
