---
sidebar_position: 110
---

# OPDS

Shisho provides an OPDS 1.2 catalog feed, allowing any OPDS-compatible app to browse and download books from your library. OPDS (Open Publication Distribution System) is an open standard that many reading apps support natively.

## Feed URL

The base catalog URL is:

```
http://your-server/opds/v1/{types}/catalog
```

Replace `{types}` with the file formats you want to see:

| Types | Description |
|-------|-------------|
| `epub` | EPUBs only |
| `cbz` | Comics only |
| `m4b` | Audiobooks only |
| `epub+cbz` | EPUBs and comics |
| `epub+cbz+m4b` | All formats |

For example, to browse only EPUBs: `http://your-server/opds/v1/epub/catalog`

### KePub variant

For Kobo devices or apps that benefit from KePub formatting (see [Kobo Sync](./kobo-sync) for the dedicated sync feature), use the KePub feed:

```
http://your-server/opds/v1/kepub/{types}/catalog
```

This serves the same catalog but EPUB and CBZ downloads are converted to KePub format.

## Authentication

OPDS uses HTTP Basic Authentication with your Shisho username and password — the same credentials you use to log in to the web UI. Most OPDS apps prompt for these when you add a new catalog.

### Behind a reverse proxy

If you're running Shisho behind an HTTPS-terminating reverse proxy (Nginx Proxy Manager, Traefik, etc.), the upstream proxy must forward `X-Forwarded-Proto: https` so Shisho generates `https://` hrefs in feed entries. OPDS clients like KOReader drop the `Authorization` header when following an `http://` → `https://` redirect, which surfaces as 401s on sub-feeds.

The bundled Caddy config trusts `X-Forwarded-*` headers from any RFC1918 private-range address, which covers the usual Docker/LAN topology. If your upstream proxy is on a public IP or in a non-RFC1918 range (some VPN networks, CGNAT, etc.), you'll need to override the Caddyfile and add the upstream's address to Caddy's [`trusted_proxies`](https://caddyserver.com/docs/caddyfile/options#trusted-proxies).

## Catalog Structure

The feed is organized as:

1. **Root catalog** — Lists your libraries
2. **Library** — Navigation page with links to All Books, Series, and Authors
3. **All Books** — Paginated list of every book in the library
4. **Series** — Alphabetical list of series, then books within each series
5. **Authors** — Alphabetical list of authors, then books by each author

Each level supports pagination (50 books per page).

## Search

Each library has an integrated OpenSearch endpoint. OPDS apps that support search will show a search bar when browsing a library. Search matches against book titles, authors, and series names.

## Compatible Apps

OPDS is widely supported. Some popular apps include:

- **KOReader** — Open-source reader for Kindle, Kobo, and other devices
- **Panels** — Comic reader for iOS/macOS
- **Librera** — Android reader
- **Cantook** — iOS/Android reader
- **Moon+ Reader** — Android reader
- **Thunderclap** — macOS OPDS browser

Consult your app's documentation for how to add a new OPDS catalog.

## Library Access

The feed respects your user's [library permissions](./users-and-permissions#library-access). You only see books from libraries you have access to.

## Sort order

Book-listing feeds (library, series, author, genre, tag, all-books, recently-added) apply the authenticated user's saved default sort for the relevant library. If no default has been saved, the feed falls back to the builtin default: **Date added, newest first**. See [Gallery Sort](./gallery-sort.md) for how to save a different default for a library.

When a feed is not scoped to a single library (for example, the all-books root feed across a user's libraries), Shisho uses the same builtin default — **Date added, newest first** — because `user_library_settings` is per-library by design.
