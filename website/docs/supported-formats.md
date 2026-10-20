# Supported Formats

Shisho has native support for six main-file formats. See [Reading and Playback](./reading-and-playback.md) for the in-app readers, [Metadata](./metadata.md) for the fields Shisho manages, and [Getting Started](./getting-started.md) for library setup.

## Capability Matrix

| Format | Import | Metadata Extraction | In-App Reader or Player | Generated Download |
|--------|--------|---------------------|-------------------------|--------------------|
| **EPUB** | Yes | Package metadata, navigation, and embedded cover data | [Ebook reader](./reading-and-playback.md#epub-mobi-and-azw3-reader) | EPUB with supported current metadata and [edited chapters](./metadata.md#chapters) applied |
| **CBZ** | Yes | `ComicInfo.xml`, page images, and detected chapters | Comic reader | CBZ with supported current metadata applied and chapters written as [folders](#cbz-downloads) |
| **M4B** | Yes | Audiobook metadata, chapters, audio details, and embedded cover data | Audiobook player | M4B with supported current metadata applied |
| **PDF** | Yes | Document metadata, page count, bookmarks, and a rendered cover | PDF reader | PDF with supported current metadata and bookmarks applied |
| **MOBI** (`.mobi`, `.azw`, `.prc`) | Yes | Embedded metadata, table of contents, and cover; see [MOBI and AZW3](#mobi-and-azw3) | [Ebook reader](./reading-and-playback.md#epub-mobi-and-azw3-reader) | MOBI with supported current metadata, cover, and [edited chapters](./metadata.md#chapters) applied |
| **AZW3** (`.azw3`) | Yes | Embedded metadata, table of contents, and cover; see [MOBI and AZW3](#mobi-and-azw3) | [Ebook reader](./reading-and-playback.md#epub-mobi-and-azw3-reader) | AZW3 with supported current metadata, cover, and [edited chapters](./metadata.md#chapters) applied |

Generated downloads are format-specific. Each format can represent a different set of metadata, so Shisho cannot write every database field or replace a cover in every generated file. The source file is not modified.

Downloads are sent with the format's media type, whatever the server's operating system knows about file extensions: `application/epub+zip` for EPUB and KePub, `application/vnd.comicbook+zip` for CBZ, `audio/mp4` for M4B, `application/pdf` for PDF, `application/x-mobipocket-ebook` for MOBI, and `application/vnd.amazon.mobi8-ebook` for AZW3. [Kobo Sync](./kobo-sync.md) downloads are sent as `application/octet-stream`. Other files, such as supplements, are typed by their extension.

The OPDS, eReader, Kobo Sync, and Share Link downloads send the original file when a generated one cannot be made: for a supplement, a format only a plugin can read, or a file whose contents Shisho cannot rewrite, such as a damaged EPUB downloaded as EPUB. The failure is written to the server log. A source file Shisho cannot read, for example because of its permissions, fails the download with an error instead. So does a failed [conversion](#conversions) through OPDS, the eReader Browser, Kobo Sync, or a Share Link's **Download as** menu, such as a KePub made from a damaged EPUB, because the original is a different file type than the one the [Delivery Rules](#delivery-rules) or the recipient picked; the server log names the file. No device sends the original file for a failed conversion. In the web app, a failed download reports an error, and **Download Original** sends the file as it is. See [Troubleshooting](./troubleshooting.md#downloads-or-readers-fail-or-cache-usage-is-high).

CBR is not a native format. [Using Plugins](./plugins/overview.md) may add parsers or converters for CBR and other formats.

## First Downloads

A generated download starts as soon as it is requested and is built while it is sent. EPUB, M4B, MOBI, AZW3, and PDF downloads know their size before they start, so the device or browser shows progress even the first time. A CBZ, a KePub, and an EPUB made from a CBZ do not: their first download has no file size, so the device or browser cannot show a percentage, and an interrupted download starts over. Later downloads of the same file show their size while it stays in the download cache.

## MOBI and AZW3

Kindle files are imported as two file types: **MOBI** for `.mobi`, `.azw`, and `.prc` files, and **AZW3** for `.azw3` files. A file keeps its own extension on disk, so an organized `.azw` file is still an `.azw`. A `.mobi` that holds both an older Mobipocket book and a newer KF8 book, as Calibre writes with its "both" output type, is read from the KF8 book.

A scan reads these fields from the file:

- Title
- Authors, with "Last, First" names such as `Doe, Jane` turned into `Jane Doe`
- Description, with HTML removed
- Publisher, using the imprint when the file has one
- Release date and language
- ISBN and ASIN identifiers
- Subjects, as genres, split on `;`
- The embedded cover
- The table of contents, as chapters with their nesting. Calibre writes the table of contents of an older Mobipocket book flat, so a `.mobi` converted by Calibre without a KF8 book has no nested chapters.

Series, tags, and author roles have no place in these formats. Set them through [sidecar files](./sidecar-files.md), [metadata plugins](./plugins/overview.md), or your own edits.

Downloads write the same fields back into the file, with Shisho's current values: title, authors, description, publisher, release date, language, ISBN and ASIN, genres, and the cover. A field Shisho has no value for keeps the value already in the file. Series, tags, author roles, and narrators are not written, and the series is never added to the title. In a combo `.mobi`, both books get the new metadata and [chapter edits](./metadata.md#chapters), so a Kindle shows them whichever book it opens. A file with no cover gets one. Covers are written as JPEG, because older Kindles cannot show a PNG. Some older files store text as Windows-1252; a character that encoding cannot hold, such as one from a non-Latin script, is written as `?` in those files.

A generated download is named with its file type, so an `.azw` or `.prc` file downloads as `.mobi`. **Download Original** keeps the file's own name.

DRM-protected files are never imported. Each one is named in the scan's job log; see [Troubleshooting](./troubleshooting.md#a-drm-protected-mobi-or-azw3-file-never-appears). KFX, Topaz, and other Kindle formats that are not MOBI or AZW3 are skipped, even when they use the `.azw` extension.

## CBZ Page Images

Native CBZ parsing recognizes these page image formats:

- PNG
- JPEG (`.jpg` and `.jpeg`)
- WebP
- GIF

Pages are read in file name order, folder names included, with numbers compared by value: `page2.jpg` comes before `page10.jpg`, and `Chapter 2/` before `Chapter 10/`. macOS metadata such as `._page1.jpg` files and anything under `__MACOSX/` is not a page. The reader, the `ComicInfo.xml` cover index, and KePub and EPUB downloads of a CBZ count pages in this order, and so do the cover page and chapter start pages that a scan records. A comic scanned by an earlier version can keep cover page and chapter numbers counted the old way; see [Troubleshooting](./troubleshooting.md#a-comics-cover-page-or-chapters-point-at-the-wrong-page).

## CBZ Downloads

A downloaded CBZ holds the file's chapters as folders: each chapter's pages go in a folder named after the chapter, and pages before the first chapter stay at the top level. KOReader lists these folders as the comic's chapters. Scanning the download into Shisho finds the same chapters when there are two or more, since a scan reads chapters only from two or more folders. Only top-level chapters are written, and when several chapters start on the same page, only the first is kept. When the chapter names would not sort in reading order, such as `Prologue` before `Chapter 1`, every folder is numbered in reading order, as in `1. Prologue` and `2. Chapter 1`, and a scan leaves the numbers out of the chapter names. A scan also leaves out numbers you added yourself when every chapter folder is numbered `1.`, `2.`, `3.` in order. Folders named only by a number, such as `1` and `2`, keep their names. Rewritten pages are named after their page numbers. A comic with no chapters, or whose folders already match its chapters, keeps its own folder and page names.

## PDF Bookmarks

Scanning a PDF turns its bookmarks into chapters, flattened into a single list. A bookmark that does not point at a page is skipped. Its nested bookmarks are still imported when they point at pages.

## PDF Downloads

A downloaded PDF is the original file, unchanged, with Shisho's metadata and bookmarks appended to the end as an update. PDF readers show the appended title, authors, description, tags, language, and chapters. Because the original bytes are kept, the file is not re-optimized, so it can be slightly larger than the original. A PDF optimized for fast web viewing (linearized) loses that optimization, so a browser may need the whole file before it shows the first page. A digital signature still covers the original bytes and verifies, though PDF readers report the document as changed after signing, and a certified PDF that permits no changes is shown as altered.

A permission-restricted PDF, one that opens without a password but limits actions such as printing or copying, gets Shisho's metadata too, and keeps its original encryption and permissions. Shisho cannot add its metadata to a PDF that needs a password to open or to one whose internal structure it cannot read, so these count as files whose contents Shisho cannot rewrite, and downloads of them [fall back as described above](#capability-matrix).

## Conversions

A file can be downloaded as its own file type or as another one Shisho converts it to while the download is sent. The file in your library is not changed. These conversions exist:

| From | To | Result |
|------|----|--------|
| **EPUB** | **KePub** | A Kobo-optimized EPUB with the book's metadata, named `.kepub.epub` |
| **CBZ** | **KePub** | A fixed-layout KePub with one page per image, named `.kepub.epub` |
| **CBZ** | **EPUB** | A fixed-layout EPUB 3 with one page per image, for readers that open EPUB but not CBZ |

M4B, PDF, MOBI, and AZW3 files download only as themselves. A comic converted from CBZ keeps every page in reading order, uses the file's cover page as its cover, and lists its chapters in the table of contents. Pages are resized to fit an e-reader screen, and PNG pages become JPEG.

Each file type a file is downloaded as is generated and cached on its own, so downloading a CBZ as EPUB does not replace its cached CBZ or KePub. Editing the book's metadata regenerates each of them on its next download.

In the web app, each file's download button has a menu beside it, **Download as**, headed by the file's title, that lists every file type the file can become, its own first. The download button itself follows your [Delivery Rules](#delivery-rules). [Share Links](./sharing.md#what-recipients-see) offer the same menu. See [Kobo Sync](./kobo-sync.md), [eReader Browser](./ereader-browser.md), and [OPDS Catalog](./opds.md) for the file types each device receives.

## Delivery Rules

Each user's default Delivery Rules decide which file type the web app's download button sends and what the [OPDS catalog](./opds.md) offers when signed in with the account password. Edit them under **User Settings > Delivery Rules**. Each [Device](./users-and-permissions.md#delivery-rules-of-a-device) follows your default, follows its type's preset, or has its own list, which decides what its eReader browser and OPDS catalog offer and what [Kobo Sync](./kobo-sync.md#what-syncs) sends.

The rules are an ordered list with at most one rule per file type, each naming the file type to send it as: itself, or one a [conversion](#conversions) reaches, such as CBZ as EPUB. For each book, the first rule matching one of its files wins, and every file of that type goes, so two EPUB editions both appear. Ebooks and audiobooks are decided separately, so a book with an EPUB and an M4B delivers both when both have a rule.

Until you edit them, your rules follow Shisho's defaults, which send every file type as itself in this order: EPUB, AZW3, MOBI, CBZ, PDF, M4B. Under the defaults, a book with an EPUB and a PDF offers only the EPUB over OPDS. Saving any change makes the list your own; a later Shisho release that changes the defaults then no longer changes your list. **Reset to Shisho's defaults** makes it follow them again. A list needs at least one rule.

A file type with no rule is left out of OPDS and the eReader browser, along with books, series, and authors that have nothing left to offer. The web app still downloads such a file as itself, and its **Download as** menu always offers every file type.

## Audiobook Browser Compatibility

Most M4B files use AAC-LC or HE-AAC and play in current browsers. xHE-AAC playback is more limited: use Safari or an iOS browser for Shisho's direct audio stream. Firefox cannot play xHE-AAC, and Chrome does not support it in this progressive-streaming setup. In a browser that cannot play the file, the player shows a warning suggesting Safari, though its controls stay available. If broad browser playback matters, encode audiobooks as AAC-LC or HE-AAC.
