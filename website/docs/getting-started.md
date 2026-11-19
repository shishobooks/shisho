# Getting Started

Shisho is a self-hosted book management system for ebooks, audiobooks, and comics. Docker Compose is the recommended way to run it. To see a running instance before you install, try the [Public Demo](./demo.md).

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

The `5173:5173` mapping exposes both the web interface and API. No separate frontend container is needed. For another host port, change only the left side of the mapping, unless a Kobo will sync without a reverse proxy, which needs `SERVER_PORT` changed instead; see [Device URLs](./deployment-and-maintenance.md#device-urls). See [Configuration](./configuration.md#server) for listener settings.

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

`PUID` and `PGID` select the user and group IDs used by the Shisho process inside the container. They do not grant access to host files.

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

## Set Up Shisho

Open `http://localhost:5173`. Setup walks the first admin through four steps: the account, the library, metadata plugins, and finishing. Your choices are kept in the browser until you finish, so reloading the page returns you to the same step with them intact.

### Create the Admin Account

Enter a username of 3 to 50 letters, numbers, or `.` `_` `-` `@` and a password of at least 8 characters, then select **Create Admin Account**. The first account receives the built-in admin role. See [Users and Permissions](./users-and-permissions.md) before adding more users.

### Choose Your Library

Give the library a name, and under **Folders** enter `/media`, or select **Browse** to pick folders from the server. Paths are the container side of your media mount. Add more folders with **Add another folder**.

:::danger[Review File Organization Before Finishing]
**Organize file structure during scans** is on by default. With a writable media mount, the first scan can move and rename your files into a folder per book, named `[Author] Title`, and can write covers and sidecars beside them. The animation on this step shows example files being rearranged; it is not a preview of your own files.

Back up the media first. Clear **Organize file structure during scans** before you finish if Shisho must keep your existing paths. You can change it later in the library's settings, but files already moved stay where they are.
:::

Select **Continue**. Setup checks that the server can find and read each folder, and marks any it can't, such as a mistyped path, a mount missing from the container, or a folder the container's user isn't allowed to read, before it goes on. While you are on this step, Shisho looks through the folders to count the ebooks, audiobooks, and comics in them, going by file extension. On a large library or a slow disk it stops after a few seconds or a few thousand files and says it looked at only part of them.

### Add Metadata Plugins

Plugins fill in covers, descriptions, series, and other metadata from online sources. Shisho recommends plugins from the official repository based on what it found:

| Found | Ticked | Also shown |
|-------|--------|------------|
| Ebooks | Goodreads Enricher | Open Library Enricher |
| Audiobooks | Audible Enricher | |
| Comics | Manga Enricher | |
| Nothing | Goodreads Enricher | Open Library, Audible, and Manga Enrichers |

**See all plugins** lists the repository's other plugins, unticked. Each plugin shows a one-line summary of what it can access, such as the sites it connects to, and **What it can access** lists every capability.

:::warning[Ticking a Plugin Approves Its Access]
A ticked plugin is installed when you finish, with the capabilities its card lists, such as network access to the sites named. Untick any plugin you don't want to run on your server. See [Using Plugins](./plugins/overview.md) before installing plugins from other sources.
:::

Select **Skip this step** to finish without plugins. If Shisho can't reach the plugin repository, the step says so and **Continue** finishes without them. You can install plugins later under **Administration > Plugins**.

### Finish

**Finish setup** installs the ticked plugins first and then creates the library, so its first scan runs with them. A plugin that fails to install is reported, and setup still creates the library. You land on the library page, which shows the scan's progress while books appear.

**Skip setup**, on the library and plugin steps, leaves setup without creating a library. Setup doesn't come back on its own. To return to it at the library step, select **Set up a library** on the empty **Administration > Libraries** page, or **Set up your first library** in [Getting Started](./users-and-permissions.md#getting-started).

## Next Steps

The **Getting Started** pill in the top nav lists what's left, such as inviting someone and connecting a reading device. See [Getting Started](./users-and-permissions.md#getting-started).

For scanning behavior and library settings, continue to [Libraries](./libraries.md). For all server options, see [Configuration](./configuration.md).

## Invite People

To give someone their own account, open **Administration > Users**, select **Add User**, and send them the invite link it creates. They choose their own username and password. See [Invite Links](./users-and-permissions.md#invite-links).

## Connect a Device

To read on an e-reader or a reading app, choose **Connect a reading device** in **Getting Started**, or click **Connect Device** under **Devices** in the user menu. Open Shisho at the address your devices use before you start, not `http://localhost:5173`, which a device can't reach. [Devices](./users-and-permissions.md#devices) covers how the flow works, and each way of connecting has its own page: [Kobo Sync](./kobo-sync.md), [KOReader](./koreader.md), the [eReader Browser](./ereader-browser.md), and the [OPDS Catalog](./opds.md#sign-in-as-a-device).
