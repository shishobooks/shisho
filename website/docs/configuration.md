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

List settings take a YAML list in the file. As environment variables they take comma-separated values, for example `SUPPLEMENT_EXCLUDE_PATTERNS=".*,*.nfo"`. Spaces around each item are trimmed and an empty variable is an empty list.

Duration settings use Go's duration format: a number with a unit, such as `500ms`, `5s`, or `1m`. A bare number is read as nanoseconds, so a duration below `1ms` is rejected.

Shisho checks every setting at startup and refuses to start when one is invalid. The error names the setting, its environment variable, and the allowed range.

Share Links are not a configuration option. An admin turns them on under **Settings > Sharing** without a restart. See [Sharing](./sharing.md).

## Settings

### Database

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `database_file_path` | `DATABASE_FILE_PATH` | `/config/shisho.db` | Path to the SQLite database file. Optional because the default is applied automatically |
| `database_debug` | `DATABASE_DEBUG` | `false` | Log every SQL statement, with parameter values, at debug level. Very verbose, and exposes stored values such as password hashes and share tokens to **Settings > Logs**. Enable briefly for troubleshooting. Queries slower than 250 ms are always logged as warnings without their values |
| `database_connect_retry_count` | `DATABASE_CONNECT_RETRY_COUNT` | `5` | Total number of connection attempts on startup, including the first. `0` skips the startup connection check. Minimum `0` |
| `database_connect_retry_delay` | `DATABASE_CONNECT_RETRY_DELAY` | `2s` | Delay between connection attempts, as a duration. Minimum `1ms` |
| `database_busy_timeout` | `DATABASE_BUSY_TIMEOUT` | `5s` | How long to wait when the database is locked, as a duration. Minimum `1ms` |
| `database_max_retries` | `DATABASE_MAX_RETRIES` | `5` | Maximum retries for database operations that report busy or locked errors. `0` means one attempt with no retries. Minimum `0` |

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
| `demo_mode` | `DEMO_MODE` | `false` | Present a prepared library without allowing API edits, including by admins. See [Demo Mode](#demo-mode) for what it disables |
| `sync_interval_minutes` | `SYNC_INTERVAL_MINUTES` | `60` | How often to scan libraries for new content, in minutes. The first scheduled scan runs one interval after startup. Set to `0` to disable scheduled scans |
| `worker_processes` | `WORKER_PROCESSES` | `2` | Number of background workers that run jobs at the same time. Minimum `1` |
| `job_retention_days` | `JOB_RETENTION_DAYS` | `30` | Days to keep completed and failed jobs, with their logs. An hourly cleanup deletes jobs created longer ago than this. Set to `0` to disable cleanup |

### Demo Mode

Set `demo_mode: true` or `DEMO_MODE=true` to let visitors browse, search, read, and listen without changing the library through the API. The restriction applies to every user, including admins. Sign-in and sign-out still work; every other change returns `403` with code `demo_mode` and message `This action is unavailable in the demo.` The [Public Demo](./demo.md) page lists what visitors can do and [what is disabled](./demo.md#what-is-disabled), including downloads, integrations, Share Links, and list sharing.

Every role loses the administration links and security settings, and gallery size, default sort, and reader preferences are stored in each visitor's browser instead of the server.

Prepare the library and user accounts before enabling Demo Mode. Scans, filesystem monitoring, job processing, and plugin loading are disabled regardless of their other settings. Reader caches and startup database migrations still require writable storage.

Hiding the download controls does not stop copying. The readers still need the generated download route, the page images, and the audio stream, so anyone signed in can save any main file by requesting its URL directly: the generated EPUB, CBZ, PDF, or M4B, or the full audiobook from the stream route. Demo Mode is not copy protection; only publish media you have permission to redistribute.

Restart after changing this setting. To curate the library or manage accounts again, disable Demo Mode on a private instance rather than making a public instance writable.

The hosted Public Demo runs in this mode.

### Library Monitor

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `library_monitor_enabled` | `LIBRARY_MONITOR_ENABLED` | `true` | Enable real-time filesystem monitoring and targeted rescans. Disable it for filesystems that do not reliably support inotify or FSEvents |
| `library_monitor_delay_seconds` | `LIBRARY_MONITOR_DELAY_SECONDS` | `60` | Seconds to wait after a filesystem change. Additional changes reset the timer so rapid changes are processed together. Values below `5` are raised to `5`, and **Settings > Server** shows the delay in use |

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
| `cache_dir` | `CACHE_DIR` | `/config/cache` | Directory for generated downloads, cover thumbnails, extracted CBZ pages, and rendered PDF pages |
| `download_cache_max_size_gb` | `DOWNLOAD_CACHE_MAX_SIZE_GB` | `5` | Maximum download cache size in GiB (1 GiB is 1024³ bytes). After each generated download, if the cache is over this size, the least recently used files are removed until it is at 80% of it. `0` removes every cached download after each generation |
| `pdf_render_dpi` | `PDF_RENDER_DPI` | `200` | PDF viewer render resolution. Range: 72 to 600. Higher values produce sharper and larger images. After a change, pages render again at the new setting |
| `pdf_render_quality` | `PDF_RENDER_QUALITY` | `85` | JPEG quality for rendered PDF pages. Range: 1 to 100. After a change, pages render again at the new setting |

Rendered PDF pages are stored per render setting, so changing `pdf_render_dpi` or `pdf_render_quality` does not reuse pages rendered at the old values, on the server or in browsers. Pages from the old setting stay in the PDF cache until you clear it in **Settings > Cache**.

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
| `supplement_exclude_patterns` | `SUPPLEMENT_EXCLUDE_PATTERNS` | `[".*", ".DS_Store", "Thumbs.db", "desktop.ini"]` | Glob patterns excluded from [supplement file](./supplement-files.md) discovery. Matching files are only skipped, never deleted. The environment variable accepts comma-separated values |
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

#### Empty Directory Cleanup

When a scan, a deletion, or moving files between books leaves a directory with no files Shisho tracks, Shisho removes the directory if everything left in it is on a fixed list:

- Hidden files whose names start with `.`, which includes `.DS_Store`
- `Thumbs.db` and `desktop.ini`
- After a scan or a deletion, also Shisho's own cover images and [sidecar files](./sidecar-files.md)

These files are deleted with the directory. Any other file keeps the directory in place. This list is not configurable, and `supplement_exclude_patterns` does not add to it, so a pattern such as `*.txt` never deletes your files.

### Authentication

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `jwt_secret` | `JWT_SECRET` | None | Required secret for signing authentication tokens. Use at least 32 characters; generate one with `openssl rand -hex 32`. The placeholder from `shisho.example.yaml` is public, so Shisho refuses to start with it. A shorter secret still starts, but logs a warning at startup and shows one on **Settings > Server** |
| `session_duration_days` | `SESSION_DURATION_DAYS` | `30` | Server-wide number of days a login session remains valid before re-authentication |

Changing `JWT_SECRET` invalidates current sessions, so replacing a short or placeholder secret signs everyone out once. Session duration is global, not configurable per user. See [Users and Permissions](./users-and-permissions.md).

## Environment-Only Variables

:::info
The variables in this section have no YAML field. `PUID` and `PGID` apply only to the container image. `LOG_FORMAT` and `LOG_LEVEL` apply to any Shisho server.
:::

| Env Variable | Default | Description |
|--------------|---------|-------------|
| `PUID` | `1000` | User ID selected for the Shisho process inside the container. This does not grant host filesystem access |
| `PGID` | `1000` | Group ID selected for the Shisho process inside the container. This does not grant host filesystem access |
| `LOG_FORMAT` | `json` in the image, `console` otherwise | Log format for Shisho. Use `console` for human-readable output |
| `LOG_LEVEL` | `info` | Lowest level of server log lines to record: `debug`, `info`, `warn`, or `error`. SQL logging from `database_debug` is recorded at debug level whatever this is set to |

The image creates and changes ownership of `/config`, but it does not create or change ownership of `/data`, `/media`, or custom paths. Configure host permissions for the selected `PUID` and `PGID`.
