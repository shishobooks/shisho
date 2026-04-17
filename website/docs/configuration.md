---
sidebar_position: 3
---

# Configuration

Shisho can be configured via a YAML config file or environment variables.

## Config File

By default, Shisho looks for a config file at `/config/shisho.yaml`. You can override this by setting the `CONFIG_FILE` environment variable to a different path.

A complete example config file is available at [`shisho.example.yaml`](https://github.com/shishobooks/shisho/blob/master/shisho.example.yaml).

## Environment Variables

Every setting can be set via an environment variable using the uppercase, underscored version of the config key (e.g., `database_file_path` becomes `DATABASE_FILE_PATH`). Environment variables take precedence over config file values.

## Settings

### Database

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `database_file_path` | `DATABASE_FILE_PATH` | `/config/shisho.db` | **Required.** Path to the SQLite database file |
| `database_debug` | `DATABASE_DEBUG` | `false` | Enable SQL query logging for debugging |
| `database_connect_retry_count` | `DATABASE_CONNECT_RETRY_COUNT` | `5` | Number of connection retry attempts on startup |
| `database_connect_retry_delay` | `DATABASE_CONNECT_RETRY_DELAY` | `2s` | Delay between connection retry attempts |
| `database_busy_timeout` | `DATABASE_BUSY_TIMEOUT` | `5s` | How long to wait when the database is locked |
| `database_max_retries` | `DATABASE_MAX_RETRIES` | `5` | Max retries for database operations on busy/locked errors |

### Server

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `server_host` | `SERVER_HOST` | `0.0.0.0` | Host address to bind the server to |
| `server_port` | `SERVER_PORT` | `3689` | Port to listen on |

### Application

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `sync_interval_minutes` | `SYNC_INTERVAL_MINUTES` | `60` | How often to scan libraries for new content (in minutes) |
| `worker_processes` | `WORKER_PROCESSES` | `2` | Number of background worker processes |
| `job_retention_days` | `JOB_RETENTION_DAYS` | `30` | Days to retain completed/failed jobs before cleanup. Set to `0` to disable |

### Library Monitor

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `library_monitor_enabled` | `LIBRARY_MONITOR_ENABLED` | `true` | Enable real-time filesystem monitoring of library paths. When enabled, file changes are detected automatically and trigger targeted rescans. Disable if your library is on a network drive that doesn't support inotify/FSEvents. On Linux, large libraries may require increasing `fs.inotify.max_user_watches` (see below) |
| `library_monitor_delay_seconds` | `LIBRARY_MONITOR_DELAY_SECONDS` | `60` | Seconds to wait before processing detected changes. Additional changes during this window reset the timer, batching rapid changes into a single rescan |

:::tip[Linux inotify watch limits]
On Linux, the monitor uses inotify which has a per-user watch limit (default 8192 on some distros). Large libraries with many subdirectories may exceed this. To increase it:

```bash
# Temporary (until reboot)
sudo sysctl -w fs.inotify.max_user_watches=524288

# Permanent
echo "fs.inotify.max_user_watches=524288" | sudo tee /etc/sysctl.d/99-inotify.conf
sudo sysctl --system
```

macOS (FSEvents) and Docker containers typically don't need this adjustment.
:::

### Cache

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `cache_dir` | `CACHE_DIR` | `/config/cache` | Directory for caching generated files (downloads, extracted CBZ pages, and rendered PDF pages) |
| `download_cache_max_size_gb` | `DOWNLOAD_CACHE_MAX_SIZE_GB` | `5` | Maximum size of the download cache in GB. Older files are removed automatically (LRU) when the limit is exceeded |
| `pdf_render_dpi` | `PDF_RENDER_DPI` | `200` | DPI for rendering PDF pages in the viewer. Higher values produce sharper images but larger files. Range: 72-600 |
| `pdf_render_quality` | `PDF_RENDER_QUALITY` | `85` | JPEG quality for rendered PDF pages (1-100). Higher values produce better quality but larger files |

### Plugins

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `plugin_dir` | `PLUGIN_DIR` | `/config/plugins/installed` | Directory where installed [plugins](./plugins/overview) are stored |
| `plugin_data_dir` | `PLUGIN_DATA_DIR` | `/config/plugins/data` | Directory where plugin persistent data is stored (caches, tokens, DB files). Data survives plugin updates; optionally deleted on uninstall with `delete_data=true` |

### Enrichment

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `enrichment_confidence_threshold` | `ENRICHMENT_CONFIDENCE_THRESHOLD` | `0.85` | Confidence threshold (0-1) for automatic metadata enrichment during scans. When a plugin returns a confidence score, results below this threshold are skipped. Per-plugin thresholds override this value. |

### Supplement Discovery

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `supplement_exclude_patterns` | `SUPPLEMENT_EXCLUDE_PATTERNS` | `[".*", ".DS_Store", "Thumbs.db", "desktop.ini"]` | Glob patterns to exclude from [supplement file](./supplement-files) discovery. Env var accepts comma-separated values |

### Docker / Caddy

These environment variables are only relevant when running Shisho in Docker, where Caddy serves as the reverse proxy.

| Env Variable | Default | Description |
|-------------|---------|-------------|
| `CADDY_ACCESS_LOG_OUTPUT` | `discard` | Caddy access log output. Set to `stdout` to enable access logs. Logs are disabled by default to reduce noise |
| `PUID` | `1000` | User ID for the Shisho process inside the container |
| `PGID` | `1000` | Group ID for the Shisho process inside the container |
| `STARTUP_TIMEOUT_SECONDS` | `120` | Seconds to wait for the backend to start before giving up. Increase for slow storage (e.g., NAS devices) |
| `LOG_FORMAT` | `console` | Log output format. Set to `json` for structured JSON logs (useful for log aggregation) |

### Authentication

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `jwt_secret` | `JWT_SECRET` | - | **Required.** Secret key for signing JWT authentication tokens. Use a long, random string (at least 32 characters). Generate one with: `openssl rand -base64 32` |
| `session_duration_days` | `SESSION_DURATION_DAYS` | `30` | How many days a login session remains valid before requiring re-authentication |
