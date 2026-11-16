# Catalog changes are recorded by database triggers, not by the code that makes them

The **Shisho app** keeps a full copy of the catalog of every Library its user can access, so that every screen works offline. It keeps that copy current by asking the server for everything that changed since its last sync, deletions included. SQLite triggers on the catalog tables and on the tables holding each user's own records note each insert, update, and delete in a change log with an increasing sequence number. Deletions are noted too, so they still reach the app after the row is gone. The app asks for changes after the last sequence number it saw.

The log is kept by record as the app sees it, not by table. A Book is one record, with its authors, series memberships, genres, tags, and Files (with their narrators, chapters, and publisher). A change to any of those rows moves the Book's entry. The server's tables can then be reshaped without changing what the app receives.

Recording changes in the database means no write path can forget to. Search indexing is the counterexample: the code that writes has to remember to reindex, and a new delete endpoint that doesn't is a known trap.

## Considered options

- **Log changes in the service code that makes them.** Rejected because every new write path, bulk operation, migration, and Scan would have to remember to log, and a missed entry leaves a stale or deleted Book in the app with no way to notice.
- **Compare `updated_at` columns and keep a separate list of deletions.** Rejected because not every write updates `updated_at`, a deletion would still need logging by hand, and clock-based cursors can skip rows written in the same instant.
- **A snapshot per Device, compared on each sync**, as Kobo sync does. Rejected for the whole catalog because the snapshot grows with the library times the number of Devices.
- **One entry per table row.** Rejected because the app would then mirror the server's schema, and every schema change would become a protocol change.

## Consequences

- **A new table needs its triggers** to appear in the app. Triggers that fire only when a value differs name each column they compare, so a new column needs adding too. An invariant test fails when a table or a column is left out.
- **The user's own records go through the same feed**: Reading Positions, Read-throughs, the per-user, per-Book record holding Rating and To Read, Lists and the List Shares visible to the user, and Bookmarks. One cursor brings an installation fully up to date.
- **Each entry records the Library or user it belongs to when it's written.** A deleted row can't be looked up afterwards, and during a cascade a child's trigger can't find its parent, so the feed filters by what the entry recorded, not by joining back. A List's entries are the exception: they're filtered against the List's current List Shares, so a shared List reaches every user it's shared with.
- **A change in access is answered outside the cursor.** When library access, List Shares, or a role change what a user can see, the next sync tells the app to drop a Library or to fetch one whole, since a Library just gained has no entries after the app's cursor.
- **The change log keeps one entry per record, not a history.** A trigger moves a record's existing entry to the next sequence number instead of adding another, so the log stays about the size of the catalog no matter how often a Scan rewrites it. The app only needs to know what is current, not every step in between.
- **A write that changes nothing records nothing.** A Scan or Resync often rewrites rows with the values they already hold. If that counted, every installation would download its whole library again after every Scan, so triggers fire only when a value differs. Today chapters are deleted and inserted again on every Resync, and relationships are replaced on a forced refresh, so those writes must update rows in place for this to hold.
- **Sequence numbers follow commit order because SQLite runs one write at a time.** A database that commits writes in parallel would break this, because an installation could move past a write that committed late.
- **The feed is part of the app's versioned Device surface** (ADR 0020). A newer server can send kinds of record, or fields, that an older app doesn't know. The app skips them, and its cursor moves past them anyway.
- **An app update that understands more refreshes the whole catalog.** The app stores which kinds and fields it understands. When an update understands more, the app downloads the catalog again from the start in the background, keeps showing its current copy until the new one is ready, and applies its own unsent changes on top. Because the log holds one entry per record, a refresh from the start is just the current catalog.
- **Deletion entries expire after 90 days.** The server remembers the highest sequence number it has expired. An installation whose cursor is below it may have missed a deletion, so it's told to download the catalog again from the start, which takes seconds even for a large library.
