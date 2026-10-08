# Supported Formats

Shisho has native support for four main-file formats. See [Reading and Playback](./reading-and-playback.md) for the in-app readers, [Metadata](./metadata.md) for the fields Shisho manages, and [Getting Started](./getting-started.md) for library setup.

## Capability Matrix

| Format | Import | Metadata Extraction | In-App Reader or Player | Generated Download |
|--------|--------|---------------------|-------------------------|--------------------|
| **EPUB** | Yes | Package metadata, navigation, and embedded cover data | EPUB reader | EPUB with supported current metadata and [edited chapters](./metadata.md#chapters) applied |
| **CBZ** | Yes | `ComicInfo.xml`, page images, and detected chapters | Comic reader | CBZ with supported current metadata applied |
| **M4B** | Yes | Audiobook metadata, chapters, audio details, and embedded cover data | Audiobook player | M4B with supported current metadata applied |
| **PDF** | Yes | Document metadata, page count, bookmarks, and a rendered cover | PDF reader | PDF with supported current metadata and bookmarks applied |

Generated downloads are format-specific. Each format can represent a different set of metadata, so Shisho cannot write every database field or replace a cover in every generated file. The source file is not modified.

Downloads are sent with the format's media type, whatever the server's operating system knows about file extensions: `application/epub+zip` for EPUB and KePub, `application/vnd.comicbook+zip` for CBZ, `audio/mp4` for M4B, and `application/pdf` for PDF. [Kobo Sync](./kobo-sync.md) downloads are sent as `application/octet-stream`. Other files, such as supplements, are typed by their extension.

The OPDS, eReader, Kobo Sync, and Share Link downloads send the original file when a generated one cannot be made: for a supplement, a format only a plugin can read, a KePub request for M4B or PDF, or a file whose contents Shisho cannot rewrite, such as a damaged EPUB. The failure is written to the server log. A source file Shisho cannot read, for example because of its permissions, fails the download with an error instead. In the web app, a failed download reports an error, and **Download Original** sends the file as it is. See [Troubleshooting](./troubleshooting.md#downloads-or-readers-fail-or-cache-usage-is-high).

CBR is not a native format. [Using Plugins](./plugins/overview.md) may add parsers or converters for CBR and other formats.

## CBZ Page Images

Native CBZ parsing recognizes these page image formats:

- PNG
- JPEG (`.jpg` and `.jpeg`)
- WebP
- GIF

Pages are read in file name order, folder names included, with numbers compared by value: `page2.jpg` comes before `page10.jpg`, and `Chapter 2/` before `Chapter 10/`. macOS metadata such as `._page1.jpg` files and anything under `__MACOSX/` is not a page. The reader, the `ComicInfo.xml` cover index, and KePub downloads count pages in this order, and so do the cover page and chapter start pages that a scan records. A comic scanned by an earlier version can keep cover page and chapter numbers counted the old way; see [Troubleshooting](./troubleshooting.md#a-comics-cover-page-or-chapters-point-at-the-wrong-page).

## PDF Bookmarks

Scanning a PDF turns its bookmarks into chapters, flattened into a single list. A bookmark that does not point at a page is skipped. Its nested bookmarks are still imported when they point at pages.

## KePub Generation

Shisho can generate Kobo-optimized KePub downloads from **EPUB and CBZ only**. M4B and PDF remain in their native formats. See [Kobo Sync](./kobo-sync.md), [eReader Browser](./ereader-browser.md), and [OPDS Catalog](./opds.md) for device delivery options.

## Audiobook Browser Compatibility

Most M4B files use AAC-LC or HE-AAC and play in current browsers. xHE-AAC playback is more limited: use Safari or an iOS browser for Shisho's direct audio stream. Firefox cannot play xHE-AAC, and Chrome does not support it in this progressive-streaming setup. In a browser that cannot play the file, the player shows a warning suggesting Safari, though its controls stay available. If broad browser playback matters, encode audiobooks as AAC-LC or HE-AAC.
