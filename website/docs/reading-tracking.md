# Tracking Your Reading

Each user can record their reading of a book, rate it, and look back at its history. This record is private: other users, admins included, never see your statuses, history, or ratings.

## Reading Status

A book's **Reading Status** shows on its page, next to its actions, and as a bookmark hanging from the top of its card wherever books are listed. Each status has its own bookmark color and icon, and hovering over the bookmark names it. It is one of:

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

## To Read

**To Read** is your queue of books to read next, in the order you mean to read them. Open it from **To Read** in a library's sidebar or in the user menu. It lists the books on it from every library you can access.

- **Add a book** from the status button on its page, or from the **⋯** menu on its card wherever books are listed. A book joins at the end of the queue.
- **Reorder** by dragging books on the To Read page, as on an [ordered list](./lists.md#sorting-and-reordering). Dragging is available when the whole queue fits on one page of up to 100 books.
- **Remove a book** from the same status button or card menu.

Starting a book, or marking it read, takes it off To Read. To plan a reread, add a book you have finished: it shows **To Read** again, and its earlier reads stay in the history.

To queue a whole [list](./lists.md), use [Copy to To Read](./lists.md#copying-to-to-read) on its page.

Reading apps can download from your queue through the [OPDS catalog](./opds.md#catalog-structure).

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
