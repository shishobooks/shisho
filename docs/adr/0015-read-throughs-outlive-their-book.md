# Read-throughs outlive their Book

ADR 0013 made Read-throughs the reading record. That record still disappeared whenever a Book was deleted, so a user's Reading Stats could change because of something an admin did, such as clearing out a library. Now, deleting a Book detaches its Read-throughs instead of deleting them. Each one keeps a copy of the Book's title and author names, and its Reading Sessions stay with it. A detached Read-through still counts toward books finished, abandoned, and time spent. It only goes away when the user deletes it.

## Considered options

- **Cascade, as before.** Rejected because history that was never kept can't be recovered, which was the reason for ADR 0013.
- **Also copy Genres and Tags,** so deleted Books stay in every breakdown. Rejected because it starts a second metadata model inside the reading record. Detached Read-throughs count toward totals and time but drop out of the genre, tag, and author breakdowns.

## Consequences

- **Anything that reads Read-throughs through their Book has to allow for there being no Book.** Reading Status, device sync, and Book pages ignore detached Read-throughs, since there is no Book for them to describe.
- **A detached Read-through still underway stays underway.** It is never ended automatically, because ending it would record a Did Not Finish the user never chose. Its Sessions still count toward time.
- **Users manage detached Read-throughs on Reading History**, the page that lists every Read-through. They can edit or delete them there like any other. Starting a reread is unavailable, because there is no Book to reread.
- **Adding the same work again doesn't reattach its history.** Scans keep the same Book when a file moves, so detaching only happens when a Book is actually deleted.
