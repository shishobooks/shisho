# Kobo sync tracks entries that keep their ids, and never changes a sent RevisionId

Kobo sync identifies each synced item by its File, with a `RevisionId` derived from the file's path and size. Tests on a Kobo showed that it treats every new `RevisionId` as a new book, so moving, renaming, or rewriting a synced file leaves a duplicate on the device, and so would every change that swaps which File a book delivers under ADR 0011. The tests also showed that a Kobo ignores a change sent under a `RevisionId` it does not have, and re-downloads a book in place when a change arrives as a `ChangedEntitlement` under the same ids. So Shisho records, per Device, each entry it has put on the Kobo: the book, category, and delivered File Type it belongs to, the File it currently holds, and the id and `RevisionId` it was sent with, which never change. An entry stays with its File while that File still qualifies under the Device's Delivery Rules. When a File stops qualifying, its entry is handed to a newly qualifying File of the same book, category, and delivered type, as an in-place change, or removed when there is none. Files tied on a rule each hold their own entry, so same-type editions still all sync, as ADR 0003 intended.

## Considered options

- One id per book, category, and delivered File Type, computed from current data. Rejected because Files tied on a rule would collide on one id.
- The first File to reach the device keeps the book's id and other tied Files get their own, promoting the next one when the first leaves. Rejected because promotion removes the promoted edition's own entry, losing the reading position of a book the reader may be partway through.
- Naming new entries after the File they first hold, as today's ids are. Rejected because an entry handed to another File keeps its id, so the first File could not get a new entry under its own name later.

## Consequences

- Entries already on a device keep their `shisho-{fileID}` ids and per-File `RevisionId`s, so upgrading never re-adds a book or stops updating it; an entry whose File the Device's rules no longer select is removed (ADR 0011). The `RevisionId` last sent for each File can be rebuilt from the file hash the sync state already stores. New entries take ids from their own record, in a form that cannot match a `shisho-{fileID}` id.
- Handing an entry to another File keeps the reading position when the two editions' chapter files line up and loses it when they do not, so the rules editor and book pages warn when a change would move a Kobo to a different edition.
- Changing a Kobo's delivered type goes out as one entry removed and another added. In those tests, changing type in place (KePub to EPUB and back) left the book unable to save its position or show highlights. The rules editor warns before saving such a change.
- Plain EPUB is reported to the Kobo as `EPUB3`, or `EPUB3FL` when fixed-layout; the Kobo never offers a book reported as `EPUB`.
- Highlights can stay hidden for a while after an in-place re-download and reappear later; they are not deleted.
