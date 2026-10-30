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

- **Start reading** begins a new read dated today. It's called **Read again** once you have read the book before, and the earlier reads stay in the history.
- **Pause** sets aside the book you're reading. **Resume** picks it back up with its original start date.
- **Mark as read** finishes the read you're on, dated today. With nothing underway, it records a finished read that ended today with no start date, for a book you read before you used Shisho.
- **Mark as Did Not Finish** ends the read you're on as abandoned, dated today.
- **Add to To Read** or **Remove from To Read** puts the book at the end of your [To Read](#to-read) queue or takes it off. It isn't offered while you're reading the book or have paused it.

"Today" is the date on your device when you choose the action.

## Reading in Shisho

The EPUB, KePub, MOBI, and AZW3 reader keeps your place and your reading history current as you read, with nothing to click. [Reading and Playback](./reading-and-playback.md) covers the reader itself.

### Resuming Where You Left Off

A book opens where you stopped reading it. Each file of a book keeps its own place, and each user has their own.

- The reader saves your place a few seconds after you turn pages, and right away when you switch to another tab or close the reader.
- Reading the same book in two browsers, the place you reached most recently wins, so opening the book on your laptop after reading on your phone continues from the phone's place.
- **Read again**, or deleting the Read-through you're on from **Reading history**, sends every file of the book back to the start.

### Starting and Finishing Automatically

- **Starting**: turning past the first page of a book you aren't reading starts a Read-through dated today, as **Start reading** would, and takes the book off To Read. Opening a book and closing it without turning a page changes nothing.
- **Resuming**: turning a page of a paused book makes it **Currently Reading** again.
- **Finishing**: reaching the last page finishes the Read-through, dated today. A message says **Marked as read**, with stars to rate the book and **Undo**, which makes it **Currently Reading** again.
- **Peeking at the end**: going back before the end within 15 minutes of an automatic finish undoes it, so jumping to the end to look at the back matter doesn't mark the book read. A finish you chose with **Mark as read** is never undone this way. After 15 minutes, going back counts as starting a reread.

### Your Time Zone

Automatic starts and finishes are dated by the day in your time zone, under **Preferences > Reading > Time zone**. Shisho fills it in from your browser while it's unset. Changing it later, when you travel for example, doesn't change any date already recorded.

## To Read

**To Read** is your queue of books to read next, in the order you plan to read them. Open it from the top of the **Lists** page, the top of the lists in the library picker, or **To Read** in the user menu. It sits with your lists and opens like one, but it isn't a list: it's yours alone, can't be shared, renamed, or deleted, and follows your reading as described below. It lists the books on it from every library you can access.

- **Add a book** from the status button on its page, or by checking **To Read** at the top of **Add to list**, on the book's page or its card wherever books are listed. A book joins at the end of the queue.
- **Reorder** by dragging books on the To Read page, as on an [ordered list](./lists.md#sorting-and-reordering). Dragging is available when the whole queue fits on one page of up to 100 books.
- **Remove a book** from any of the same places.

A queued book's page shows its place beside the status button, such as **#3 on To Read**, counting only books you can access. Choose it to open To Read. The **Lists** tab of the page's Related panel also shows To Read first, up to its first 100 books.

Starting a book, or marking it read, takes it off To Read. To plan a reread, add a book you have finished: it shows **To Read** again, and its earlier reads stay in the history.

To queue a whole [list](./lists.md), use [Copy to To Read](./lists.md#copying-to-to-read) on its page.

Reading apps can download from your queue through the [OPDS catalog](./opds.md#catalog-structure).

## Finding Books by Status and Rating

Your statuses and ratings are ways to browse, in a library's gallery and on any [list](./lists.md). Each user filters and sorts by their own.

- **Filter** has a **Reading status** picker, where you can choose one or more statuses, and **No status** for books you have never tracked. A book matching any chosen status shows. Below it, the **Rating** slider sets a range in quarter stars: drag the left end up to set the lowest rating, and the right end down to set the highest. An end left at its edge, marked **Any**, sets no limit on that side. Unrated books match only while both ends are at **Any**.
- **Sort** offers **Reading status**, **Rating**, and **Date finished**.
  - **Reading status** orders books Currently Reading, Paused, To Read, Read, Did Not Finish, then books with no status, so the books you're reading come first. Reverse it to put them last. Add another level, such as **Date added**, to order books with the same status.
  - **Rating** and **Date finished** start highest and latest first. Date finished is the end date of your latest finished read; a book you abandoned, or finished with no end date, has none. Books with no rating or finish date come last in either direction.
  - A list's sort menu offers **Currently Reading First**, **Currently Reading Last**, **Rating (high to low)**, **Rating (low to high)**, **Recently Finished**, and **Earliest Finished**.

Filters apply before the gallery or list pages, so the count and every page hold only matching books. An ordered list can't be dragged into a new order while it's filtered.

A list's saved default sort can be by any of these. Each person viewing the list then sees it in the order of their own reading. A library's saved default sort can be too, and it orders that library in the [OPDS](./opds.md) catalog and [eReader Browser](./ereader-browser.md) by your reading.

Reading apps can download the books you're reading, or have paused, through the [OPDS catalog](./opds.md#catalog-structure).

## Reading History

Each time you read a book is a **Read-through**: when it started, when it ended, and whether you finished or abandoned it. Once a book has a Read-through, its page shows **Reading history** below its files, with a summary such as "Read twice, last finished Mar 2024" and every Read-through, newest first.

- **Edit dates** changes a Read-through's start and end dates. Choose **Clear** in a date's calendar if you don't know it, and **Today** for today. Dates after today can't be picked. A Read-through you're still on has no end date.
- **Delete** removes a Read-through you logged by mistake. It no longer counts toward the history or the status, and this can't be undone.

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
