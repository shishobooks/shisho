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

### Cache

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `cache_dir` | `CACHE_DIR` | `/config/cache` | Directory for caching generated files (downloads and extracted CBZ pages) |
| `download_cache_max_size_gb` | `DOWNLOAD_CACHE_MAX_SIZE_GB` | `5` | Maximum size of the download cache in GB. Older files are removed automatically (LRU) when the limit is exceeded |

### Plugins

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `plugin_dir` | `PLUGIN_DIR` | `/config/plugins/installed` | Directory where installed [plugins](./plugins/overview) are stored |

### Supplement Discovery

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `supplement_exclude_patterns` | `SUPPLEMENT_EXCLUDE_PATTERNS` | `[".*", ".DS_Store", "Thumbs.db", "desktop.ini"]` | Glob patterns to exclude from [supplement file](./supplement-files) discovery. Env var accepts comma-separated values |

### Authentication

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `jwt_secret` | `JWT_SECRET` | - | **Required.** Secret key for signing JWT authentication tokens. Use a long, random string (at least 32 characters). Generate one with: `openssl rand -base64 32` |
