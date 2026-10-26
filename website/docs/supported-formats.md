# Supported Formats

Shisho has native support for seven main-file formats. See [Reading and Playback](./reading-and-playback.md) for the in-app readers, [Metadata](./metadata.md) for the fields Shisho manages, and [Getting Started](./getting-started.md) for library setup.

## Capability Matrix

| Format | Import | Metadata Extraction | In-App Reader or Player | Generated Download |
|--------|--------|---------------------|-------------------------|--------------------|
| **EPUB** | Yes | Package metadata, navigation, and embedded cover data | [Ebook reader](./reading-and-playback.md#epub-mobi-and-azw3-reader) | EPUB with supported current metadata and [edited chapters](./metadata.md#chapters) applied |
| **KePub** (`.kepub.epub`, `.kepub`) | Yes | The same as EPUB; see [KePub](#kepub) | [Ebook reader](./reading-and-playback.md#epub-mobi-and-azw3-reader) | KePub with supported current metadata and [edited chapters](./metadata.md#chapters) applied, Kobo markup kept |
| **CBZ** | Yes | `ComicInfo.xml`, page images, and detected chapters | Comic reader | CBZ with supported current metadata applied and chapters written as [folders](#cbz-downloads) |
| **M4B** | Yes | Audiobook metadata, chapters, audio details, and embedded cover data | Audiobook player | M4B with supported current metadata applied |
| **PDF** | Yes | Document metadata, page count, bookmarks, and a rendered cover | PDF reader | PDF with supported current metadata and bookmarks applied |
| **MOBI** (`.mobi`, `.azw`, `.prc`) | Yes | Embedded metadata, table of contents, and cover; see [MOBI and AZW3](#mobi-and-azw3) | [Ebook reader](./reading-and-playback.md#epub-mobi-and-azw3-reader) | MOBI with supported current metadata, cover, and [edited chapters](./metadata.md#chapters) applied |
| **AZW3** (`.azw3`) | Yes | Embedded metadata, table of contents, and cover; see [MOBI and AZW3](#mobi-and-azw3) | [Ebook reader](./reading-and-playback.md#epub-mobi-and-azw3-reader) | AZW3 with supported current metadata, cover, and [edited chapters](./metadata.md#chapters) applied |

Generated downloads are format-specific. Each format can represent a different set of metadata, so Shisho cannot write every database field or replace a cover in every generated file. The source file is not modified.

Downloads are sent with the format's media type, whatever the server's operating system knows about file extensions: `application/epub+zip` for EPUB and KePub, `application/vnd.comicbook+zip` for CBZ, `audio/mp4` for M4B, `application/pdf` for PDF, `application/x-mobipocket-ebook` for MOBI, and `application/vnd.amazon.mobi8-ebook` for AZW3. [Kobo Sync](./kobo-sync.md) downloads are sent as `application/octet-stream`. Other files, such as supplements, are typed by their extension.

The OPDS, eReader, Kobo Sync, and Share Link downloads send the unmodified file when a generated one cannot be made: for a supplement, a format only a plugin can read, or a file whose contents Shisho cannot rewrite, such as a damaged EPUB downloaded as EPUB. The failure is written to the server log. A source file Shisho cannot read, for example because of its permissions, fails the download with an error instead. So does a failed [conversion](#conversions) through OPDS, the eReader Browser, Kobo Sync, or a Share Link's **Download as** menu, such as a KePub made from a damaged EPUB, because the original is a different file type than the one the [Delivery Rules](#delivery-rules) or the recipient picked; the server log names the file. No device sends the original file for a failed conversion. In the web app, a failed download reports the error, and its **Download unmodified file** button sends the file as it is on disk. See [Troubleshooting](./troubleshooting.md#downloads-or-readers-fail-or-cache-usage-is-high).

CBR is not a native format. [Using Plugins](./plugins/overview.md) may add parsers or converters for CBR and other formats.

## Download Filenames

A generated download is named `Author - Series 002 - Title.epub`, and an audiobook adds its narrator: `Author - Series 002 - Title (Narrator).m4b`. The author and series are the book's first ones, and the title is the file's **Name**, or the book's title when the file has no name. A part with no value is left out. When the title has its own volume number, such as `Manga v1`, the series is left out unless the book covers a range of the series, and the volume number is padded to three digits (`Manga v001`). Series numbers are padded the same way (`Series 002`, `Series 001.5`), so a series sorts in order by filename. The unmodified file downloads under the same name with the file's own extension, and a device or Share Link that gets the original file in place of a generated one uses its file type's extension, such as `.kepub.epub` for a KePub. A supplement is named after its book, followed by its own name, such as `Author - Series 002 - Title - Map.pdf`, so a book's files sort together; a supplement name with nothing left after the rules below becomes `File` and the file's ID. A Kindle names its downloads from the link instead, as [eReader Browser](./ereader-browser.md) describes.

Accented letters become plain ones, so `Les Misérables` downloads as `Les Miserables` and `Straße` as `Strasse`. Characters that are invalid in filenames or that shells and URLs treat specially are removed: ``/ \ : * ? " < > | [ ] { } # $ % ` `` and invisible control characters. Letters of other scripts, such as Japanese or Cyrillic, are kept. A name that would be too long for a filesystem is shortened, its title first. A leading dot is dropped, so the file is not hidden, and a name Windows reserves for a device, such as `Con`, gets a leading `_`.

A KePub's name keeps only the letters A to Z, digits, spaces, and `-_.,()'`, because other characters can stop a Kobo from reading the file, so other scripts are dropped from it. A KePub whose title has none of those characters is named `File` and the file's ID, such as `Author - File 42.kepub.epub`. The book's metadata keeps every character. A download whose name has characters outside ASCII also carries an ASCII name, which an app that cannot handle the full name saves the file under instead. It is built the same way as a KePub's name.

## First Downloads

A generated download starts as soon as it is requested and is built while it is sent. EPUB, KePub, M4B, MOBI, AZW3, and PDF downloads of a file as its own type know their size before they start, so the device or browser shows progress even the first time. An EPUB converted from an AZW3 or MOBI does too. An AZW3 converted from another file type is built in full before its download starts, so it shows its size, but its first download waits until the whole book is built. A CBZ, a KePub converted from another file type, and an EPUB made from a CBZ or a KePub do not: their first download has no file size, so the device or browser cannot show a percentage, and an interrupted download starts over. Later downloads of the same file show their size while it stays in the download cache.

## KePub

A KePub is the EPUB variant Kobo's own reader is built for. Shisho imports any file named `.kepub.epub` as a **KePub**, and every other `.epub` as an EPUB. A file named just `.kepub`, as Calibre stores a KePub in its library, is a KePub too. A scan reads a KePub's metadata, chapters, and cover the same way it reads an EPUB's. Organizing a library keeps the file's own extension, `.kepub.epub` or `.kepub`, and so does downloading the unmodified file. A generated KePub download is always named `.kepub.epub`, the name Kobo's reader and KOReader recognize.

A KePub downloaded as KePub gets Shisho's current metadata, cover, and chapter edits, the same as an EPUB download, and keeps the Kobo markup inside it, so it still opens in Kobo's own reader. A KePub can also be downloaded as a plain EPUB, without the Kobo markup, for readers that aren't Kobos. A KePub converted from another file type is a different download. See [Conversions](#conversions) for both.

DRM-protected KePubs, such as books bought from the Kobo store and copied off a Kobo, are never imported. Each one is named in the scan's job log; see [Troubleshooting](./troubleshooting.md#a-drm-protected-file-never-appears).

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

A generated download is named with its file type, so an `.azw` or `.prc` file downloads as `.mobi`. When a download fails, the web app offers **Download unmodified file**, which sends the file as it is on disk, without Shisho's metadata, with its own extension.

DRM-protected files are never imported. Each one is named in the scan's job log; see [Troubleshooting](./troubleshooting.md#a-drm-protected-file-never-appears). KFX, Topaz, and other Kindle formats that are not MOBI or AZW3 are skipped, even when they use the `.azw` extension.

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
| **KePub** | **EPUB** | A plain EPUB with the book's metadata and the Kobo markup removed, for readers that aren't Kobos |
| **AZW3** | **EPUB** | An EPUB 3 with the book's metadata, for readers that open EPUB but not Kindle files |
| **MOBI** | **EPUB** | The same |
| **EPUB** | **AZW3** | A Kindle book (KF8) with the book's metadata, for Kindles and the Kindle apps |

Conversions chain, so an AZW3 or MOBI also reaches KePub through EPUB, and a KePub, CBZ, or MOBI reaches AZW3 through EPUB. M4B and PDF files download only as themselves.

An AZW3 or MOBI converted to EPUB keeps its text, images, stylesheets, embedded fonts, internal links, and table of contents. A `.mobi` that holds both an older Mobipocket book and a newer KF8 book converts from the KF8 book. A `.mobi` that holds only an older Mobipocket book keeps its formatting, such as indents, alignment, and font sizes. A file that cannot be converted, such as a damaged one or one with an image over 64 MB, fails instead of sending a broken EPUB, and the web app and Share Links say why.

An EPUB converted to AZW3 keeps its text, images, stylesheets, cover, table of contents, links between places in the book (footnotes included), and embedded TrueType and OpenType fonts, including fonts the EPUB obfuscates. The book's start and landmarks become the Kindle's **Go To** entries, such as **Beginning**, and a new download opens at the start. It carries the metadata a [MOBI or AZW3 download](#mobi-and-azw3) writes, except the ASIN: its ASIN record holds the identifier described below. Kindles show no WOFF fonts, so a WOFF font is left out and its text uses the Kindle's own font. A comic converted from CBZ becomes a fixed-layout AZW3, one page per screen. Each file converted to AZW3 keeps one identifier across regenerations, so after a metadata edit a Kindle opens the new download where the reader left off.

A KePub converted to EPUB keeps its text, images, and internal links; a link or table of contents entry that pointed inside a paragraph goes to the start of that paragraph. A comic converted from CBZ keeps every page in reading order, uses the file's cover page as its cover, and lists its chapters in the table of contents. Pages are resized to fit an e-reader screen, and PNG pages become JPEG.

Each file type a file is downloaded as is generated and cached on its own, so downloading a CBZ as EPUB does not replace its cached CBZ or KePub. Editing the book's metadata regenerates each of them on its next download.

In the web app, each file's download button has a menu beside it, **Download as**, headed by the file's title, that lists every file type the file can become, its own first, marked **original**, and the rest marked **converted**. The download button itself follows your [Delivery Rules](#delivery-rules). [Share Links](./sharing.md#what-recipients-see) offer the same menu. See [Kobo Sync](./kobo-sync.md), [eReader Browser](./ereader-browser.md), and [OPDS Catalog](./opds.md) for the file types each device receives.

## Delivery Rules

Each user's default Delivery Rules decide which file type the web app's download button sends and what the [OPDS catalog](./opds.md) offers when signed in with the account password. Edit them under **Preferences > Delivery Rules**. Each [Device](./users-and-permissions.md#delivery-rules-of-a-device) follows your default, follows its type's preset, or has its own list, which decides what its eReader browser and OPDS catalog offer and what [Kobo Sync](./kobo-sync.md#what-syncs) sends.

The rules are an ordered list with at most one rule per file type, each naming the file type to send it as: itself, or one a [conversion](#conversions) reaches, such as CBZ as EPUB. For each book, the first rule matching one of its files wins, and every file of that type goes, so two EPUB editions both appear. Ebooks and audiobooks are decided separately, so a book with an EPUB and an M4B delivers both when both have a rule.

Until you edit them, your rules follow Shisho's defaults, which send every file type as itself in this order: EPUB, KePub, AZW3, MOBI, CBZ, PDF, M4B. Under the defaults, a book with an EPUB and a PDF offers only the EPUB over OPDS. Saving any change makes the list your own; a later Shisho release that changes the defaults then no longer changes your list. **Reset to Shisho's defaults** makes it follow them again. A list needs at least one rule.

A file type with no rule is left out of OPDS and the eReader browser, along with books, series, and authors that have nothing left to offer. The web app still downloads such a file as itself, and its **Download as** menu always offers every file type.

## Audiobook Browser Compatibility

Most M4B files use AAC-LC or HE-AAC and play in current browsers. xHE-AAC playback is more limited: use Safari or an iOS browser for Shisho's direct audio stream. Firefox cannot play xHE-AAC, and Chrome does not support it in this progressive-streaming setup. In a browser that cannot play the file, the player shows a warning suggesting Safari, though its controls stay available. If broad browser playback matters, encode audiobooks as AAC-LC or HE-AAC.
