# Tracking Your Reading

Each user can record their reading of a book, rate it, and look back at its history. This record is private: other users, admins included, never see your statuses, history, or ratings.

## Reading Status

A book's **Reading Status** shows on its page, next to its actions, and as a bookmark hanging from the top of its cover, on its page and on its card wherever books are listed. Each status has its own bookmark color and icon, and hovering over the bookmark names it. It is one of:

| Status | Meaning |
|--------|---------|
| **Currently Reading** | You started the book and haven't finished or abandoned it. |
| **Paused** | You started the book and set it aside for now. |
| **To Read** | The book is on your [To Read](#to-read) queue. |
| **Read** | The last time you read the book, you finished it. |
| **Did Not Finish** | The last time you read the book, you abandoned it. |

A book you have never tracked has no status and no bookmark.

The status follows your history. A book you are reading or have paused shows that, whatever happened before. Otherwise a book on To Read shows **To Read**, even if you have read it before. Otherwise it shows how your latest read ended, where latest means the latest end date. A read with no end date counts as older than any dated one, so editing or deleting a Read-through can change the status.

## Changing the Status

On a book's page, open the status button (**Track reading** on a book you haven't tracked) and choose an action. The menu offers only the actions that make sense from the current status:

- **Add to To Read** or **Remove from To Read** puts the book at the end of your [To Read](#to-read) queue or takes it off. It isn't offered while you're reading the book or have paused it.
- **Start reading** begins a new read dated today. It's called **Read again** once you have read the book before, and the earlier reads stay in the history.
- **Pause** sets aside the book you're reading. **Resume** picks it back up with its original start date.
- **Mark as read** finishes the read you're on, dated [the day you last read it](#last-read). With nothing underway, it records a finished read that ended today with no start date, for a book you read before you used Shisho.
- **Mark as Did Not Finish** ends the read you're on as abandoned, dated the same way.
- **Mark as not started** takes back the read you're on, as if you had never started it: it's deleted from the history, every main file of the book opens at the start again, and a book that left To Read when you started it goes back to its place there.

"Today" is the date in [your time zone](#your-time-zone) when you choose the action. To correct a date afterwards, use **Edit dates** in [Reading History](#reading-history).

## Reading in Shisho

Every reader keeps your place and your reading history current as you read or listen, with nothing to click: EPUB, KePub, MOBI, and AZW3 books, CBZ comics, PDFs, and M4B audiobooks. [Reading and Playback](./reading-and-playback.md) covers the reader itself.

### Resuming Where You Left Off

A book opens where you stopped reading it. Each file of a book keeps its own place, and each user has their own, so listening to a book's audiobook doesn't move your page in its ebook. Each file on a book's page with a saved place shows how far through it you are, as a ring with the percentage. While you're reading a book, its bookmark shows your progress through the book in place of its icon: the furthest you've reached in any of its main files. Wherever books are listed, a bar sits under its cover showing the same progress.

- The ebook reader saves your place a few seconds after you turn pages, and right away when you switch to another tab or close the reader.
- The comic and PDF reader saves each page you turn to. In the **Scrolled** flow, it saves every few seconds as you scroll, and right away when you switch to another tab or close the reader. A link to a page, such as a shared `?page=` link, a page thumbnail, or a chapter, opens that page and saves it as your place, but doesn't start or finish the book until you turn a page.
- The audiobook player saves every 15 seconds while playing, and right away when you pause or seek.
- Reading the same book in two browsers, the place you reached most recently wins, so opening the book on your laptop after reading on your phone continues from the phone's place.
- **Read again**, or deleting the Read-through you're on from **Reading history**, sends every main file of the book back to the start.

### Starting and Finishing Automatically

- **Starting**: turning past the first page of a book you have never read, or listening to a minute of its audiobook in one sitting, starts a Read-through dated today, as **Start reading** would, and takes the book off To Read. Opening a book and closing it without turning a page changes nothing, and neither does seeking around an audiobook, short of its last 60 seconds, or listening to less than a minute of it. Sittings don't add up, but pausing for under 5 minutes doesn't end one. The minute that starts a read counts toward it, and the read is dated from when you started listening.
- **Rereading**: a book you have read or abandoned starts again only when you choose **Read again**, or when you've added it back to To Read to plan a reread. Otherwise you can open it to look something up, and only your place in it is saved.
- **Resuming**: turning a page of a paused book, or listening to a minute of it in one sitting, makes it **Currently Reading** again.
- **Finishing**: reaching the last page finishes the Read-through, dated today. An audiobook finishes when playback ends or you're within its last 60 seconds, so end credits don't stop it being marked read. A message says **Marked as read**, with stars to rate the book and **Undo**, which makes it **Currently Reading** again.
- **Peeking at the end**: going back before the end within 15 minutes of an automatic finish undoes it, so jumping to the end to look at the back matter doesn't mark the book read. A finish you chose with **Mark as read** is never undone this way. After 15 minutes, the book stays **Read**.

### Supplements

A supplement keeps your place like any other file, and shows how far through it you are, but reading it never changes the book's status: it doesn't start, resume, or finish a read. Starting a read doesn't send a supplement back to the start either. Supplements don't count toward [last read](#last-read).

### Last Read

The page of a book you're reading or have paused shows when you last read it, such as **Last read 4 hours ago** or **Last read 5 days ago**, and so does its card on the [Currently Reading page](#currently-reading-page). Hovering over it shows the exact date and time. Last read is the latest reading on any of the book's main files during the read you're on. Opening a place from a link, such as a page thumbnail or a shared `?page=` link, doesn't count as reading. A book you have read or abandoned shows no last read, even if you open it again to look something up.

**Mark as read** and **Mark as Did Not Finish** date the read's end by the day you last read the book, not today, so a book you put down before the epilogue and close out a week later shows when you really stopped. With no reading to go on, such as a book you marked **Start reading** and read elsewhere, they use today.

### Currently Reading Page

**Currently Reading** in a library's sidebar lists the books in that library you're reading, most recently read first. It works like **Books**, with the same search, filters, sort, and selection, except:

- Each card shows when you last read the book.
- There's no **Reading status** filter, since the page is already one status.
- Sorting here can't be saved as the library's default.

Paused books aren't listed until you resume them. To find them, filter **Books** by the **Paused** status.

### Your Time Zone

Starts and finishes are dated by the day in your time zone, under **Preferences > Reading > Time zone**, whether you choose them from the status menu or they happen as you read. Shisho fills it in from your browser while it's unset. Changing it later, when you travel for example, doesn't change any date already recorded.

## To Read

**To Read** is your queue of books to read next, in the order you plan to read them. Open it from the top of the **Lists** page, the top of the lists in the library picker, or **To Read** in the user menu. It sits with your lists and opens like one, but it isn't a list: it's yours alone, can't be shared, renamed, or deleted, and follows your reading as described below. It lists the books on it from every library you can access.

- **Add a book** from the status button on its page, or by checking **To Read** at the top of **Add to list**, on the book's page or its card wherever books are listed. A book joins at the end of the queue.
- **Reorder** by dragging books on the To Read page, as on an [ordered list](./lists.md#sorting-and-reordering). Dragging is available when the whole queue fits on one page of up to 100 books.
- **Remove a book** from any of the same places.

A queued book's page shows its place beside the status button, such as **#3 on To Read**, counting only books you can access. Choose it to open To Read. The **Lists** tab of the page's Related panel also shows To Read first, up to its first 100 books.

Starting a book, or marking it read, takes it off To Read. To plan a reread, add a book you have finished: it shows **To Read** again and opens at the start, and its earlier reads stay in the history.

To queue a whole [list](./lists.md), use [Copy to To Read](./lists.md#copying-to-to-read) on its page.

Reading apps can download from your queue through the [OPDS catalog](./opds.md#catalog-structure).

## Finding Books by Status and Rating

Your statuses and ratings are ways to browse, in a library's gallery and on any [list](./lists.md). Each user filters and sorts by their own.

- **Filter** has a **Reading status** picker, where you can choose one or more statuses, and **No status** for books you have never tracked. A book matching any chosen status shows. Below it, the **Rating** slider sets a range in quarter stars: drag the left end up to set the lowest rating, and the right end down to set the highest. An end left at its edge, marked **Any**, sets no limit on that side. Unrated books match only while both ends are at **Any**.
- **Sort** offers **Reading status**, **Rating**, **Date finished**, and **Last read**.
  - **Reading status** orders books Currently Reading, Paused, To Read, Read, Did Not Finish, then books with no status, so the books you're reading come first. Reverse it to put them last. Add another level, such as **Date added**, to order books with the same status.
  - **Rating**, **Date finished**, and **Last read** start highest and latest first. Date finished is the end date of your latest finished read; a book you abandoned, or finished with no end date, has none. [Last read](#last-read) only applies to books you're reading or have paused. Books with no rating or date come last in either direction.
  - A list's sort menu offers **Currently Reading First**, **Currently Reading Last**, **Rating (high to low)**, **Rating (low to high)**, **Recently Finished**, and **Earliest Finished**.

Filters apply before the gallery or list pages, so the count and every page hold only matching books. An ordered list can't be dragged into a new order while it's filtered.

A list's saved default sort can be by any of these. Each person viewing the list then sees it in the order of their own reading. A library's saved default sort can be too, and it orders that library in the [OPDS](./opds.md) catalog and [eReader Browser](./ereader-browser.md) by your reading.

Reading apps can download the books you're reading, or have paused, through the [OPDS catalog](./opds.md#catalog-structure).

## Reading History

Each time you read a book is a **Read-through**: when it started, when it ended, and whether you finished or abandoned it. Once a book has a Read-through, its page shows **Reading history** below its files, with a summary such as "Read twice, last finished Mar 2024" and every Read-through, newest first.

- **Edit dates** changes a Read-through's start and end dates. Choose **Clear** in a date's calendar if you don't know it, and **Today** for today. Dates after today can't be picked. A Read-through you're still on has no end date.
- **Delete** removes a Read-through you logged by mistake. It no longer counts toward the history or the status, and this can't be undone. Deleting the one you're on is the same as **Mark as not started**.

The status and summary update after each change.

## Ratings

Rate a book with the stars next to its status, in quarter stars from a quarter star to five stars. The rating's number shows beside the stars. Hovering over them previews, in paler stars and with its number, the rating a click would set, and the stars turn solid once it's saved. Click or drag across the stars, or focus them and use the arrow keys. Choose the **×** beside the stars to clear a rating.

Your rating also shows in the bottom-left corner of the book's card wherever books are listed.

A rating doesn't depend on status. You can rate a book you abandoned or one you never marked read, and rereading keeps your rating.

## Merging and Deleting Books

When [merging books](./managing-books-and-files.md#merging-books) removes a book, every user's reading history moves to the book it merged into:

- The merged book keeps its own rating and its place on To Read. When it has none, it takes those of a removed book, the one added to Shisho first if several have one.
- If both books were being read, the two reads become one, starting on the earlier start date.

Moving only some of a book's files moves no reading data. Deleting a book deletes everyone's reading data for it. A user who loses access to a library keeps their reading data, and sees it again if their access returns.
