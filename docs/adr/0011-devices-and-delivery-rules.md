# Devices own their credentials and Delivery Rules decide what each one receives

Which File Type a reader gets is decided in four unrelated places: a per-library download setting that only the web UI reads, Kobo sync always serving KePub, a parallel `/opds/v1/kepub` feed tree, and User-Agent detection in the eReader browser. None of them can say "keep CBZs as CBZ in KOReader but send them to Kobo's own reader as KePub", and adding a File Type or a Conversion means touching all four. We replace them with two concepts. A **Device** is one reading app on one piece of hardware; it belongs to a user and owns the single credential all its connections use (Kobo sync, eReader browser, OPDS), replacing API keys, whose only uses were Kobo sync and the eReader browser. Each Device has a Device type, such as Kobo or Kindle, that limits the File Types it can receive and supplies preset rules. **Delivery Rules** are an ordered list with at most one rule per source File Type, each naming the File Type to deliver it as; an unlisted type is not delivered, and the earliest matching rule picks which of a book's Files in a category goes, along with any File tied with it. A Device follows its user's default, follows its type's preset, or has its own list; the user's default follows Shisho's built-in rules until edited. A list that follows another is never copied, so a new File Type reaches every list nobody customized without a data migration.

## Considered options

- A ranked list of delivered File Types per Device, like an HTTP `Accept` header. Rejected because it cannot accept a type as-is while refusing to convert into it: a reader ranking EPUB above CBZ would have every CBZ-only book converted to EPUB.
- A map from each source File Type to its delivered File Type. Rejected because it has no order, so it cannot choose between a book's Files; every Kobo would keep receiving one entry per File.
- Preferences per user only, or per user with per-library overrides. Rejected because the same user's Kobo reader and KOReader on that same Kobo want different results from the same library.
- Per-rule overrides on top of the user default instead of follow-or-customize. Rejected because an overridden rule has no defined place when the default is reordered, and the settings UI would mix inherited and overridden rules. Library plugin order already uses follow-or-customize.
- An Auto-detect Device type that keeps reading the User-Agent. Rejected because a typed Device always knows what it is. The User-Agent is still read, but only to warn on the device itself when its Device type looks wrong; it never changes what is served.

## Consequences

- Nothing that works without a Device today starts requiring one: OPDS signed in with the account password uses the user's default Delivery Rules, and the web UI uses them to pick its default download.
- The web UI and Share Link download menus offer every reachable File Type regardless of rules, since they are also how files reach readers with no Device type. Share Links default to the Original Files' types.
- This narrows ADR 0003, which sends Kobo sync, the eReader browser, and OPDS every compatible File. A Device now gets only the Files on its earliest matching rule, so after upgrading, a book that synced to a Kobo as both an EPUB and a CBZ keeps only the entry its rules select.
- A Device's scope (all books, one library, or one list) moves from the Kobo sync URL onto the Device and applies to every connection. A migrated Kobo URL's scope segment sets the Device's scope on its first request after upgrading and is ignored after that.
- How Kobo sync identifies books under these rules is ADR 0012.
- Removing the library setting, the KePub feed tree, the OPDS file-type URL segment, and User-Agent-driven delivery are breaking changes, with no redirects. Migrated API keys keep their id and key as the Device's, so Kobo sync state survives and saved Kobo and eReader URLs keep working.
