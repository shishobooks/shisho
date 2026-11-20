# Change Feed for the Shisho App

The Shisho app keeps a copy of the catalog from a change log that SQLite triggers keep (ADR 0023): one entry per record the app sees, moved to the next sequence number when anything in that record changes. `pkg/changefeed` pages through it; `pkg/shishoapp` turns entries into the app's documents.

## A write that changes nothing must write nothing

Update triggers fire only when a compared column differs, so a Scan or Resync that rewrites a row with the values it holds records nothing. Two kinds of write still move a record without changing it, and every app installation then downloads it again:

- **Bumping `files.updated_at`.** It keys the cover version, so the files trigger compares it, and `books.Service.UpdateFile` sets it on every call. Call `UpdateFile` only when a listed column really changed; a provenance stamp with the same source, or a size and mtime that match, is not a change.
- **Deleting and inserting rows again.** Replace a relationship with the in-place `books.Service.Replace*` methods and chapters with `chapters.Service.ReplaceChapters`, which keep rows whose values the write reads again. `TestResync_UnchangedBookRecordsNothing` in `pkg/worker` fails when a Resync of an unchanged Book moves it.

A transaction that reads before it writes can't take SQLite's write lock once another connection has written, which the tests' shared database does. Take the lock first, as `lockChapters` in `pkg/chapters/service.go` does, or read outside the transaction where a lock already keeps writers apart.

## A new table or column

`pkg/migrations/change_log_invariants_test.go` fails until every table is in the feed or excused, and every column of a table in the feed is compared or excused, each with a reason. Triggers are created in migrations and never edited: a column joins the feed through a new migration that drops and recreates its table's update trigger.

## The Device surface is a contract

Routes under `/mobile` serve app releases that can't be forced to update (ADR 0020). Within a protocol, responses only grow and request fields stay strict; removing or retyping a field in `pkg/shishoapp/types.go`, or a feed document, needs a new protocol. A feature the app turns on per server gets a capability in `capabilities` in `pkg/shishoapp/contract.go`.
