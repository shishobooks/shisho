# The KOReader plugin syncs KOReader's own records, and late data is placed by its own time

KOReader's progress sync protocol (ADR 0014) sends no time with a push, so an offline push is dated when it arrives, and a reading session can only be estimated from a pull and a push. It also has no way to report a finish, a Rating, or anything about a Book that isn't open. E-readers are often offline for weeks to save battery, so most reading happens exactly where the protocol is weakest. The **KOReader plugin** replaces the protocol for the servers it signs into, and it syncs records KOReader already keeps on the device, not records of its own:

- **Reading time comes from KOReader's statistics database.** It times every page visit, leaves out time asleep, survives restarts, is never pruned, and already matches what KOReader's own statistics screens show. The plugin only keeps a small journal of what statistics doesn't record: positions, and status and Rating changes with when they happened. If statistics is turned off, the plugin says that reading time won't sync and still syncs everything else.
- **The server receives raw page rows, keeps them, and groups them into Reading Sessions itself.** A session's time is the sum of its page durations, so it can be less than its span. Keeping the rows lets a later change to the grouping, or a new figure on Reading Stats, be computed again without a re-upload from a device that may be offline for months.
- **The server keeps a record of everything it has taken in from each Device**, keyed by KOReader's own key for a page row (document hash, page, start time) and an id for each journal event. Repeats are ignored even after the user has deleted what they produced, so a deletion in Reading History stays deleted. Deleting statistics in KOReader changes nothing in Shisho.
- **Late data is placed by its own time.** A session from three weeks ago attaches to the Read-through underway three weeks ago, or to none, and a start or finish is dated by when it happened. The device clock is trusted after one correction: each sync sends the device's current time, and the server shifts the batch by the measured offset when that offset is large. Times before the Device existed are refused and logged, except in history sent from before the plugin was installed, where only implausibly early times are.
- **Status and Ratings go both ways.** KOReader's finished, On hold, and reading map to Read, Paused, and Currently Reading. Did Not Finish goes to KOReader as On hold. The plugin remembers what it last synced for each Book, so a value written by Shisho is never sent back as a change. When both sides changed, the later change wins, compared by day, since KOReader only records a day; on the same day Shisho wins. Ratings go to KOReader rounded to whole stars, and a rounded value is never sent back.
- **One installation signs into several servers**, as a separate Device on each. Each server receives data for every document hash it recognizes, which needs no record of where a copy came from.

## Considered options

- **Keep the progress sync protocol and add only what it lacks.** Rejected because users would still configure two connections with the same credential, and both would report the same reading.
- **Time reading in a journal of the plugin's own.** Rejected because it duplicates the statistics plugin's timing, disagrees with KOReader's own figures, and covers nothing from before the plugin was installed.
- **Group page rows into sessions on the device.** Rejected because changing the grouping later would need every device to send its history again.
- **Attach late data to the Read-through underway when it arrives.** Rejected because months of offline reading would land on whatever Book happens to be underway at the next sync.

## Consequences

- **The update service places a session by its time,** where it used to record an old update's session against the Read-through underway now. Sessions with no Read-through underway at their time still count toward time on Reading Stats.
- **History from before the plugin can be sent,** on first sign-in to each server if the user chooses. It attaches by time like any late data. It creates Read-throughs only from KOReader's finished and On hold statuses, at most one per Book, since KOReader keeps no record of rereads. Estimated sessions the protocol recorded for the same Device are removed where measured ones cover the same time.
- **A KOReader status change is always deliberate,** because opening a Book never changes it. Setting a finished Book back to reading on the device starts a reread.
- **Writing status to a Book a device has never opened** needs the plugin to know each local file's hash, so it keeps an index of the files in KOReader's home folder.
- **The protocol endpoints stay** for KOReader without the plugin, and a Device that has synced through the plugin has its protocol pushes ignored.
- **The device's time zone is never used.** Days come from the user's saved time zone, as for every other source.
