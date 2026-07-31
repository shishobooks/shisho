# Getting Started

Shisho is a self-hosted book management system for ebooks, audiobooks, and comics. Docker Compose is the recommended way to run it.

## Prerequisites

Install Docker Engine or Docker Desktop with the Docker Compose plugin. Published Shisho images are available for Linux AMD64 (`linux/amd64`) and Linux ARM64 (`linux/arm64`).

You also need:

- A directory for persistent Shisho state
- A media directory that is backed up
- OpenSSL, or another secure random secret generator

## Create the Compose Project

Create an empty directory for Shisho and save this as `docker-compose.yml`:

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
      # The default library setting can also move and rename media during scans.
      - /path/to/your/books:/media
    environment:
      PUID: "1000"
      PGID: "1000"
      JWT_SECRET: "${JWT_SECRET:?Set JWT_SECRET before starting Shisho}"
```

Replace `/path/to/your/books` with the host path to your media. The container path `/media` is the path you will enter when creating the library.

The single `/config` mount persists the default SQLite database at `/config/shisho.db`, caches, installed plugins, and plugin data. `DATABASE_FILE_PATH` does not need to be set for this layout.

## Generate the JWT Secret

:::warning[Protect the Authentication Secret]
Anyone with `JWT_SECRET` can forge authentication credentials. Keep the value private, store it with your other deployment secrets, and include it in your secure deployment backup. Reuse the same secret across container recreations because changing it invalidates existing login sessions.
:::

Generate a long random value:

```bash
openssl rand -hex 32
```

Provide that value as `JWT_SECRET` using the environment or secret-management method appropriate for your deployment. The Compose example expects `JWT_SECRET` to be available when you start the service.

## Configure PUID and PGID

`PUID` and `PGID` select the user and group IDs used by the processes inside the container. They do not grant access to host files.

Find the IDs that should own Shisho-created files:

```bash
id -u
id -g
```

If those IDs are not `1000`, replace the `PUID` and `PGID` values in the Compose file with the values returned by those commands.

Configure ownership and permissions on the host so those IDs can write to `./config` and to the mounted media. The image creates and adjusts ownership for `/config`. It does not create or adjust ownership for `/media` or another custom media path.

Shisho needs media writes for full functionality, including generated covers and sidecar files. A read-only media mount is not a fully functional deployment.

## Start and Verify Shisho

Start the container:

```bash
docker compose up -d
```

Check container status and the liveness endpoint:

```bash
docker compose ps
curl --fail http://localhost:5173/health
```

Follow startup and migration logs if the service is not healthy:

```bash
docker compose logs -f shisho
```

See [Deployment and Maintenance](./deployment-and-maintenance.md) for updates, backups, reverse proxies, and health check limitations.

## Create the First Admin

Open `http://localhost:5173`. On the initial setup screen, enter a username of at least 3 characters and a password of at least 8 characters, then select **Create Admin Account**.

The first account receives the built-in admin role. See [Users and Permissions](./users-and-permissions.md) before adding more users.

## Create the First Library

:::danger[Review File Organization Before Creating the Library]
**Organize file structure during scans** is enabled by default for every new library. Creating the library immediately queues its first scan. With a writable media mount, that scan can move and rename files into Shisho's organized structure and can write covers and sidecars.

Back up the media first. Clear **Organize file structure during scans** before selecting **Create Library** if you want Shisho to preserve your existing paths.
:::

Select **Create Library**, give the library a name, and add `/media` under **Library Paths**.

For scanning behavior and library settings, continue to [Libraries](./libraries.md). For all server options, see [Configuration](./configuration.md).
