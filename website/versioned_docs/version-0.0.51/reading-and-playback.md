# Reading and Playback

Users with **Books Read** permission and access to a book's library can open supported files directly in Shisho. Select **Read** for EPUB, CBZ, and PDF files or **Listen** for M4B files.

Reader preferences are saved to your Shisho account and follow you between browsers. Reading and listening positions are not saved as progress records.

## EPUB Reader

The EPUB reader provides:

- Table-of-contents navigation
- Previous and next page controls
- A progress bar that can jump within the book
- **Paginated** and **Scrolled** flow modes
- **Light**, **Dark**, and **Sepia** themes
- Font sizes from 50% to 200%
- Optional automatic hiding of the header and footer

Use the **Settings** button inside the reader to change font size, theme, flow, and control visibility. In paginated mode, click or tap the left and right sides to change pages. The Left Arrow or `A` moves back; the Right Arrow or `D` moves forward.

EPUB reading position is not persisted. Reopening a book starts from the beginning.

## CBZ and PDF Readers

CBZ and PDF use the same page-based reader. It provides:

- Page and chapter navigation
- A clickable page progress bar
- **Fit Height** and **Fit Width** display modes
- Configurable preloading of nearby pages
- Optional automatic hiding of controls

The current page is stored in the URL as `?page=`, so refreshing or bookmarking that URL returns to the same page. It is not stored as account-level reading progress.

Use the Left Arrow or `A` for the previous page and the Right Arrow or `D` for the next page.

## M4B Player

The audiobook player provides:

- Play, pause, and seek controls
- 30-second skip controls
- Chapter navigation when chapters are available
- Playback speeds from 0.5x to 3x

The selected playback speed is saved to your account. Listening position is not saved, so reopening an audiobook starts from the beginning.

Browser audio support depends on the file's codec. AAC-LC and HE-AAC are broadly supported. xHE-AAC works most reliably in Safari and iOS browsers; Firefox cannot play it, and Chrome cannot play it through Shisho's progressive stream. See [Supported Formats](./supported-formats.md#audiobook-browser-compatibility).

## Reader Settings

Open the user menu and select **User Settings** for shared reader preferences such as **Auto-hide controls**. Format-specific settings are available from the reader itself.
