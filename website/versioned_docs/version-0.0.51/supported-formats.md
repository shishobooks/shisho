# Supported Formats

Shisho has native support for four main-file formats. See [Reading and Playback](./reading-and-playback.md) for the in-app readers, [Metadata](./metadata.md) for the fields Shisho manages, and [Getting Started](./getting-started.md) for library setup.

## Capability Matrix

| Format | Import | Metadata Extraction | In-App Reader or Player | Generated Download |
|--------|--------|---------------------|-------------------------|--------------------|
| **EPUB** | Yes | Package metadata, navigation, and embedded cover data | EPUB reader | EPUB with supported current metadata applied |
| **CBZ** | Yes | `ComicInfo.xml`, page images, and detected chapters | Comic reader | CBZ with supported current metadata applied |
| **M4B** | Yes | Audiobook metadata, chapters, audio details, and embedded cover data | Audiobook player | M4B with supported current metadata applied |
| **PDF** | Yes | Document metadata, page count, bookmarks, and a rendered cover | PDF reader | PDF with supported current metadata and bookmarks applied |

Generated downloads are format-specific. Each format can represent a different set of metadata, so Shisho cannot write every database field or replace a cover in every generated file. The source file is not modified.

CBR is not a native format. [Using Plugins](./plugins/overview.md) may add parsers or converters for CBR and other formats.

## CBZ Page Images

Native CBZ parsing recognizes these page image formats:

- PNG
- JPEG (`.jpg` and `.jpeg`)
- WebP
- GIF

## KePub Generation

Shisho can generate Kobo-optimized KePub downloads from **EPUB and CBZ only**. M4B and PDF remain in their native formats. See [Kobo Sync](./kobo-sync.md), [eReader Browser](./ereader-browser.md), and [OPDS Catalog](./opds.md) for device delivery options.

## Audiobook Browser Compatibility

Most M4B files use AAC-LC or HE-AAC and play in current browsers. xHE-AAC playback is more limited: use Safari or an iOS browser for Shisho's direct audio stream. Firefox cannot play xHE-AAC, and Chrome does not support it in this progressive-streaming setup. If broad browser playback matters, encode audiobooks as AAC-LC or HE-AAC.
