# Reading and Playback

Users with Books Read permission and access to a book's library can open supported files directly in Shisho. Select **Read** for EPUB, KePub, MOBI, AZW3, CBZ, and PDF files or **Listen** for M4B files.

Reader preferences are saved to your Shisho account and follow you between browsers. The EPUB, KePub, MOBI, and AZW3 reader also remembers your place and keeps your reading history current, as described in [Reading in Shisho](./reading-tracking.md#reading-in-shisho). CBZ, PDF, and M4B positions are not saved.

## EPUB, MOBI, and AZW3 Reader

EPUB, KePub, MOBI, and AZW3 files open in the same reader, which provides:

- Table-of-contents navigation
- Previous and next page controls
- A progress bar that can jump within the book
- **Paginated** and **Scrolled** flow modes
- **Light**, **Dark**, and **Sepia** themes. Light keeps the book's own text and background colors; Dark and Sepia replace them.
- Font sizes from 50% to 200%
- Optional automatic hiding of the header and footer

Use the **Settings** button inside the reader to change font size, theme, flow, and control visibility. These settings apply to all of these formats, so a theme chosen while reading a MOBI also applies to EPUBs. In paginated mode, click or tap the left and right sides to change pages. The Left Arrow or `A` moves back; the Right Arrow or `D` moves forward. When controls are hidden automatically, move the pointer to show them, or tap the middle of the page (paginated) or the text (scrolled).

Reopening a book returns to where you left off. See [Resuming Where You Left Off](./reading-tracking.md#resuming-where-you-left-off).

## CBZ and PDF Readers

CBZ and PDF use the same page-based reader. It provides:

- Page and chapter navigation
- A clickable page progress bar
- **Fit Height** and **Fit Width** display modes
- Configurable preloading of nearby pages
- Optional automatic hiding of controls
- **Paginated** and **Scrolled** flow modes

A CBZ or PDF file's page shows its pages as a grid; select one to open the reader on that page.

The current page is stored in the URL as `?page=`, so refreshing or bookmarking that URL returns to the same page. It is not stored as account-level reading progress.

Use the Left Arrow or `A` for the previous page and the Right Arrow or `D` for the next page. In the **Scrolled** flow, the Up and Down Arrows, Space, and Page Up and Page Down scroll the pages.

### Scrolled Flow for Webtoons

Comics made for vertical scrolling, often called webtoons, are usually stored as tall strips rather than printed-size pages. In the **Scrolled** flow, the reader shows every page in one continuous column that you scroll through, and the page counter and `?page=` follow your position.

The flow is saved per book for each user, from the reader settings, and applies to both CBZ and PDF files. Until you choose one for a book, the reader checks the shape of the first two pages after the cover: if either is at least 2.5 times taller than it is wide, the book opens in the **Scrolled** flow, and otherwise in the **Paginated** flow.

## M4B Player

The audiobook player provides:

- Play, pause, and seek controls
- 30-second skip controls
- Chapter navigation when chapters are available
- Playback speeds from 0.5x to 3x

The selected playback speed is saved to your account. Listening position is not saved, so reopening an audiobook starts from the beginning.

Browser audio support depends on the file's codec, and some audiobooks play only in certain browsers. See [Audiobook Browser Compatibility](./supported-formats.md#audiobook-browser-compatibility).

## Reader Settings

Open the user menu and select **Preferences** for shared reader preferences such as **Auto-hide controls**. Format-specific settings are available from the reader itself.
