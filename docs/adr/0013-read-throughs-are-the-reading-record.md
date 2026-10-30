# Read-throughs are the reading record, and Reading Status is derived from them

Shisho records each user's reading of a Book in two ways:

- as **Read-throughs**, each with an optional begin day, an end day once it ends, and whether it was finished or abandoned, or is still underway and possibly paused
- as whether the Book is on the user's **To Read** queue

There is no stored status. **Reading Status** is computed in priority order:

1. A Read-through underway gives Currently Reading or Paused.
2. Otherwise, being on To Read gives To Read.
3. Otherwise, the latest ended Read-through gives Read or Did Not Finish. "Latest" means the latest end day; Read-throughs with no end day rank below dated ones, and ties go to the one recorded last.
4. Otherwise there is none.

Rereads, finish dates, and reading stats all need history, and history that was never recorded can't be recovered later. So the history is the record, and the status is a view of it.

## Considered options

- A single status per user and Book, optionally with dates. Rejected because a reread overwrites the earlier read, so "read twice", "books finished in 2026", and any calendar of finishes become impossible.
- A status column alongside a Read-through history. Rejected because the two drift: every control and every device update would have to keep both in step, and a mismatch has no correct answer.

## Consequences

- **Controls act on the records, not on a status.**
  - "Mark as read" ends the Read-through underway as finished, or creates a finished one when none is underway.
  - "Add to To Read" adds the Book to the end of the queue. A Book with a Read-through underway can't be added, since its status would stay Currently Reading or Paused.
  - Starting a Read-through takes the Book off To Read and resets the Book's Reading Positions to the start.
  - Ending a Read-through, by hand or from a device, also takes the Book off To Read, so "Mark as read" on a queued Book shows Read. Adding it again afterwards plans a reread.
- **Deleting a Read-through means "this one doesn't count".** The Reading Sessions and the history of Reading Position updates recorded while it was underway belong to it, so they go with it. Deleting the one underway also resets the Book's Reading Positions.
- **Rating and To Read membership live on a separate per-user, per-Book record**, not on a Read-through. That way a Rating survives rereads, and a one-tap favorite, if it's ever wanted, is one more field on that record.
- **Finishing from position updates.** Reaching the end of a File finishes the Read-through underway. A finish inferred this way is undone if the next update for that File arrives within a short window and is back before the end. An explicit finish is never undone this way, whether it's a user's "Mark as read" or a device reporting the book finished.
- **End-day defaults.** A Read-through ended by hand defaults its end day to the day the user last read the Book, or to today when there's no reading activity.
- **Days, not instants.** Dates on a Read-through are days. Each is the day of the event's instant in the user's time zone at that moment, including events reported later by a device, and it's never recalculated.
