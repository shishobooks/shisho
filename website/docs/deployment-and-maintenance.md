# Deployment and Maintenance

This page covers operating a Docker deployment after the initial [Getting Started](./getting-started.md) setup.

## Choose an Image Tag

Published images support Linux AMD64 and ARM64.

- A pinned release tag, such as `ghcr.io/shishobooks/shisho:0.0.49`, keeps deployments reproducible and changes only when you edit the tag.
- `ghcr.io/shishobooks/shisho:latest` follows the newest release and is convenient for evaluation, but a future pull may introduce application and database changes.

Use a pinned tag for installations where you want to review and schedule each update.

## Persistent State

With the default container paths, persist `/config`. It contains:

- The SQLite database at `/config/shisho.db`
- Server caches under `/config/cache`
- Installed plugins and plugin data under `/config/plugins`
- `/config/shisho.yaml`, if you use a config file

Also protect these deployment-specific resources:

- The deployment secret containing `JWT_SECRET`
- Writable media, including Shisho-generated covers and sidecars
- A custom database path if `DATABASE_FILE_PATH` points outside `/config`
- Custom cache or plugin directories if they point outside `/config`

The image creates `/config` and adjusts its ownership for `PUID` and `PGID`. It does not create or adjust ownership for `/data`, `/media`, or other custom mounts.

## Back Up Shisho

Stop the container before making a filesystem copy of the database. SQLite may use write-ahead logging, so copying only `shisho.db` while Shisho is running can omit committed data from the `-wal` file or capture mismatched files.

For the bind-mount layout from Getting Started:

```bash
docker compose stop shisho
tar -C . -czf "shisho-config-$(date +%Y%m%d-%H%M%S).tar.gz" config
docker compose start shisho
```

Store the archive and the JWT secret securely. If the database, cache, plugin data, or media uses another host path or volume, back it up separately while the container is stopped. Media backups matter because scans and metadata edits can write sidecars and covers, and file organization can move or rename content.

Test backups by restoring them in an isolated environment. An untested archive is not a recovery plan.

## Update Shisho Safely

:::warning[Back Up Before Updating]
Shisho automatically applies forward database migrations during startup. Database downgrades are not guaranteed, so changing the image tag back may not make a database migrated by a newer release compatible with an older release.

Take and test a stopped-container backup before updating. Recovery may require restoring that pre-update backup as well as the corresponding application version.
:::

1. Read the release notes and choose the target tag.
2. Stop Shisho and take a backup of `/config` and any custom state paths.
3. Update the image tag in `docker-compose.yml`.
4. Pull and recreate the container.
5. Inspect logs, health, and normal library access.

```bash
docker compose stop shisho
# Back up persistent state here.
docker compose pull shisho
docker compose up -d
docker compose logs -f shisho
```

Startup applies migrations before the web frontend is served.

## Restore a Backup

:::warning[Restore a Consistent Backup]
Do not restore only `shisho.db` from a live filesystem copy, and do not use a restore to downgrade a newer database in place. Restore a complete, consistent stopped-container backup and select an application version compatible with it. Keep the current state until the restored installation has been verified.
:::

1. Stop Shisho.
2. Move the current state aside rather than deleting it immediately.
3. Restore the complete `/config` backup, plus any custom database, plugin, cache, media, and secret paths.
4. Select an application version compatible with the restored database.
5. Start Shisho and inspect migration and startup logs.
6. Verify login, libraries, and representative files before removing the previous state.

## Health Checks

The image exposes:

```text
GET /health
```

Use it as a liveness signal that the HTTP service is responding:

```bash
curl --fail http://localhost:5173/health
```

The endpoint is not a database readiness or integrity check. It does not replace checking startup logs, confirming migrations completed, logging in, or reading representative library data.

## Logs

Follow container output with:

```bash
docker compose logs -f shisho
```

The image emits JSON logs by default. Set `CADDY_ACCESS_LOG_OUTPUT=stdout` if you also need Caddy access logs. The **Settings > Logs** page shows recent application logs to users with Config Read permission, but container logs remain important for startup failures and reverse proxy issues.

## HTTPS and Reverse Proxies

Do not expose an unauthenticated HTTP origin to the public internet. Terminate HTTPS at a trusted reverse proxy and proxy to Shisho's container port `5173`. Preserve the original host and scheme with standard forwarded headers.

Shisho currently must be served from the origin root, such as `https://books.example.com/`. Deploying it below a path prefix such as `https://example.com/shisho/` is not supported.

The reverse proxy must pass all of these public route families without rewriting them to the frontend:

- `/api/*` for the application API and event streams
- `/opds/*` for OPDS feeds
- `/kobo/*` for Kobo sync
- `/ereader/*` for the eReader browser
- `/e/*` for short eReader setup URLs

Also pass `/health` if your external monitor uses it. Avoid response buffering for streaming API responses.

Review authentication for [OPDS](./opds.md), Kobo, and eReader links before publishing them. Treat device URLs and API keys as secrets.

## Outbound Connections

Core local library management does not require sending your library to a hosted Shisho service. Features you explicitly use can make outbound connections, including plugin repository access, plugin installation and updates, metadata or cover providers configured through plugins, Audible chapter lookup, and Kobo integration. Plugins can implement their own network behavior, so review a plugin and its configuration before enabling it.

## Maintain Server Caches

Generated downloads, extracted CBZ pages, and rendered PDF pages live under `CACHE_DIR`, which defaults to `/config/cache`. The download cache is automatically limited by `DOWNLOAD_CACHE_MAX_SIZE_GB`; viewer caches can still grow with use.

:::warning[Check the Cache Before Clearing]
Clearing a cache permanently deletes its generated files, but does not delete the library database or source media. Verify that you selected the intended cache. The content can be regenerated, although the next affected download or reader request may be slower.
:::

Users with Config Read permission can open **Settings > Cache** to inspect each cache. Config Write permission is required to select **Clear** and reclaim space or regenerate output. See [Configuration](./configuration.md#cache) for cache limits and rendering options.
