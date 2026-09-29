# eReader Browser

The eReader Browser is a lightweight, server-rendered interface for downloading books from simple e-ink web browsers. It uses minimal HTML and no JavaScript.

## Setup

### 1. Add a Device

Open the user menu and select **Security**. Under **eReader Browser Access**, click **Add Device**, enter a device name, and click **Add Device**.

### 2. Generate the Setup URL

Click **Setup** for the device. Shisho displays a short URL such as:

```text
https://your-server/e/abc123
```

The short URL is reusable for 30 minutes. Opening it redirects to a full URL under `/ereader/key/...` that contains the device API key. Expiration of the short URL does not expire the full URL.

### 3. Bookmark the Full URL

:::warning
The redirected full URL contains the device API key. Treat it like a password. Do not share it, publish it, include it in screenshots, or bookmark it on a device you do not control. Removing the device under **Security** revokes the URL and cannot be undone. If access may be needed later, keep the device entry and protect its URL instead.
:::

Open the short URL in the eReader's browser. After the redirect, bookmark the resulting full `/ereader/key/...` URL.

### How Long the URL Works

The full URL keeps working only while all three of these hold. The same rules apply to a [Kobo Sync](./kobo-sync.md) URL.

- The device still appears under **Security**. Removing it revokes the URL permanently.
- The device owner's account is active. A [deactivated](./users-and-permissions.md#deactivate-users) owner's URL returns an unauthorized response.
- The owner's role has Books Read. Without it, the URL returns a forbidden response, and it works again once Books Read is restored.

The short setup URL refuses to redirect in the same cases.

## Features

The current browser provides:

- A list of libraries the device owner's user can access.
- Per-library **All Books**, **Series**, **Authors**, and **Search** pages.
- EPUB, CBZ, M4B, and PDF type filters.
- Book details and a separate download for every main-file edition.
- The user's saved per-library [sort](./browsing-search-bulk-actions.md#gallery-sort) on **All Books**, author, and search results. Series books follow series-number order.
- An optional cover toggle.

Supplement files are not offered as book downloads. See [Libraries, Scanning, and File Organization](./libraries.md), [Browsing, Search, and Bulk Actions](./browsing-search-bulk-actions.md), and [Supported Formats](./supported-formats.md).

## Downloads

Shisho normally prepares each native-format download with the metadata that format supports. When the request's User-Agent contains `Kobo`, EPUB and CBZ download links use generated KePubs instead. M4B and PDF remain generated downloads in their native formats.

Covers are off by default. Leave them off on slow devices or networks to reduce page size and image requests.

## Troubleshooting

### The Short URL Expired

**Symptom:** Opening `/e/...` no longer redirects to the eReader Browser.

**Likely cause:** The 30-minute setup window expired.

**Verify:** Try the bookmarked full `/ereader/key/...` URL, if one was saved.

**Fix:** Keep using a valid full bookmark, or open the device's **Setup** dialog and generate another short URL.

### The Bookmark Stops Working

**Symptom:** A previously working bookmark returns an unauthorized or forbidden response.

**Likely cause:** The bookmark is incomplete, the device was removed, the owner was deactivated, or the owner's role lost Books Read. See [How Long the URL Works](#how-long-the-url-works).

**Verify:** Confirm that the bookmark contains the complete `/ereader/key/...` path and that the device still appears under **Security**. Ask an administrator to check the owner's account status and role under **Settings > Users**.

**Fix:** If the device was removed, add it again and replace the bookmark. The old key cannot be restored. If the role lost Books Read, an administrator can add it back under **Settings > Users** by selecting the role in the **Roles** section. See [Custom Roles](./users-and-permissions.md#custom-roles). A deactivated owner cannot be reactivated from the web interface, so add a device under an active account instead, or see [Deactivate Users](./users-and-permissions.md#deactivate-users).

### The Page Does Not Load Through a Proxy

**Symptom:** The main web interface works, but the setup URL, eReader pages, covers, or downloads do not.

**Likely cause:** The proxy does not forward both eReader route families or the device cannot trust the external hostname.

**Verify:** Confirm that `/e/` and `/ereader/` both reach Shisho with their complete paths, and that the eReader can resolve the hostname and trust its certificate.

**Fix:** Forward `/e/` for setup URLs and `/ereader/` for the full browser, covers, and downloads. See [Deployment and Maintenance](./deployment-and-maintenance.md) and [Troubleshooting](./troubleshooting.md).

### Books or Libraries Are Missing

**Symptom:** The browser opens, but an expected library or book is absent, or a saved book, cover, or download link returns **not found**.

**Likely cause:** The owning user lacks library access, a file-type filter is active, or the expected file is a supplement. A book or file in a library the user cannot access returns **not found**, the same as one that does not exist, so the URL does not reveal whether it exists.

**Verify:** Check the user's library access, clear the filter, and confirm the book has a main file.

**Fix:** Grant the intended [library access](./users-and-permissions.md) or select a matching main-file type. Supplements are not offered.
