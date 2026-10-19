# Kobo Sync

Kobo Sync sends books from Shisho to a Kobo eReader over Wi-Fi. Shisho handles personal-library sync requests and proxies other Kobo API requests to Kobo's store service.

## What Syncs

- Only **main EPUB and CBZ files** sync.
- Every compatible main file syncs, so a book with multiple editions can create multiple entries on the device.
- Shisho generates a KePub for each synced EPUB or CBZ. Its filename uses the author, the series, and the file's name (or the book's title when the file has no name). It keeps only unaccented letters A to Z, digits, spaces, and `-_.,()'`, because other characters can stop a Kobo from reading the file, so accented and non-Latin characters are dropped from the filename. The book's metadata keeps them.
- On the Kobo, each entry's title is the file's **Name**, or the book's title when the file has no name. The subtitle and authors come from the book. Changing a synced file's name updates the title on the Kobo at the next sync.
- The first sync is full. Later syncs are incremental and send additions, removals, and detected metadata or cover changes, including books entering or leaving the Device's [scope](./users-and-permissions.md#scope).
- Moving, renaming, or replacing a synced file updates the book already on the Kobo instead of adding a second copy. The Kobo downloads the new file in place and keeps the reading position. Highlights can stay hidden for a while after the download, then reappear.
- M4B, PDF, MOBI, AZW3, and [supplement files](./supplement-files.md) do not sync. A Kobo cannot read MOBI or AZW3, and they cannot be converted to KePub.

## Setup

### 1. Add a Kobo Device

Open the user menu and select **Devices**. Click **Add Device**, enter a name, select **Kobo**, and click **Add Device**. Shisho opens the new Device's page. See [Devices](./users-and-permissions.md#devices) for what a Device is.

### 2. Choose a Scope

Under **Scope** on the Device's page, choose **All Books**, **Library**, or **List**, and for the last two, choose the library or list. The scope is saved on the Device, so you can change it later from Shisho without touching the Kobo; the next sync adds and removes books to match. See [Scope](./users-and-permissions.md#scope) for which books each scope reaches.

Book downloads, covers, and metadata follow the same scope, so a Kobo cannot fetch a book outside it.

### 3. Configure the Kobo

:::warning
The **Kobo sync URL** contains the Device's credential. Treat the complete URL like a password. Do not publish it, include it in screenshots or logs, or copy it to a device you do not control.
:::

1. Under **Connections** on the Device's page, copy the **Kobo sync URL**. It has the form `https://your-server/kobo/<credential>`.
2. Connect the Kobo to your computer with USB.
3. Open `.kobo/Kobo/Kobo eReader.conf` on the device.
4. Find `api_endpoint=https://storeapi.kobo.com`.
5. Replace the value with the complete **Kobo sync URL**.
6. Safely eject the Kobo and start a sync.

If `.kobo` is hidden, enable hidden files in your file manager. On macOS, press `Cmd+Shift+.` in Finder.

### Kobos Set Up Before Devices

A Kobo set up with an older Shisho has a sync URL that ends in a scope, such as `/all` or `/library/3`. Upgrading turned its API key into a Kobo Device with the same credential, so the URL keeps working and the books already on the Kobo stay. Its first sync after the upgrade copies the scope from the URL onto the Device, unless you change the Device's **Scope** first. After that, the scope in the URL is ignored, and the Device's **Scope** setting applies. You do not need to change the URL on the Kobo.

A Device has one scope. If several Kobos shared one sync URL credential with different scopes, the first to sync after the upgrade sets the scope for all of them. Add a Kobo Device for each of the others, choose its scope, and update its `api_endpoint` with the new **Kobo sync URL**.

### How Long the URL Works

The **Kobo sync URL** follows the same rules as an [eReader Browser URL](./ereader-browser.md#how-long-the-url-works): syncs fail once the Device is deleted, revoked, or its credential is regenerated, while its owner is deactivated, and until the owner's role has Books Read again. Syncs also fail while the Device's **Kobo sync** connection is off.

## Reset or Remove Sync

- **Reset:** On the Device's page, click **Reset Sync** under **Kobo sync**. This clears Shisho's sync history. The next sync is a fresh sync and resends every book in scope.
- **Change scope:** Change **Scope** on the Device's page and sync. The Kobo keeps its URL.
- **Stop using Shisho:** Restore `api_endpoint=https://storeapi.kobo.com` on the Kobo, then delete the Device in Shisho to revoke its credential. Deleting cannot be undone, so restore Kobo's store endpoint first when you want to keep normal Kobo service.

## Network Requirements

The Kobo must be able to reach the public or local Shisho URL in `api_endpoint`. Shisho also needs outbound HTTPS access to `https://storeapi.kobo.com` because unhandled store requests are proxied upstream. This outbound connection is an operational requirement and does not change which Shisho books are in scope.

## Troubleshooting

### Sync Fails Immediately

**Symptom:** The Kobo reports a sync error before personal books appear.

**Likely cause:** `api_endpoint` is incomplete or has a mistyped credential, a reverse proxy is not forwarding `/kobo/`, or the URL stopped working for one of the reasons in [How Long the URL Works](#how-long-the-url-works).

**Verify:** Compare the complete configured value with the **Kobo sync URL** on the Device's page. The credential must match exactly, in lowercase with no spaces or hyphens. Confirm that the Kobo can resolve and open the external Shisho hostname.

**Fix:** Copy the **Kobo sync URL** in full, update `Kobo eReader.conf`, and forward `/kobo/` without removing route segments.

### Books Sync but Never Download, or Covers Are Missing

**Symptom:** Books appear on the Kobo after a sync, but downloading them fails and covers do not load.

**Likely cause:** The download and cover URLs Shisho sends point at a different port than the one in `api_endpoint`. Kobo devices leave the port out of their requests, so Shisho works it out as described in [Device URLs](./deployment-and-maintenance.md#device-urls).

**Verify:** Check whether `api_endpoint` uses a port other than `80` or `443`, and whether a Docker port mapping or reverse proxy sits between the Kobo and Shisho.

**Fix:** Without a proxy, publish the same port on both sides of the Docker mapping, setting `SERVER_PORT` to it if it is not `5173`. Behind a proxy on a non-default port, have the proxy send its public port as `X-Forwarded-Port`. Then sync again.

### Books or Metadata Are Stale

**Symptom:** Removed books remain, metadata is old, or the device shows the wrong scope.

**Likely cause:** The Device's **Scope** is not the one you expect, or its incremental sync history no longer matches the desired state.

**Verify:** Check **Scope** on the Device's page. A scope whose library or list was deleted reaches no books.

**Fix:** Choose the intended scope and sync. If the books are still wrong, click **Reset Sync** on the Device's page, then sync again.

### Kobo Store Features Fail

**Symptom:** Personal books sync, but Kobo store features fail or remain empty.

**Likely cause:** The Shisho server cannot reach Kobo's upstream service.

**Verify:** Check outbound DNS and HTTPS connectivity from the Shisho host to `storeapi.kobo.com`.

**Fix:** Allow that outbound connection without exposing additional inbound Shisho routes. See [Deployment and Maintenance](./deployment-and-maintenance.md) and [Troubleshooting](./troubleshooting.md).

For access questions, see [Users and Permissions](./users-and-permissions.md).
