# The Shisho app records changes as a journal of events, and the server places each by its own time

The **Shisho app** works offline first. Positions, Reading Sessions, Read-through actions, Ratings, To Read changes, List changes, and Bookmarks are saved on the device and work without a connection. Each change is saved as an event in a local journal, with an id that is never reused, the instant it happened, and a type. The app sends the journal in batches with the device's current clock. The server ignores events it has already taken in from that installation, corrects the batch for clock drift, and places each event by its own time. This is the model the KOReader plugin uses (ADR 0016), generalized from KOReader's records to every kind of change the app can make.

A user signs into the app with their normal username and password, and each sign-in on an installation creates a **Device** of that user. The Device appears in the user's Device list, where revoking it signs that installation out, and is how Reading History says where reading happened. Its credential never expires, because a phone may be offline for weeks. The installation also has its own id, made when the app is installed and kept across signing out and back in, and the Device records it.

## Considered options

- **Call the regular API directly and queue failed calls offline.** Rejected because replaying calls in order loses when each change actually happened, and a request retried after a lost response is applied twice.
- **A CRDT for conflict resolution.** Not needed, because the server is the single authority and each record has a natural rule:
  - the latest instant wins for positions, Ratings, and a Bookmark's note;
  - sessions and Bookmarks are added by unique id, and a delete wins;
  - Read-throughs follow ADR 0013;
  - List and To Read order is the one place two offline edits can conflict, and there the latest move of each Book wins.
- **A separate app password or pairing code**, as e-readers use. Rejected because a first-party app that the user can sign into on the web with their password should accept the same password.
- **Deduplicating by Device**, as the KOReader plugin does. Rejected because signing in again after a revoke creates a new Device, and the journal it resends would be applied twice. Read-through actions aren't safe to apply twice.

## Consequences

- **Events are deduplicated by user, installation, and event id**, so a journal resent after signing in again is still applied once. For the same reason, the earliest time the server accepts is when the user's account was made, not when the current Device was.
- **A new kind of change the app can make is a new event type**, not a new protocol. Highlights and notes will arrive this way.
- **A feature that writes a new event type only appears for servers that list it.** Highlights, for example, appear only for servers whose capabilities (ADR 0020) include them. A new field on an existing event type is a capability too, and the app sends that field only to servers that list it.
- **"Not understood" is never "refused".** The capability list can be out of date, for example after a server rolls back while the phone was offline. The journal endpoint judges each event on its own instead of rejecting the batch for one unknown field, and answers an event type or field it doesn't know with a result that means not understood. The app keeps those events queued for that account, and sends them once the server lists them again. Event types don't depend on each other, so queued events of one type don't hold back the others. If the server rejects a whole batch anyway, the app keeps every event in it.
- **The server can permanently reject an event it understood**, for example when the Book was deleted or the event fails validation. Only then does the app drop it, and it tells the user instead of retrying forever.
- **Reading Sessions are timed on the device** with the same rules as the web reader, and the app sends whole sessions. ADR 0016 rejected this for KOReader, because changing how sessions are grouped would need every device to send its history again. Here the grouping rules are Shisho's own, already used by the web reader, which also sends whole sessions, so the app follows the web reader. Regrouping past sessions isn't planned for either.
- **Changing a password doesn't sign the app out.** Revoking the Device or deactivating the user does. A signed-out installation keeps its unsent journal and sends it after the user signs in again.
