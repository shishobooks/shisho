# Reading and Playback

Users with Books Read permission and access to a book's library can open supported files directly in Shisho. Select **Read** for EPUB, KePub, MOBI, AZW3, CBZ, and PDF files or **Listen** for M4B files.

Reader preferences are saved to your Shisho account and follow you between browsers, except **Follow Reading Direction** and the ebook reader's [display settings](#display-settings), which each browser keeps on its own. Every reader also remembers your place and keeps your reading history current, as described in [Reading in Shisho](./reading-tracking.md#reading-in-shisho).

## EPUB, MOBI, and AZW3 Reader

EPUB, KePub, MOBI, and AZW3 files open in the same reader, which provides:

- Table-of-contents navigation
- Previous and next page controls
- A progress bar that can jump within the book, marked where each chapter in the book's table of contents starts. Point at a mark to see the chapter's name, or select it to go to the start of that chapter.
- [Display settings](#display-settings) for the font, size, spacing, margins, theme, and layout
- Footnotes that open in a popover beside the text, in the page's theme and in the font chosen under [display settings](#display-settings). With **Book's font**, a book's embedded fonts may show as a standard serif font in the popover. Select **Go to note** in the popover to jump to the full note.
- Optional automatic hiding of the header and footer

In paginated mode, click or tap the left and right sides to change pages, or swipe from anywhere on the page. Fixed-layout books always turn this way, whatever the layout setting. The Left Arrow or `A` moves back; the Right Arrow or `D` moves forward. In a book that reads right to left, such as many Arabic, Hebrew, and Japanese books, the sides swap: the left side, the Left Arrow, and the left footer button move forward. When controls are hidden automatically, move the pointer to show them, or tap the middle of the page or its top or bottom edge (paginated) or the text (scrolled).

Reopening a book returns to where you left off. See [Resuming Where You Left Off](./reading-tracking.md#resuming-where-you-left-off).

### Display Settings

Use the **Settings** button inside the reader to change how books look:

| Setting | Options |
| --- | --- |
| **Font** | **Book's font** keeps the fonts the book sets. The included fonts are Libron, Literata, Atkinson Hyperlegible, Noto Serif, Noto Sans, and OpenDyslexic. You can also [import your own](#imported-fonts). |
| **Font size** | 10 to 40 px, in steps of 0.5 px. Headings and other text the book sizes relative to its body text scale with it. |
| **Force font size** | Off by default. Turn it on when the font size has no effect because the book sets its own text sizes. All text then uses the font size, except headings, small print, and code. Text the book sizes for effect, such as a large first letter, also takes the font size. |
| **Line spacing** | 1.00 to 2.50 times the font size, in steps of 0.05, from compact to relaxed |
| **Margins** | 1% to 15% of the page's width on each side, in steps of 1%, from narrow to wide. When two pages show side by side, the same space separates them. |
| **Justify text** | On justifies and hyphenates paragraphs; off aligns them to the start of the line. Text the book centers or aligns right, such as scene breaks, keeps its alignment either way. |
| **Theme** | **Light**, **Sepia**, **Dark**, or **Black**. Light keeps the book's own text and background colors; the others replace them. In Dark and Black, images get a white background so transparent images stay visible. |
| **Layout** | **Paginated** or **Scrolled** |
| **Pages** | **Auto** shows two pages side by side when the reader is wider than it is tall and has room for both. **One** always shows a single page, which stops widening at about 720 px so lines stay a comfortable length; past that, the margins have no further effect. Applies only to **Paginated**. |

A change applies right away and keeps your place in the book. Display settings are kept in your browser rather than your account, so each browser or device can have its own. They apply to every book you open in that browser, in all of these formats.

#### Imported Fonts

Select **Import font** to add a TTF, OTF, or WOFF2 font file you have. The font is stored in that browser and is never uploaded to Shisho, so import it again in each browser where you want it. Clearing the browser's data for Shisho also removes it. To remove a font, select the remove button next to its name, below the font list.

## CBZ and PDF Readers

CBZ and PDF use the same page-based reader. It provides:

- Page and chapter navigation
- A clickable page progress bar, marked where each chapter starts. Point at a mark to see the chapter's name, or select it to go to the chapter's first page.
- **Fit Height** and **Fit Width** display modes
- Configurable preloading of nearby pages
- Optional automatic hiding of controls
- **Paginated** and **Scrolled** flow modes
- Right to left page turns for files that read that way, such as manga

A CBZ or PDF file's page shows its pages as a grid; select one to open the reader on that page.

Reopening a comic or PDF returns to the page you left. See [Resuming Where You Left Off](./reading-tracking.md#resuming-where-you-left-off). The current page is also in the URL as `?page=`, so you can bookmark or share a link to a page.

Use the Left Arrow or `A` for the previous page and the Right Arrow or `D` for the next page. In the **Paginated** flow, you can also tap the left and right sides or swipe from anywhere on the page. In the **Scrolled** flow, the Up and Down Arrows, Space, and Page Up and Page Down scroll the pages.

A file whose [Reading Direction](./metadata.md#reading-direction) is right to left turns the other way: the left side, a swipe to the right, the Left Arrow or `A`, and the left footer button move forward, the next page slides in from the left, and the progress bar fills from the right. The **Scrolled** flow runs top to bottom either way. To have every file turn left to right, turn off **Follow Reading Direction** in the reader settings. It is on until you turn it off, and the browser remembers your choice.

### Scrolled Flow for Webtoons

Comics made for vertical scrolling, often called webtoons, are usually stored as tall strips rather than printed-size pages. In the **Scrolled** flow, the reader shows every page in one continuous column that you scroll through, and the page counter and `?page=` follow your position.

The flow is saved per book for each user, from the reader settings, and applies to both CBZ and PDF files. Until you choose one for a book, the reader checks the shape of the first two pages after the cover: if either is at least 2.5 times taller than it is wide, the book opens in the **Scrolled** flow, and otherwise in the **Paginated** flow.

## M4B Player

The audiobook player provides:

- Play, pause, and seek controls
- 30-second skip controls
- Chapter navigation when chapters are available, with each chapter's start marked on the seek bar. Point at a mark to see the chapter's name, or select it to go to the start of that chapter.
- Playback speed from 0.5x to 4x
- Bookmarks

Opening an audiobook returns to where you stopped and starts playing. If your browser blocks that, as it can when you open the player's address directly, press play. An audiobook you reached the very end of stays paused. See [Resuming Where You Left Off](./reading-tracking.md#resuming-where-you-left-off).

Browser audio support depends on the file's codec, and some audiobooks play only in certain browsers. See [Audiobook Browser Compatibility](./supported-formats.md#audiobook-browser-compatibility).

### Playback Speed

The minus and plus buttons beside the speed change it by 0.1, from 0.5x to 4x. Select the speed itself to pick a common one from a menu.

Each book remembers its own speed in the browser you set it in, so a fast narrator and a slow one each keep theirs. A book you have not set a speed for starts at 1x. The speed is not saved to your account, so another browser or device starts that book at 1x until you set it there.

### Bookmarks

A bookmark marks a moment in an audiobook to come back to. Select **Add Bookmark** to mark the current moment, and add a note if you like. Playback carries on while you write it.

Select **Bookmarks** to list an audiobook's bookmarks in time order. Select a bookmark to jump to its moment, or use its buttons to edit its note or delete it. Deleting a bookmark takes effect at once and cannot be undone.

Each bookmark is also marked on the seek bar with a dot. Point at a dot to see the bookmark's time and note, or select it to go to that moment.

Bookmarks are saved to your account, so they follow you to any browser. They are private: other users never see yours. Bookmarks belong to one audiobook file, and deleting that file from Shisho deletes its bookmarks too.

## Reader Settings

Open the user menu and select **Preferences** for shared reader preferences such as **Auto-hide controls**. Format-specific settings are available from the reader itself.
