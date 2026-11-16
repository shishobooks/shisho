# The KOReader plugin and the Shisho app agree with each server on a protocol range and a list of capabilities

Two parts of Shisho ship apart from the server: the **KOReader plugin** and the **Shisho app**. A copy of either can't be forced to update, and a self-hosted server upgrades whenever its owner gets to it, so any pairing of old and new must work or fail safely. Each has its own protocol integer. The server accepts a range of protocols and answers anything outside it with 426 Upgrade Required, as it already does for the plugin. Within a protocol, the server also lists named capabilities, and the plugin or app turns a feature on for a server only when that server lists it.

The rules both follow:

- **Capabilities come with every response from their surface**, not only with the server info, so a server rolled back to an older release is noticed mid-session and the missing features turn off. A response served from a cache, such as an immutable cover, is never read for capabilities.
- **The capability list depends on the protocol the request names.** A protocol bump folds every existing capability into the new baseline, so a request on the new protocol is sent only what was added since, while a request on an older protocol still in range is sent the full list it knows.
- **The server keeps the old meaning of existing input for every protocol still in its range.** If it can't, it bumps the protocol instead.
- **Request fields stay strict.** The server's binder rejects fields it doesn't know, and a new field is sent only to servers that list the capability for it. The app's journal is the exception (ADR 0024): it judges each event on its own, so one event the server doesn't understand never refuses the whole batch.
- **Responses only grow within a protocol.** Removing a field or changing its type is a protocol bump. The plugin and the app decode leniently: they ignore fields they don't know and map unknown enum values to an unknown case.
- **Being turned away for a version never costs data.** Whatever the server turns away for being too old or too new is kept and sent again once the server accepts it.

## Considered options

- **The app calls `/api` like the web app, with capability checks everywhere.** Rejected because every route the web app uses would become an external contract, and web changes would wait on the app.
- **Online-only screens open the server's own web pages in a WebView.** Rejected because those are the management screens wanted in the app, and a WebView gives up the native polish the app exists for.
- **Version numbers instead of capabilities.** Rejected because a feature then depends on comparing releases, and a server that adds a feature in a patch release, or a rollback, gets it wrong.

## Consequences

- **The Shisho app has two surfaces.** Its Device surface covers everything it needs to work at all, and everything it does offline: sign-in, server info, the catalog change feed (ADR 0023), the journal (ADR 0024), downloads and streaming, and covers. That surface is versioned like the plugin's. Its online-only screens (admin, metadata editing, Identify) call the same `/api` as the web app, each behind a versioned capability. When a route behind a listed capability changes incompatibly, the server lists the next version instead, and an older app hides that screen. A test pins the request and response types of every route behind a listed capability. Every other `/api` route stays internal to the web app, as before.
- **The minimum server for the app's first release** is the first Shisho release with the Device surface, which is app protocol 1.
- **An app protocol stays accepted for at least 12 months** after a newer one supersedes it, and the release notes announce when one is dropped. The plugin has few users and fixes forward, so this floor is the app's.
- **A server too old for the app, or an app too old for its server, leaves the account working offline.** Downloads still play and changes keep queuing until the gap closes.
- **The app doesn't connect to a server in Demo Mode**, which refuses every write, including the sign-in that creates a Device.
