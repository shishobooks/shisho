# Configuration

Shisho loads configuration at startup. Restart the container or server after changing any option.

## Configuration Sources

Values are applied in this order, with later sources taking precedence:

1. Built-in defaults
2. A YAML config file
3. Environment variables

Shisho looks for `/config/shisho.yaml` by default. Set the bootstrap environment variable `CONFIG_FILE` to use another file. A complete example is available at [`shisho.example.yaml`](https://github.com/shishobooks/shisho/blob/master/shisho.example.yaml).

Every setting below can also be provided as an unprefixed environment variable using its uppercase, underscored name. For example, `database_file_path` becomes `DATABASE_FILE_PATH`. Do not add a `SHISHO_` prefix.

Environment variables override values from the YAML file. Keep secrets such as `JWT_SECRET` out of source control.

## Settings

### Database

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `database_file_path` | `DATABASE_FILE_PATH` | `/config/shisho.db` | Path to the SQLite database file. Optional because the default is applied automatically |
| `database_debug` | `DATABASE_DEBUG` | `false` | Enable SQL query logging for debugging |
| `database_connect_retry_count` | `DATABASE_CONNECT_RETRY_COUNT` | `5` | Number of connection retry attempts on startup |
| `database_connect_retry_delay` | `DATABASE_CONNECT_RETRY_DELAY` | `2s` | Delay between connection retry attempts |
| `database_busy_timeout` | `DATABASE_BUSY_TIMEOUT` | `5s` | How long to wait when the database is locked |
| `database_max_retries` | `DATABASE_MAX_RETRIES` | `5` | Maximum retries for database operations that report busy or locked errors |

Keep the database on persistent storage. The standard image layout persists it through the `/config` mount. See [Deployment and Maintenance](./deployment-and-maintenance.md#back-up-shisho) before moving or backing up the database.

### Server

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `server_host` | `SERVER_HOST` | `0.0.0.0` | Reserved for listener configuration. The current server binds all interfaces and does not apply this value |
| `server_port` | `SERVER_PORT` | `3689` | Internal backend port. The production image publishes its Caddy frontend on container port `5173` |

The stock production image expects the backend on port `3689`. Changing `SERVER_PORT` without also rebuilding the image's entrypoint and Caddy configuration prevents the image from starting correctly. Publish container port `5173`, not backend port `3689`, for normal Docker deployments.

### Application

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `sync_interval_minutes` | `SYNC_INTERVAL_MINUTES` | `60` | How often to scan libraries for new content, in minutes. Set to `0` to disable scheduled scans |
| `worker_processes` | `WORKER_PROCESSES` | `2` | Number of background worker processes |
| `job_retention_days` | `JOB_RETENTION_DAYS` | `30` | Days to retain completed and failed jobs before cleanup. Set to `0` to disable cleanup |

### Library Monitor

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `library_monitor_enabled` | `LIBRARY_MONITOR_ENABLED` | `true` | Enable real-time filesystem monitoring and targeted rescans. Disable it for filesystems that do not reliably support inotify or FSEvents |
| `library_monitor_delay_seconds` | `LIBRARY_MONITOR_DELAY_SECONDS` | `60` | Seconds to wait after a filesystem change. Additional changes reset the timer so rapid changes are processed together |

:::tip[Linux inotify Watch Limits]
On Linux, including Linux Docker hosts, filesystem monitoring uses the host's inotify limits. Large libraries with many directories may exceed a low `fs.inotify.max_user_watches` value.

Check and temporarily increase the host limit with:

```bash
sysctl fs.inotify.max_user_watches
sudo sysctl -w fs.inotify.max_user_watches=524288
```

To persist the value on the Linux host:

```bash
printf 'fs.inotify.max_user_watches=524288\n' | sudo tee /etc/sysctl.d/99-inotify.conf
sudo sysctl --system
```
:::

### Cache

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `cache_dir` | `CACHE_DIR` | `/config/cache` | Directory for generated downloads, extracted CBZ pages, and rendered PDF pages |
| `download_cache_max_size_gb` | `DOWNLOAD_CACHE_MAX_SIZE_GB` | `5` | Maximum download cache size in GB. Older files are removed using least-recently-used eviction when the limit is exceeded |
| `pdf_render_dpi` | `PDF_RENDER_DPI` | `200` | PDF viewer render resolution. Range: 72 to 600. Higher values produce sharper and larger images |
| `pdf_render_quality` | `PDF_RENDER_QUALITY` | `85` | JPEG quality for rendered PDF pages. Range: 1 to 100 |

For cache inspection and clearing, see [Deployment and Maintenance](./deployment-and-maintenance.md#maintain-server-caches).

### Plugins

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `plugin_dir` | `PLUGIN_DIR` | `/config/plugins/installed` | Directory where installed [plugins](./plugins/overview) are stored |
| `plugin_data_dir` | `PLUGIN_DATA_DIR` | `/config/plugins/data` | Directory for persistent plugin caches, tokens, and database files. Data survives plugin updates and normal uninstalls |

### Enrichment

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `enrichment_confidence_threshold` | `ENRICHMENT_CONFIDENCE_THRESHOLD` | `0.85` | Confidence threshold from 0 to 1 for automatic metadata enrichment during scans. Results below it are skipped. Per-plugin thresholds take precedence |

### Supplement Discovery

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `supplement_exclude_patterns` | `SUPPLEMENT_EXCLUDE_PATTERNS` | `[".*", ".DS_Store", "Thumbs.db", "desktop.ini"]` | Glob patterns excluded from [supplement file](./supplement-files.md) discovery. The environment variable accepts comma-separated values |
| `pdf_supplement_filenames` | `PDF_SUPPLEMENT_FILENAMES` | See below | Case-insensitive exact PDF basenames, without extensions, used for [PDF auto-demotion](./supplement-files.md#pdf-auto-demotion) when a non-PDF main file or existing book is present in the same book directory. A root-level PDF remains a main file. The environment variable accepts comma-separated values. Set an empty list in YAML to disable |

The default `pdf_supplement_filenames` list is:

```text
supplement
supplemental
bonus
bonus material
bonus content
companion
notes
liner notes
errata
booklet
digital booklet
appendix
map
maps
insert
guide
reference
cheat sheet
cheatsheet
cribsheet
pamphlet
extras
```

See [Supplement Files](./supplement-files.md) for the exact discovery and classification rules.

### Authentication

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `jwt_secret` | `JWT_SECRET` | None | Required secret for signing authentication tokens. Use a long random value of at least 32 characters, for example from `openssl rand -hex 32` |
| `session_duration_days` | `SESSION_DURATION_DAYS` | `30` | Server-wide number of days a login session remains valid before re-authentication |

Changing `JWT_SECRET` invalidates current sessions. Session duration is global, not configurable per user. See [Users and Permissions](./users-and-permissions.md).

## Container-Only Environment Variables

:::info
The variables in this section configure the production container wrapper or its bundled Caddy server. They are not YAML fields in Shisho's application configuration.
:::

| Env Variable | Image Default | Description |
|--------------|---------------|-------------|
| `CADDY_ACCESS_LOG_OUTPUT` | `discard` | Caddy access log output. Set to `stdout` to enable access logs |
| `PUID` | `1000` | User ID selected for Shisho and Caddy processes inside the container. This does not grant host filesystem access |
| `PGID` | `1000` | Group ID selected for Shisho and Caddy processes inside the container. This does not grant host filesystem access |
| `STARTUP_TIMEOUT_SECONDS` | `120` | Seconds the entrypoint waits for the internal backend health endpoint before exiting |
| `LOG_FORMAT` | `json` | Log format for the image entrypoint, backend, and Caddy. Use `console` for human-readable output |

The image creates and changes ownership of `/config`, but it does not create or change ownership of `/data`, `/media`, or custom paths. Configure host permissions for the selected `PUID` and `PGID`.
