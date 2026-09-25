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
| `server_host` | `SERVER_HOST` | `0.0.0.0` | Address to bind the HTTP listener to. Use `127.0.0.1` to accept only local connections outside Docker |
| `server_port` | `SERVER_PORT` | `3689` | HTTP port for both the web interface and API. The production image sets `SERVER_PORT=5173` |

The built-in port default stays `3689` for local development and non-container runs. The production image's `SERVER_PORT=5173` environment variable overrides a YAML `server_port` value. Publish container port `5173` for normal Docker deployments. To change only the host-facing port, change the left side of the Compose mapping, for example `8080:5173`.

Keep `SERVER_HOST=0.0.0.0` inside Docker so published ports can reach the listener. If you override the container's `SERVER_PORT`, update its port mapping and health check URL too; the image's health check targets port `5173`. See [Deployment and Maintenance](./deployment-and-maintenance.md#https-and-reverse-proxies) for reverse proxy routing and forwarded-header trust.

### Application

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `demo_mode` | `DEMO_MODE` | `false` | Present a prepared library without allowing API edits, including by admins. Disables background work, plugins, integrations, and explicit original, KePub, and bulk downloads. See [Demo Mode](#demo-mode) |
| `sync_interval_minutes` | `SYNC_INTERVAL_MINUTES` | `60` | How often to scan libraries for new content, in minutes. Set to `0` to disable scheduled scans |
| `worker_processes` | `WORKER_PROCESSES` | `2` | Number of background worker processes |
| `job_retention_days` | `JOB_RETENTION_DAYS` | `30` | Days to retain completed and failed jobs before cleanup. Set to `0` to disable cleanup |

### Demo Mode

Set `demo_mode: true` or `DEMO_MODE=true` to let visitors browse, search, read EPUB, CBZ, and PDF files, and stream M4B audio without changing the library through the API. The restriction applies to every user, including admins. Sign-in and sign-out still work; edits, setup, password changes, and server-side preference updates return `403` with code `demo_mode` and message `This action is unavailable in the demo.`

Prepare the library and user accounts before enabling Demo Mode. Scans, filesystem monitoring, job processing, and plugin loading are disabled regardless of their other settings. OPDS, eReader, Kobo, and plugin endpoints are unavailable.

Original, KePub, supplement, and bulk download controls are hidden, and the original, KePub, and bulk download routes stay blocked. Reader delivery stays available, including generated EPUB files that can still be saved by URL. Demo Mode is not copy protection; only publish media you have permission to redistribute. Reader caches and startup database migrations still require writable storage.

The sign-in page identifies Demo Mode. After sign-in, a banner stays visible across the app and readers. The interface hides administration links and security settings, while rejected edits show `This action is unavailable in the demo.` Gallery size, default sort, and reader preferences are stored in each visitor's browser instead of the server.

Restart after changing this setting. To curate the library or manage accounts again, disable Demo Mode on a private instance rather than making a public instance writable.

The hosted [Public Demo](./demo.md) runs in this mode.

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
The variables in this section select the container's runtime identity and log format. They are not YAML fields in Shisho's application configuration.
:::

| Env Variable | Image Default | Description |
|--------------|---------------|-------------|
| `PUID` | `1000` | User ID selected for the Shisho process inside the container. This does not grant host filesystem access |
| `PGID` | `1000` | Group ID selected for the Shisho process inside the container. This does not grant host filesystem access |
| `LOG_FORMAT` | `json` | Log format for Shisho. Use `console` for human-readable output |

The image creates and changes ownership of `/config`, but it does not create or change ownership of `/data`, `/media`, or custom paths. Configure host permissions for the selected `PUID` and `PGID`.
