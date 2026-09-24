# Deployment and Maintenance

This page covers operating a Docker deployment after the initial [Getting Started](./getting-started.md) setup.

The image runs one Shisho process that serves both the web interface and API on container port `5173`. It does not require a separate web server inside the container. The entrypoint prepares `/config` ownership and starts Shisho as `PUID` and `PGID`; Shisho handles shutdown directly.

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

The image emits JSON logs by default. Set `LOG_FORMAT=console` for human-readable output. Successful frontend asset requests are omitted from request logs; application API, integration, and error responses are logged. The **Settings > Logs** page shows recent application logs to users with Config Read permission, but container logs remain important for startup failures and reverse proxy issues.

## HTTPS and Reverse Proxies

:::warning[Protect the HTTP Origin]
Shisho trusts forwarded headers from private-network peers. A client that can reach the origin through such a peer can influence generated URLs and secure-cookie handling if the proxy passes through client-supplied headers. Restrict origin access to your trusted proxy and have it overwrite forwarded headers before exposing the service. Keep direct access on a trusted local network if you do not need public access.
:::

Terminate HTTPS at a trusted reverse proxy and proxy to Shisho's container port `5173`. Preserve the original `Host` and supply `X-Forwarded-Proto` for the public scheme. Kobo also uses `X-Forwarded-Host` and `X-Forwarded-Port` when provided.

Shisho accepts `X-Forwarded-Proto`, `X-Forwarded-Host`, `X-Forwarded-Port`, `X-Forwarded-Prefix`, and `X-Forwarded-For` only when the direct TCP peer is loopback, link-local, an RFC 1918 IPv4 address, or an IPv6 unique-local address in `fc00::/7`. It strips these headers from public-address peers. Trust depends on the direct connection, not an address claimed in `X-Forwarded-For`. Connect a remote proxy over a private network if its public address would otherwise reach Shisho directly.

Shisho currently must be served from the origin root, such as `https://books.example.com/`. Deploying it below a path prefix such as `https://example.com/shisho/` is not supported.

The reverse proxy must pass all of these public route families without rewriting them to the frontend:

- `/api/*` for the application API and event streams
- `/opds/*` for OPDS feeds
- `/kobo/*` for Kobo sync
- `/ereader/*` for the eReader browser
- `/e/*` for short eReader setup URLs

Do not strip `/api` before forwarding it. Also pass `/health` if your external monitor uses it. Avoid response buffering for streaming API responses. The browser interface and API must share an origin; Shisho does not send CORS permission headers for cross-origin browser access.

Review authentication for [OPDS](./opds.md), Kobo, and eReader links before publishing them. Treat device URLs and API keys as secrets.

### Response Handling

Shisho uses gzip for eligible responses of at least 1 KiB when the client accepts it. Event streams, covers, page images, and file downloads bypass gzip. Keep proxy buffering disabled for event streams so live updates arrive promptly.

Hashed frontend files under `/assets/` carry `Cache-Control: public, max-age=31536000, immutable`. The frontend's `index.html` uses `Cache-Control: no-cache` so browsers revalidate it. Do not override these with a blanket proxy cache rule. Unknown paths under `/api`, `/opds`, `/kobo`, `/ereader`, and `/e` return JSON errors rather than the frontend page; other unknown paths open the frontend.

Responses include `X-Frame-Options: SAMEORIGIN`, `X-Content-Type-Options: nosniff`, and `Referrer-Policy: strict-origin-when-cross-origin`. Shisho does not emit a `Server` header or the deprecated `X-XSS-Protection` header. A reverse proxy may add its own headers, so check the public response when auditing the deployment.

## Outbound Connections

Core local library management does not require sending your library to a hosted Shisho service. Features you explicitly use can make outbound connections, including plugin repository access, plugin installation and updates, metadata or cover providers configured through plugins, Audible chapter lookup, and Kobo integration. Plugins can implement their own network behavior, so review a plugin and its configuration before enabling it.

## Maintain Server Caches

Generated downloads, extracted CBZ pages, and rendered PDF pages live under `CACHE_DIR`, which defaults to `/config/cache`. The download cache is automatically limited by `DOWNLOAD_CACHE_MAX_SIZE_GB`; viewer caches can still grow with use.

:::warning[Check the Cache Before Clearing]
Clearing a cache permanently deletes its generated files, but does not delete the library database or source media. Verify that you selected the intended cache. The content can be regenerated, although the next affected download or reader request may be slower.
:::

Users with Config Read permission can open **Settings > Cache** to inspect each cache. Config Write permission is required to select **Clear** and reclaim space or regenerate output. See [Configuration](./configuration.md#cache) for cache limits and rendering options.
