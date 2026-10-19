# eReader Browser

The eReader Browser is a lightweight, server-rendered interface for downloading books from simple e-ink web browsers, such as the ones on Kobo and Kindle devices. It uses minimal HTML and no JavaScript.

## Setup

### 1. Add a Device

Open the user menu and select **Devices**. Click **Add Device**, enter a name, select the type that matches the e-reader (**Kobo**, **Kindle**, or **Browser** for any other e-reader), and click **Add Device**. Shisho opens the new Device's page. A Kobo that already syncs with [Kobo Sync](./kobo-sync.md) can use its existing Kobo Device. See [Devices](./users-and-permissions.md#devices) for types, credentials, and scope.

### 2. Generate the Setup URL

Under **Connections** on the Device's page, make sure **eReader browser** is on, then click **Get Setup URL**. Shisho displays a short URL such as:

```text
https://your-server/e/abc123
```

The short URL is reusable for 30 minutes. Opening it redirects to a full URL under `/ereader/key/...` that contains the Device's credential. Expiration of the short URL does not expire the full URL. The Device's page also shows the full URL, for an e-reader where typing it is easier than following a redirect.

### 3. Bookmark the Full URL

:::warning
The redirected full URL contains the Device's credential. Treat it like a password. Do not share it, publish it, include it in screenshots, or bookmark it on a device you do not control. Deleting the Device revokes the URL and cannot be undone. If access may be needed later, keep the Device and protect its URL instead.
:::

Open the short URL in the eReader's browser. After the redirect, bookmark the resulting full `/ereader/key/...` URL. Bookmarks saved before Devices existed keep working.

### How Long the URL Works

The full URL keeps working only while all of these hold. The same rules apply to a [Kobo Sync](./kobo-sync.md) URL.

- The Device still appears under **Devices**, and its credential has not been regenerated. Deleting the Device, an administrator revoking it, or regenerating its credential revokes the URL permanently.
- The Device's **eReader browser** connection is on. While it is off, the URL returns a forbidden response.
- The Device owner's account is active. A [deactivated](./users-and-permissions.md#deactivate-users) owner's URL returns an unauthorized response.
- The owner's role has Books Read. Without it, the URL returns a forbidden response, and it works again once Books Read is restored.

The short setup URL refuses to redirect in the same cases.

## Features

The current browser provides:

- A list of the libraries the Device's [scope](./users-and-permissions.md#scope) reaches. A Device scoped to one library shows only that library. A Device scoped to a list shows the libraries that have books on the list, and only those books, with the series and authors they belong to.
- Per-library **All Books**, **Series**, **Authors**, and **Search** pages.
- EPUB, CBZ, M4B, PDF, MOBI, and AZW3 type filters. A filter shows the books with a main file of that type.
- Book details and a separate download for every main-file edition, except on a Kindle (see [On a Kindle](#on-a-kindle)).
- The user's saved per-library [sort](./browsing-search-bulk-actions.md#gallery-sort) on **All Books**, author, and search results. Series books follow series-number order.
- An optional cover toggle.

Supplement files are not offered as book downloads. See [Libraries, Scanning, and File Organization](./libraries.md), [Browsing, Search, and Bulk Actions](./browsing-search-bulk-actions.md), and [Supported Formats](./supported-formats.md).

## Downloads

Shisho normally prepares each native-format download with the metadata that format supports. When the request's User-Agent contains `Kobo`, EPUB and CBZ download links use generated KePubs instead. M4B, PDF, MOBI, and AZW3 remain generated downloads in their native formats. The first download of some formats shows no progress or size on the device; later downloads of the same book do. [First Downloads](./supported-formats.md#first-downloads) lists those formats.

Covers are off by default. Leave them off on slow devices or networks to reduce page size and image requests.

### On a Kindle

A Kindle's browser downloads only MOBI and AZW3, and refuses EPUB, PDF, and everything else. Some Kindles refuse AZW3 downloads; if yours does, add a MOBI file to the book. When the request's User-Agent contains `Kindle/`, which every e-ink Kindle sends, the browser offers only what a Kindle can download:

- A book's page offers its MOBI files. A book with no MOBI offers its AZW3 files instead, with a note that some Kindles refuse AZW3 downloads.
- A book with neither is marked **Unavailable on Kindle**, both in book lists and on the book's page, and offers no download.

Fire tablets browse with Silk, which does not send `Kindle/`, so they see every download like any other browser.

A MOBI or AZW3 download carries the book's current metadata and cover; see [MOBI and AZW3](./supported-formats.md#mobi-and-azw3) for what is written.

To use it, follow [Setup](#setup) in the Kindle's web browser and bookmark the full URL there. A downloaded book appears in the Kindle's library. To get a book onto a Kindle when it has only an EPUB, or only an AZW3 and the Kindle refuses it, add a MOBI file to the book in Shisho.

## Troubleshooting

### The Short URL Expired

**Symptom:** Opening `/e/...` no longer redirects to the eReader Browser.

**Likely cause:** The 30-minute setup window expired.

**Verify:** Try the bookmarked full `/ereader/key/...` URL, if one was saved.

**Fix:** Keep using a valid full bookmark, or click **Get Setup URL** on the Device's page for another short URL.

### The Bookmark Stops Working

**Symptom:** A previously working bookmark returns an unauthorized or forbidden response.

**Likely cause:** The bookmark is incomplete, the Device was deleted or revoked, its credential was regenerated, its **eReader browser** connection is off, the owner was deactivated, or the owner's role lost Books Read. See [How Long the URL Works](#how-long-the-url-works).

**Verify:** Confirm that the bookmark contains the complete `/ereader/key/...` path and that its credential matches the one on the Device's page. Ask an administrator to check the owner's account status and role under **Settings > Users**.

**Fix:** If the credential changed, replace the bookmark with the new URL. If the Device was deleted, add it again and replace the bookmark. A deleted Device's credential cannot be restored. If the role lost Books Read, an administrator can add it back under **Settings > Users** by selecting the role in the **Roles** section. See [Custom Roles](./users-and-permissions.md#custom-roles). A deactivated owner cannot be reactivated from the web interface, so add a device under an active account instead, or see [Deactivate Users](./users-and-permissions.md#deactivate-users).

### The Page Does Not Load Through a Proxy

**Symptom:** The main web interface works, but the setup URL, eReader pages, covers, or downloads do not.

**Likely cause:** The proxy does not forward both eReader route families or the device cannot trust the external hostname.

**Verify:** Confirm that `/e/` and `/ereader/` both reach Shisho with their complete paths, and that the eReader can resolve the hostname and trust its certificate.

**Fix:** Forward `/e/` for setup URLs and `/ereader/` for the full browser, covers, and downloads. See [Deployment and Maintenance](./deployment-and-maintenance.md) and [Troubleshooting](./troubleshooting.md).

### Books or Libraries Are Missing

**Symptom:** The browser opens, but an expected library or book is absent, or a saved book, cover, or download link returns **not found**.

**Likely cause:** The Device's scope leaves it out, the owning user lacks library access, a file-type filter is active, or the expected file is a supplement. A book or file outside the scope or in a library the user cannot access returns **not found**, the same as one that does not exist, so the URL does not reveal whether it exists.

**Verify:** Check **Scope** on the Device's page and the user's library access, clear the filter, and confirm the book has a main file.

**Fix:** Change the Device's [scope](./users-and-permissions.md#scope), grant the intended [library access](./users-and-permissions.md#library-access), or select a matching main-file type. Supplements are not offered.
