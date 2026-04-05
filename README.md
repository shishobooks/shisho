# Shisho

<p align="center">
  <img src="assets/splash.png" alt="Shisho - Your all-in-one solution for ebooks, audiobooks, and comics" width="600">
</p>

<p align="center">
  <a href="https://github.com/shishobooks/shisho/releases"><img src="https://img.shields.io/github/v/release/shishobooks/shisho?color=green&label=version" alt="Version"></a>
  <a href="https://github.com/shishobooks/shisho/actions/workflows/ci.yml"><img src="https://github.com/shishobooks/shisho/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
</p>

Your all-in-one solution for ebooks, audiobooks, and comics.

## Why Shisho?

There is currently no great self-hosted solution to manage all digital book types. While there are some excellent options for specific formats like Audiobookshelf for audiobooks and Komga for comics, all other file types end up as second-class citizens. There's no way to manage all of your books in a single, unified system.

Calibre is the de facto standard for ebook management, but it doesn't work well as a self-hosted system. Calibre-web helps bridge some of the gap, and calibre-web-automated adds some automation, but I never found anything that worked with how I wanted to manage my files. As an active user of Jellyfin, I essentially wanted Jellyfin, but for all my books. And that's what this project ended up being.

I wanted a way where all book types can be displayed and organized as first-class citizens. I wanted to build a robust plugin system that enables automated fetching of metadata. I wanted a manual identification flow to correct any mistakes that the automated workflow made. I wanted it to work seamlessly with my Kobo and my phone. I wanted user management to be core to the platform so that I can share my digital library with friends and family. And I have so many other ideas for what this could be (e.g. track reading progress, allow book ratings, support more file formats, etc.).

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
      - "5173:5173"
    volumes:
      # Persistent data (database)
      - ./data:/data
      # Configuration
      - ./config:/config
      # Mount your media library (adjust path as needed)
      - /path/to/your/books:/media
    environment:
      - PUID=1000
      - PGID=1000
      - DATABASE_FILE_PATH=/data/shisho.db
      - JWT_SECRET=your-secret-key-here-change-me
```

To generate a random string to use as your `JWT_SECRET`, you can use:

```sh
openssl rand -hex 32
```

2. Start the container:

```bash
docker compose up -d
```

3. Access Shisho at `http://localhost:5173` and create a library pointing to `/media` (or wherever you mounted your books).

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
  - PUID=1000 # Replace with your UID
  - PGID=1000 # Replace with your GID
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

### Required Settings

| Setting              | Env Variable         | Description                          |
| -------------------- | -------------------- | ------------------------------------ |
| `database_file_path` | `DATABASE_FILE_PATH` | Path to the SQLite database          |
| `jwt_secret`         | `JWT_SECRET`         | Secret key for authentication tokens |

For a complete reference of all configuration options, see [`shisho.example.yaml`](shisho.example.yaml).

## AI Usage

While the app itself doesn't use any AI, it was built with the assistance of AI tooling. I know this community has been burned by other AI-built self-hosted applications in the past, but I don't intend on repeating those mistakes. I've wanted to build a "Jellyfin for books" for several years now, and while I would make small progress on it here and there, I wouldn't be able to meaningfully get any work done on it in between work and life. But with AI-assisted coding, I was able to make much more consistent progress on it. That doesn't mean I just let the AI go wild on it. Every feature that has been developed was planned and outlined by me. I created the foundation of the repo, and I picked the structure, the tooling, and the overall architectural design of it. And I intend on continuing to watch over it and ensure its stability. Unfortunately, you just have my word to go off of, but I care a lot about my digital book collection, and I want to make sure is usable, if even for my own sake. It's completely up to you on whether you want to use this yourself, but I wanted to make this point clear upfront.

## Contributing

If there's a feature you'd like to see or a bug you've encountered:

1. **Check existing issues** - Search the [issues](https://github.com/shishobooks/shisho/issues) to see if someone has already reported it
2. **Give it a thumbs up** - If an issue already exists, add a reaction. This is how I prioritize what to work on next
3. **Open a new issue** - If you can't find an existing issue, feel free to create one

If you'd like to contribute code, you're more than welcome to! I'd recommend creating an issue first to discuss the bug or feature before starting work. This helps us align on the approach and avoids unnecessary back-and-forth during review.

## Support

If you'd like to support the project, I have a [Patreon](https://www.patreon.com/shishobooks). No pressure though. The project will always be open-source and available for self-hosting. That was always the goal and will always be the goal.

## License

[MIT](LICENSE)
