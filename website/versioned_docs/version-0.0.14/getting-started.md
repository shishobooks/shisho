---
sidebar_position: 1
---

# Getting Started

Shisho is a self-hosted book management system for ebooks, audiobooks, and comics. The recommended way to run Shisho is with Docker.

## Docker Setup

Create a `docker-compose.yml` file:

```yaml
services:
  shisho:
    image: ghcr.io/shishobooks/shisho:latest
    container_name: shisho
    restart: unless-stopped
    ports:
      - "5173:8080"
    volumes:
      - ./data:/data
      - ./config:/config
      - /path/to/your/books:/media
    environment:
      - PUID=1000
      - PGID=1000
      - DATABASE_FILE_PATH=/data/shisho.db
      - JWT_SECRET=your-secret-key-here-change-me
```

Generate a random JWT secret:

```bash
openssl rand -hex 32
```

Start the container:

```bash
docker compose up -d
```

Access Shisho at `http://localhost:5173` and create a library pointing to `/media` (or wherever you mounted your books).

## File Permissions (PUID/PGID)

Shisho uses `PUID` and `PGID` environment variables to run as a specific user inside the container. This ensures the container can read (and optionally write) your book files with the correct permissions.

To find the UID and GID of the user that owns your book files:

```bash
ls -ln /path/to/your/books
```

Set these values in your `docker-compose.yml`:

```yaml
environment:
  - PUID=1000  # Replace with your UID
  - PGID=1000  # Replace with your GID
```
