# Shisho

<p align="center">
  <img src="assets/splash.png" alt="Shisho - Your all-in-one solution for ebooks, audiobooks, and comics" width="600">
</p>

<p align="center">
  <a href="https://github.com/shishobooks/shisho/releases"><img src="https://img.shields.io/badge/version-v0.0.0-green.svg" alt="Version"></a>
  <a href="https://github.com/shishobooks/shisho/actions/workflows/ci.yml"><img src="https://github.com/shishobooks/shisho/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
</p>

Your all-in-one solution for ebooks, audiobooks, and comics.

## Why Shisho?

There is currently no great self-hosted solution to manage all book types. While there are some excellent options for specific formats—Audiobookshelf for audiobooks, Komga for comics—all other file types end up as second-class citizens. There's no way to manage all of your books in a single, unified system.

Calibre is the de facto standard for ebook management, but it doesn't work well as a self-hosted system. Calibre-web helps bridge some of the gap, and calibre-web-automated adds some automation, but I never found anything that worked with how I wanted to manage my files. As an active user of Jellyfin, I essentially wanted Jellyfin, but for all my books.

So that's what I set out to create.

## Getting Started

### Docker Setup

The recommended way to run Shisho is with Docker Compose, though any container runtime will work.

1. Create a `docker-compose.yml` file:

```yaml
services:
  shisho:
    image: ghcr.io/shishobooks/shisho:latest
    container_name: shisho
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      # Persistent data (database)
      - ./data:/data
      # Configuration
      - ./config:/config
      # Mount your media library (adjust path as needed)
      - /path/to/your/books:/media:ro
    environment:
      - PUID=1000
      - PGID=1000
      - DATABASE_FILE_PATH=/data/shisho.db
      - JWT_SECRET=your-secret-key-here-change-me
```

2. Start the container:

```bash
docker compose up -d
```

3. Access Shisho at `http://localhost:8080` and create a library pointing to `/media` (or wherever you mounted your books).

### File Permissions (PUID/PGID)

Shisho uses `PUID` and `PGID` environment variables to run as a specific user inside the container. This ensures the container can read (and optionally write) your book files with the correct permissions.

**This is important.** If you don't configure these correctly, Shisho may not be able to read your files, or you may end up with permission issues on files it creates.

To find the UID and GID of the user that owns your book files:

```bash
# Check ownership of your books directory
ls -ln /path/to/your/books

# Example output:
# drwxr-xr-x 15 1000 1000 4096 Jan 15 10:30 books
#               ^^^^ ^^^^
#               UID  GID

# Or check a specific file
stat -c '%u %g' /path/to/your/books/some-book.epub
```

Set these values in your `docker-compose.yml`:

```yaml
environment:
  - PUID=1000  # Replace with your UID
  - PGID=1000  # Replace with your GID
```

## Directory Structure

Shisho works best when each book has its own directory. All editions of a book (EPUB, M4B, PDF, CBZ, etc.) should live in the same directory.

### Recommended Structure

```
/media/
└── Main Library/
    ├── [Andy Weir] Project Hail Mary/
    │   ├── Project Hail Mary.epub
    │   ├── Project Hail Mary.epub.cover.jpeg
    │   ├── Project Hail Mary {Ray Porter}.m4b
    │   └── Project Hail Mary {Ray Porter}.m4b.cover.png
    ├── [Andy Weir] The Martian/
    │   └── The Martian.epub
    ├── [Brian K. Vaughan] Saga Vol 1/
    │   └── Saga Vol 1.cbz
    └── [James Clear] Atomic Habits/
        ├── Atomic Habits.epub
        └── Supplement.pdf
```

### Organize Files Setting

Shisho includes an optional "Organize Files" feature in library settings that can automatically organize your books into a consistent directory structure. When enabled, Shisho will move and rename files based on metadata.

If you prefer to manage your own file organization, you can leave this disabled and Shisho will work with whatever structure you have.

## Configuration

Shisho can be configured via a YAML config file or environment variables. Environment variables take precedence over config file values.

### Notable Settings

| Setting | Env Variable | Default | Description |
|---------|--------------|---------|-------------|
| `database_file_path` | `DATABASE_FILE_PATH` | - | **Required.** Path to the SQLite database |
| `jwt_secret` | `JWT_SECRET` | - | **Required.** Secret key for authentication tokens |
| `server_port` | `SERVER_PORT` | `3689` | Port the server listens on |
| `sync_interval_minutes` | `SYNC_INTERVAL_MINUTES` | `60` | How often to scan libraries for changes |
| `worker_processes` | `WORKER_PROCESSES` | `2` | Number of background worker processes |
| `cache_dir` | `CACHE_DIR` | `/config/cache` | Directory for cached files |

For a complete reference of all configuration options, see [`shisho.example.yaml`](shisho.example.yaml).

## Contributing

If there's a feature you'd like to see or a bug you've encountered:

1. **Check existing issues** - Search the [issues](https://github.com/shishobooks/shisho/issues) to see if someone has already reported it
2. **Give it a thumbs up** - If an issue already exists, add a reaction. This is how I prioritize what to work on next
3. **Open a new issue** - If you can't find an existing issue, feel free to create one

If you'd like to contribute code, you're more than welcome to! I'd recommend creating an issue first to discuss the bug or feature before starting work. This helps us align on the approach and avoids unnecessary back-and-forth during review.

## Support

If you'd like to support the project, I have a [Patreon](https://www.patreon.com/shishobooks). No pressure—the project will always be open-source and self-hosted. That was always the goal.

## License

[MIT](LICENSE)
