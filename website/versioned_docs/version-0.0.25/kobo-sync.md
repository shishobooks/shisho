---
sidebar_position: 8
---

# Kobo Sync

Kobo sync lets you push books from Shisho directly to a Kobo e-reader over WiFi. When your Kobo syncs, it receives new books, updated metadata, and cover art automatically — the same way it would from the Kobo store.

## How It Works

Kobo e-readers sync by contacting an API endpoint to check for changes. By default, this points to Kobo's own store. Shisho replaces that endpoint with its own, so your Kobo syncs against your personal library instead. Requests that Shisho doesn't handle (like store browsing) are proxied through to Kobo's servers so nothing else breaks.

Each sync is incremental. Shisho tracks what has been sent to each device and only transfers changes — new books, removed books, and updated metadata or covers. The first sync after setup sends your entire library.

EPUB and CBZ files are automatically converted to KePub format during sync for the best reading experience on Kobo hardware.

## Setup

### 1. Add a Kobo device in Shisho

Go to **Settings > Security** and click **Add Kobo** under the Kobo Wireless Sync section. Give it a name to identify the device.

### 2. Choose a sync scope

After adding the device, click **Setup** to configure what it syncs. You can scope the sync to:

| Scope | Description |
|-------|-------------|
| **All Libraries** | Syncs every book from all your libraries |
| **Library** | Syncs books from a single library |
| **List** | Syncs books from a single list |

The setup dialog generates an API endpoint URL based on your selection.

### 3. Configure the Kobo

1. Connect the Kobo to your computer via USB
2. Open the hidden `.kobo` folder on the device
3. Edit `Kobo/Kobo eReader.conf`
4. Find the line starting with `api_endpoint=`
5. Replace the value with the URL from the setup dialog
6. Safely eject the Kobo and trigger a sync from the device

:::tip
If you don't see a `.kobo` folder, enable hidden files in your file manager. On macOS, press `Cmd+Shift+.` in Finder.
:::

## Sync Behavior

- **Books with multiple files** — Only the [primary file](advanced/primary-file.md) is synced, so you won't get duplicate entries on the device
- **Metadata changes** — If you update a book's title, authors, or cover in Shisho, those changes are picked up on the next sync
- **Removed books** — Books removed from the sync scope are removed from the device on next sync

## Resetting Sync

If your device gets out of sync, you can clear the sync history from the setup dialog by clicking **Reset**. The next sync will be a full sync, re-sending all books as if the device were new.

## Requirements

- Your Kobo must be on the same network as Shisho (or Shisho must be accessible from the internet)
- The Kobo needs to reach Shisho's URL — if you're running behind a reverse proxy, make sure the `/kobo/` path is forwarded
