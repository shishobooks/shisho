---
sidebar_position: 9
---

# eReader Browser

The eReader browser is a lightweight, text-based interface for browsing and downloading books directly from an e-reader's built-in web browser. It's designed for the constraints of e-ink screens — simple HTML, minimal styling, and no JavaScript.

## How It Works

E-readers like Kindle, Kobo, and PocketBook have built-in web browsers. The eReader browser gives them a stripped-down view of your library where you can browse, search, and download books. Each device gets its own URL with an embedded API key, so no login form is needed.

## Setup

### 1. Add a device in Shisho

Go to **Settings > Security** and click **Add Device** under the eReader Browser Access section. Give it a name (e.g., "Bedroom Kindle").

### 2. Get the short URL

Click **Setup** on the device. Shisho generates a short URL like `http://your-server/e/abc123` that's easy to type on an e-reader keyboard. This short URL expires in 30 minutes — it's only used once to get to the full URL.

### 3. Open and bookmark

On your e-reader, open the web browser and type in the short URL. It redirects to the full eReader browser URL. **Bookmark this page** so you can return to it anytime without needing to generate another short URL.

## Features

### Browsing

The eReader browser provides the same navigation as the main web UI:

- **Libraries** — Browse your libraries
- **All books** — Paginated list of every book in a library
- **Series** — Browse by series
- **Authors** — Browse by author
- **Search** — Full-text search within a library

### File type filtering

Use the type filter to show only specific formats. This is useful if your library has both ebooks and audiobooks but your device only reads EPUBs.

### Cover toggle

Covers can be turned on or off. Disabling covers makes pages load faster on slow e-ink browsers and reduces data usage.

### KePub downloads

When a Kobo device is detected (via its User-Agent string), EPUB and CBZ downloads are automatically served as KePub files for better integration with the Kobo reading system.

## Supported Devices

The eReader browser works with any device that has a web browser, including:

- **Kindle** — Via the Experimental Browser
- **Kobo** — Via the built-in browser (though [Kobo sync](kobo-sync.md) is a better option for Kobos)
- **PocketBook** — Via the built-in browser
- **Phones/tablets** — Works as a lightweight alternative to the main UI

## Authentication

Each device gets its own API key. The key is embedded in the URL, so there's no login prompt. You can revoke access to a specific device at any time by removing it from the Security settings — the URL immediately stops working.
