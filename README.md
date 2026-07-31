# Shisho

<p align="center">
  <img src="assets/splash.png" alt="Shisho, your all-in-one solution for ebooks, audiobooks, and comics" width="600">
</p>

<p align="center">
  <a href="https://github.com/shishobooks/shisho/releases"><img src="https://img.shields.io/github/v/release/shishobooks/shisho?color=green&label=version" alt="Version"></a>
  <a href="https://github.com/shishobooks/shisho/actions/workflows/ci.yml"><img src="https://github.com/shishobooks/shisho/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
</p>

Shisho is a self-hosted manager for ebooks, audiobooks, and comics. It provides one library for multiple book formats, metadata management, OPDS and device access, user permissions, and an extensible plugin system.

The [Shisho website documentation](https://www.shishobooks.com/docs/getting-started) is the canonical source for setup and configuration guidance for the latest release.

## Why Shisho?

There is no single self-hosted solution that treats ebooks, audiobooks, and comics as equal parts of one library. Tools such as Audiobookshelf and Komga are excellent for particular media, while Calibre and its web frontends focus primarily on ebooks. Shisho grew from wanting a "Jellyfin for books" that could manage supported formats together instead of treating some of them as secondary.

The goal is a unified library with metadata extraction and editing, optional plugin-based enrichment, manual identification when automation is wrong, Kobo and phone access, and user management for sharing a collection with friends and family.

## Quick Start

Docker Compose is the recommended deployment method. The published image supports Linux AMD64 and ARM64.

1. Create a working directory and save the following as `docker-compose.yml`:

```yaml
services:
  shisho:
    image: ghcr.io/shishobooks/shisho:latest
    container_name: shisho
    restart: unless-stopped
    ports:
      - "5173:5173"
    volumes:
      - ./config:/config
      # WARNING: This writable mount lets Shisho create covers and sidecars.
      # A library's default organization setting can also move and rename media.
      - /path/to/your/books:/media
    environment:
      PUID: "1000"
      PGID: "1000"
      JWT_SECRET: "${JWT_SECRET:?Set JWT_SECRET before starting Shisho}"
```

2. Replace `/path/to/your/books`, then generate a persistent JWT secret:

```sh
openssl rand -hex 32
```

Provide the generated value as `JWT_SECRET` using your deployment platform's environment or secret-management method. Keep it private and backed up. Changing it invalidates existing sessions.

3. Start Shisho:

```sh
docker compose up -d
docker compose logs -f shisho
```

Open `http://localhost:5173` and create the first admin account.

> **Warning:** Creating a library immediately queues a scan. **Organize file structure during scans** is enabled by default for new libraries and can move or rename files on writable media. Back up your media and review that setting before creating the library if you do not want Shisho to reorganize it.

Then create a library whose container path is `/media`.

`DATABASE_FILE_PATH` is optional and defaults to `/config/shisho.db`, so the single `/config` mount persists the default database, caches, and plugin data.

`PUID` and `PGID` select the identity used by the processes inside the container. They do not grant that identity access to the media mount. Configure host ownership and permissions so the selected IDs can read and write the paths Shisho needs.

See [Getting Started](https://www.shishobooks.com/docs/getting-started) for the latest released setup guidance. If you are running the current source checkout, use the [Unreleased documentation](https://www.shishobooks.com/docs/unreleased/getting-started), including [Deployment and Maintenance](https://www.shishobooks.com/docs/unreleased/deployment-and-maintenance), before exposing or updating the installation.

## AI Usage

Shisho does not include an AI service. It has been built with assistance from AI coding tools, with features planned, reviewed, and maintained by the project author. The project started before that tooling was available, but AI assistance made it possible to work on it more consistently around work and life. The architecture, product direction, review, and responsibility for the result remain human-owned.

This disclosure matters because self-hosted users have reasonable concerns about low-quality or unmaintained AI-generated applications. Shisho is developed for a real personal library, and stability and maintainability remain core goals.

## Contributing

Before opening a new issue, search the [existing issues](https://github.com/shishobooks/shisho/issues). Reactions on existing requests help with prioritization. If you want to contribute code, opening an issue first helps align on the approach.

## Support

You can support development through [Patreon](https://www.patreon.com/shishobooks). Shisho remains open source and available for self-hosting.

## License

[MIT](LICENSE)
