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

Open the short URL in the eReader's browser. After the redirect, bookmark the resulting full `/ereader/key/...` URL. The full URL remains valid until the device is removed from Shisho and its API key is revoked.

## Features

The current browser provides:

- A list of libraries the device owner's user can access.
- Per-library **All Books**, **Series**, **Authors**, and **Search** pages.
- EPUB, CBZ, M4B, and PDF type filters.
- Book details and a separate download for every main-file edition.
- The user's saved per-library sort on **All Books**, author, and search results, or **Date Added, Newest First** when none is saved. Series books follow series-number order.
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

### The Bookmark Returns Unauthorized

**Symptom:** A previously working bookmark returns an unauthorized response.

**Likely cause:** The bookmark is incomplete or the device was removed and its API key was revoked.

**Verify:** Confirm that the bookmark contains the complete `/ereader/key/...` path and that the device still appears under **Security**.

**Fix:** If the device was removed, add it again and replace the bookmark. The old key cannot be restored.

### The Page Does Not Load Through a Proxy

**Symptom:** The main web interface works, but the setup URL, eReader pages, covers, or downloads do not.

**Likely cause:** The proxy does not forward both eReader route families or the device cannot trust the external hostname.

**Verify:** Confirm that `/e/` and `/ereader/` both reach Shisho with their complete paths, and that the eReader can resolve the hostname and trust its certificate.

**Fix:** Forward `/e/` for setup URLs and `/ereader/` for the full browser, covers, and downloads. See [Deployment and Maintenance](./deployment-and-maintenance.md) and [Troubleshooting](./troubleshooting.md).

### Books or Libraries Are Missing

**Symptom:** The browser opens, but an expected library or book is absent.

**Likely cause:** The owning user lacks library access, a file-type filter is active, or the expected file is a supplement.

**Verify:** Check the user's library access, clear the filter, and confirm the book has a main file.

**Fix:** Grant the intended [library access](./users-and-permissions.md) or select a matching main-file type. Supplements are not offered.
