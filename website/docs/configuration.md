---
sidebar_position: 3
---

# Configuration

Shisho can be configured via a YAML config file or environment variables. Environment variables take precedence over config file values.

## Settings

| Setting | Env Variable | Default | Description |
|---------|-------------|---------|-------------|
| `database_file_path` | `DATABASE_FILE_PATH` | - | **Required.** Path to the SQLite database |
| `jwt_secret` | `JWT_SECRET` | - | **Required.** Secret key for authentication tokens |
| `server_port` | `SERVER_PORT` | `3689` | Port the server listens on |
| `sync_interval_minutes` | `SYNC_INTERVAL_MINUTES` | `60` | How often to scan libraries for changes |
| `worker_processes` | `WORKER_PROCESSES` | `2` | Number of background worker processes |
| `cache_dir` | `CACHE_DIR` | `/config/cache` | Directory for cached files |

For a complete reference, see [`shisho.example.yaml`](https://github.com/shishobooks/shisho/blob/master/shisho.example.yaml).
