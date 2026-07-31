# Troubleshooting

Start with the symptom below, preserve the relevant logs, and make the least destructive change first. The linked pages own the complete setup and configuration details.

## Container Does Not Start

**Symptom:** The container stops before the web interface becomes available, restarts repeatedly, or reports a configuration or permission error.

**Likely cause:** `JWT_SECRET` is missing, a persistent path is not mounted or writable, the database cannot be opened, or startup on slow storage exceeded its timeout.

**Verify:** Read the container's startup output before restarting it again. Confirm that `/config` is persistent, any custom database path is inside a writable mount, and `JWT_SECRET` is set without printing its value into a support request.

**Fix:** Follow the Compose and secret-configuration steps in [Getting Started](./getting-started.md). Match the container's `PUID` and `PGID` to the host files, correct the mounts, and review startup and database settings in [Configuration](./configuration.md). See [Deployment and Maintenance](./deployment-and-maintenance.md) before moving an existing database or changing storage.

:::warning[Changing the JWT secret]
Changing `JWT_SECRET` invalidates existing signed sessions. Generate and store a strong secret, but do not rotate it merely to troubleshoot an unrelated startup problem.
:::

## Scans Report Permission Errors or Files Move Unexpectedly

**Symptom:** A scan cannot read books, cannot write covers or sidecars, or files and directories are renamed or moved.

**Likely cause:** The container user cannot access the media mount, the mount is read-only for an operation that writes files, or **Organize file structure during scans** is enabled for the library.

**Verify:** Check the library's configured container paths, the media mount mode, and the host ownership represented by `PUID` and `PGID`. In **Settings > Libraries**, open the library and check whether file organization is enabled. Review the failed scan's job log for the exact path and operation.

**Fix:** Grant only the access Shisho needs. Read access is required to scan. Write access is required for file organization and other operations that write beside library files. Disable file organization before another scan if Shisho must not rename or move files. See [Getting Started](./getting-started.md), [Libraries](./libraries.md), and [Managing Books and Files](./managing-books-and-files.md).

:::warning[Protect the source library]
Before enabling file organization, back up the library and test the setting on a small library. Do not grant broad write access as a substitute for identifying the failing path.
:::

## New Files Are Missing After a Scan or Monitor Event

**Symptom:** Files copied into a library do not appear automatically, or a manual scan finds files that the real-time monitor missed.

**Likely cause:** The host path is not mounted at the path configured for the library, the file type is unsupported, the monitor is disabled, a network filesystem does not deliver filesystem events, the monitor delay has not elapsed, or Linux exhausted its inotify watches.

**Verify:** Confirm the file is visible at the configured path from the container and compare its extension with [Supported Formats](./supported-formats.md). Run a library scan. If the scan finds the file but monitoring does not, inspect the monitor settings and current-session server logs. On Linux, check for inotify watch errors. Network storage that misses events points to a monitor limitation rather than a parser failure.

**Fix:** Correct the mount or library path, wait for the configured monitor delay, or increase the Linux watch limit as documented in [Configuration](./configuration.md). For network filesystems that do not support reliable events, disable the real-time monitor and rely on scheduled or manual scans. See [Libraries](./libraries.md) and [Deployment and Maintenance](./deployment-and-maintenance.md).

## A Background Job Failed

**Symptom:** A scan, download, hash, or plugin-related job is marked **Failed**.

**Likely cause:** The job log usually identifies a file permission, missing path, parser, plugin, storage, or database error.

**Verify:** Open **Settings > Jobs**, select the failed job, and read its job log. Job logs belong to that persisted job and remain subject to the configured job-retention period. **Settings > Logs** shows the in-memory server log for the current server session. It may contain surrounding startup or request context, but it is cleared when the server restarts.

**Fix:** Correct the first actionable error in the job log, then retry the original task. Use current-session server logs for context, not as a replacement for the job's own history. See [Deployment and Maintenance](./deployment-and-maintenance.md), [Configuration](./configuration.md), and [Users and Permissions](./users-and-permissions.md) for access to administrative pages.

## The Database Reports That It Is Locked

**Symptom:** Startup or a request repeatedly reports `database is locked`, `SQLITE_BUSY`, or `SQLITE_LOCKED` after Shisho's wait and retry handling.

**Likely cause:** More than one Shisho process is using the same SQLite file, another program is holding it open for writes, or the database is on storage with unreliable SQLite locking.

**Verify:** Confirm that only one Shisho instance points at the database file. Check whether backup, sync, or database tools are operating on the live file. Note whether the database lives on a network filesystem.

**Fix:** Stop the competing writer and keep a single Shisho instance attached to the database. Follow [Deployment and Maintenance](./deployment-and-maintenance.md) for safe backups and storage placement. The busy timeout and retry settings are documented in [Configuration](./configuration.md); increasing them may help short contention, but it does not make multiple writers or unreliable storage safe.

:::warning[Preserve the database]
Do not delete the database or its related files to clear a lock. Back it up before moving it or attempting recovery.
:::

## Downloads or Readers Fail, or Cache Usage Is High

**Symptom:** A generated download fails, a reader cannot render pages, a stale generated file is reused, or the cache consumes unexpected disk space.

**Likely cause:** The cache path is full or not writable, the source media is no longer readable, generation failed in a background job, or cached generated output needs to be rebuilt after its producer changed.

**Verify:** Check **Settings > Jobs** for the generation job, **Settings > Cache** for cache size, the current-session server logs for request errors, and available space and permissions at `cache_dir`. Confirm that the original file remains readable.

**Fix:** Correct storage or permissions first. Clear only the affected cache from **Settings > Cache** when regeneration is appropriate. Clearing cache removes generated copies, which Shisho rebuilds on demand, and may make the next request slower. See [Supported Formats](./supported-formats.md), [Deployment and Maintenance](./deployment-and-maintenance.md), and the cache settings in [Configuration](./configuration.md).

## Login, Links, or Cookies Break Behind a Reverse Proxy

**Symptom:** Login succeeds and immediately appears logged out, generated links use the wrong scheme or host, or the main interface works while API or device routes return the frontend page or a proxy error.

**Likely cause:** The proxy is not preserving the public host and HTTPS scheme, the browser and server are switching between HTTP and HTTPS, or only the single-page application route is forwarded.

**Verify:** Confirm the public URL stays on one scheme and host. Inspect whether the proxy forwards `X-Forwarded-Proto` and `X-Forwarded-Host`, and whether requests under `/api/`, `/opds/`, `/kobo/`, `/ereader/`, and `/e/` reach Shisho. A session cookie set for HTTPS will not be sent over a later HTTP request.

**Fix:** Configure one canonical public HTTPS URL, forward the public scheme and host, and route all Shisho endpoint prefixes rather than only the frontend. Review [Deployment and Maintenance](./deployment-and-maintenance.md) and the integration page for the affected route.

## OPDS Returns 401

**Symptom:** An OPDS client rejects the catalog or opens the root catalog but receives `401 Unauthorized` on a sub-feed or download.

**Likely cause:** The client has incorrect Shisho credentials, the user lacks access to the library, or an HTTP-to-HTTPS redirect caused the client to drop its Basic Authentication header.

**Verify:** Use the exact catalog URL and the same username and password used for Shisho. Confirm the user can access the target library. Check generated sub-feed links for the wrong scheme and inspect the proxy's `X-Forwarded-Proto` handling.

**Fix:** Correct the credentials or library access. Behind an HTTPS-terminating proxy, forward `X-Forwarded-Proto: https` so Shisho generates HTTPS feed links without a redirect. See [OPDS Catalog](./opds.md) and [Users and Permissions](./users-and-permissions.md).

## Kobo Sync or the eReader Browser Cannot Connect

**Symptom:** Kobo sync cannot reach Shisho, the eReader short URL fails, or a device URL returns the main web interface instead of the device endpoint.

**Likely cause:** The device cannot resolve or reach the generated host, the reverse proxy does not forward the required path, or the eReader short URL expired before first use.

**Verify:** From the same network as the device, check that the generated public host is reachable. For Kobo, verify that `/kobo/` routes reach Shisho. For the eReader Browser, verify both `/e/` short links and `/ereader/` full links. Short eReader setup links expire after 30 minutes, while the resulting full URL is the one to bookmark.

**Fix:** Correct public DNS, scheme, host, and proxy routing. Generate a new eReader short URL if it expired. Do not share a device URL because it contains the device's access key. Follow [Kobo Sync](./kobo-sync.md) or [eReader Browser](./ereader-browser.md) for setup and reset steps.

## A Plugin Will Not Load or Run

**Symptom:** An installed plugin is marked **Error** or **Incompatible**, enabling it shows a load error, required configuration is missing, or a scan completes without the expected plugin result.

**Likely cause:** The plugin requires a newer Shisho version, its manifest or JavaScript failed to load, required configuration is unset, its mode disables automated scans, a per-library override disables it, or the requested access is not declared in its manifest.

**Verify:** Open **Settings > Plugins**, select the plugin, and read its status and full load error. Check required configuration, global mode and order, and the library's plugin overrides. For a failure during a job, inspect that job's log; for load-time context, inspect the current-session server logs.

**Fix:** Update Shisho or the plugin when versions are incompatible, save the required configuration, enable the intended mode, or correct the plugin's manifest and code. Operators should start with [Using Plugins](./plugins/overview.md). Plugin authors should use [Developing Plugins](./plugins/development.md), [Manifest and Hooks Reference](./plugins/manifest-hooks-reference.md), and [Testing Plugins](./plugins/testing.md).
