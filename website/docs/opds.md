# OPDS Catalog

Shisho provides an OPDS 1.2 catalog for browsing and downloading books from compatible reading apps.

## Catalog URL

Use this root catalog URL:

```text
https://your-server/opds/v1/catalog
```

Feeds list the books your [Delivery Rules](./supported-formats.md#delivery-rules) deliver something from, and each book offers one download link per file the rules pick, in the file type they send it as. With the rules untouched, every book offers its EPUB, or its earliest other file type in Shisho's default order, as itself, plus its M4B when it has one. To get CBZs as EPUB or EPUBs as KePub in a reading app, change the rules under **User Settings > Delivery Rules**; they also decide the web app's download button.

Each download link carries the media type of the file type it sends, the one listed in [Supported Formats](./supported-formats.md), so the reading app knows how to open it, and a title naming that file type and the file's name, or the book's title when the file has no name, such as `EPUB · Illustrated Edition`. Reading apps that label downloads by title use it to tell a book's files apart. A KePub is typed as an EPUB, with a link ending in `.kepub.epub`, because KOReader recognizes downloads by their extension or media type. Downloads are generated with the book's current metadata, as that page describes for each format. The first download of some formats shows no progress or size in the reading app; later downloads of the same book do. [First Downloads](./supported-formats.md#first-downloads) lists those formats.

A download delivered through a [conversion](./supported-formats.md#conversions), such as a KePub made from an EPUB, fails with `500 Internal Server Error` when the conversion fails, for example on a damaged EPUB. It never sends the original file in its place. A file delivered as its own file type still falls back to the original file when Shisho cannot write metadata into it.

## Authentication and Access

OPDS uses HTTP Basic Authentication. Add the catalog with the same Shisho username and password used for the web interface. Use HTTPS whenever the catalog is reachable outside a trusted local network.

The user's role needs Books Read. Without it, every catalog feed, cover, and download returns `403 Forbidden`, the same rule [Kobo Sync](./kobo-sync.md) and [eReader Browser](./ereader-browser.md) URLs follow.

The root catalog lists only libraries the authenticated user can access, followed by a **Lists** entry when the user owns or has been shared a [list](./lists.md) with something to download. Library feeds, covers, and downloads enforce the same [library access](./users-and-permissions.md), and return `403 Forbidden` for a library the user cannot access. OPDS remembers a successful sign-in for up to a minute, so a change to the user's role or library access, or deactivating the user, can take that long to reach a connected client.

## Catalog Structure

After the root catalog, clients follow these routes under `/opds/v1`:

| Catalog | Route |
|---------|-------|
| Library navigation | `/libraries/{libraryID}` |
| All books | `/libraries/{libraryID}/all` |
| Series list | `/libraries/{libraryID}/series` |
| Books in a series | `/libraries/{libraryID}/series/{seriesID}` |
| Authors list | `/libraries/{libraryID}/authors` |
| Books by an author | `/libraries/{libraryID}/authors/{authorName}` |
| Search | `/libraries/{libraryID}/search?q={query}` |
| Lists the user owns or has been shared | `/lists` |
| Books in a list | `/lists/{listID}` |
| Search within a list | `/lists/{listID}/search?q={query}` |

There are no separate genre, tag, or recently added routes. A list can hold books from several libraries; its feed shows only the books in libraries the user can access. The lists feed leaves out a list with nothing the user's Delivery Rules deliver from those libraries, and a list the user neither owns nor has been shared returns `404 Not Found`.

## Search and Sort

Search inside a library covers that library's books, and search inside a list covers only that list's books. Either search matches book titles, subtitles, book and file paths, authors, narrators, and series. Author, narrator, and series aliases are searchable. It does not search genres, tags, or descriptions.

The **All Books**, author, and search book feeds use the authenticated user's [saved sort](./browsing-search-bulk-actions.md#gallery-sort) for that library. Books within a series follow series-number order. Books in a list follow the list's default sort.

## Client Compatibility

Shisho includes tested compatibility behavior for KOReader. Other OPDS 1.2 clients may work, but client support for authentication, search, multiple acquisition links, and file types varies. Consult the client's documentation when adding the catalog.

## Troubleshooting

### Repeated Login Prompts or 401 Responses

**Symptom:** The root catalog opens, but a sub-feed or download prompts again or returns `401 Unauthorized`.

**Likely cause:** The reverse proxy dropped the `Authorization` header or generated an incorrect scheme. An HTTP-to-HTTPS redirect can make an OPDS client drop Basic Auth.

**Verify:** Confirm that `/opds/` reaches Shisho without a redirect and that generated links use the external HTTPS scheme.

**Fix:** Forward `/opds/`, preserve `Authorization` and the public `Host`, and pass the original scheme with `X-Forwarded-Proto: https`. Shisho trusts forwarded headers only from private-network direct peers, so the proxy's connection to Shisho must meet the [forwarded-header trust rules](./deployment-and-maintenance.md#https-and-reverse-proxies). OPDS also honors `X-Forwarded-Prefix` from a trusted peer when a proxy strips a catalog URL prefix; this does not make a path-prefixed deployment of the web interface supported. See [Troubleshooting](./troubleshooting.md) for other proxy symptoms.

### Missing Libraries or Books

**Symptom:** A library or expected book is absent from the catalog.

**Likely cause:** The account lacks library access, the user's [Delivery Rules](./supported-formats.md#delivery-rules) have no rule for the book's main file types, or the file is a supplement. A file can also be absent while its book is listed: only the files on a book's first matching rule are offered.

**Verify:** Check the user's library access and compare the book's main-file types with the rules under **User Settings > Delivery Rules**.

**Fix:** Grant the intended [library access](./users-and-permissions.md) or add or reorder a rule for the required file type. The web app's **Download as** menu offers every file type regardless of the rules. Supplements do not appear as acquisitions. See [Libraries, Scanning, and File Organization](./libraries.md) and [Supplement Files](./supplement-files.md).
