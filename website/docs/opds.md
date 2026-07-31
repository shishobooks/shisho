# OPDS Catalog

Shisho provides an OPDS 1.2 catalog for browsing and downloading books from compatible reading apps.

## Catalog URLs

Use this root catalog URL:

```text
https://your-server/opds/v1/{types}/catalog
```

Replace `{types}` with one or more of `epub`, `cbz`, `m4b`, and `pdf`. Join multiple types with `+`:

```text
https://your-server/opds/v1/epub/catalog
https://your-server/opds/v1/epub+cbz/catalog
https://your-server/opds/v1/epub+cbz+m4b+pdf/catalog
```

Any non-empty combination of those four types is accepted. Order does not change the catalog behavior.

### KePub Catalog

To request KePub downloads where conversion is supported, insert `/kepub` before the type selection:

```text
https://your-server/opds/v1/kepub/epub+cbz/catalog
```

In a KePub catalog, EPUB and CBZ files are converted to KePub. M4B and PDF files remain in their native formats. See [Supported Formats](./supported-formats.md) for generated-download limitations.

## Authentication and Access

OPDS uses HTTP Basic Authentication. Add the catalog with the same Shisho username and password used for the web interface. Use HTTPS whenever the catalog is reachable outside a trusted local network.

The root catalog lists only libraries the authenticated user can access. Library feeds, covers, and downloads enforce the same [library access](./users-and-permissions.md).

## Catalog Structure

After the root catalog, clients follow these routes under `/opds/v1/{types}`:

| Catalog | Route |
|---------|-------|
| Library navigation | `/libraries/{libraryID}` |
| All books | `/libraries/{libraryID}/all` |
| Series list | `/libraries/{libraryID}/series` |
| Books in a series | `/libraries/{libraryID}/series/{seriesID}` |
| Authors list | `/libraries/{libraryID}/authors` |
| Books by an author | `/libraries/{libraryID}/authors/{authorName}` |
| Search | `/libraries/{libraryID}/search?q={query}` |

KePub catalogs use the same routes under `/opds/v1/kepub/{types}`. There are no separate genre, tag, recently added, or cross-library book routes.

## Search and Sort

Catalog search matches book titles, subtitles, book and file paths, authors, narrators, and series. Author, narrator, and series aliases are searchable. It does not search genres, tags, or descriptions.

The **All Books**, author, and search book feeds use the authenticated user's saved sort for that library. Without a saved sort, they use **Date Added, Newest First**. Books within a series follow series-number order. See [Browsing, Search, and Bulk Actions](./browsing-search-bulk-actions.md).

## Client Compatibility

Shisho includes tested compatibility behavior for KOReader. Other OPDS 1.2 clients may work, but client support for authentication, search, multiple acquisition links, and file types varies. Consult the client's documentation when adding the catalog.

## Troubleshooting

### Repeated Login Prompts or 401 Responses

**Symptom:** The root catalog opens, but a sub-feed or download prompts again or returns `401 Unauthorized`.

**Likely cause:** The reverse proxy dropped the `Authorization` header or generated an incorrect scheme. An HTTP-to-HTTPS redirect can make an OPDS client drop Basic Auth.

**Verify:** Confirm that `/opds/` reaches Shisho without a redirect and that generated links use the external HTTPS scheme.

**Fix:** Forward `/opds/`, preserve `Authorization`, and pass the original scheme with `X-Forwarded-Proto: https`. If another proxy is in front of Shisho's bundled Caddy server, configure it as a trusted proxy before relying on forwarded headers. See [Deployment and Maintenance](./deployment-and-maintenance.md) and [Troubleshooting](./troubleshooting.md).

### Missing Libraries or Books

**Symptom:** A library or expected book is absent from the catalog.

**Likely cause:** The account lacks library access, the `{types}` selection excludes the book's main formats, or the file is a supplement.

**Verify:** Check the user's library access and compare the book's main-file types with the type segment in the catalog URL.

**Fix:** Grant the intended [library access](./users-and-permissions.md) or update the catalog URL to include the required type. Supplements do not appear as acquisitions. See [Libraries, Scanning, and File Organization](./libraries.md) and [Supplement Files](./supplement-files.md).
