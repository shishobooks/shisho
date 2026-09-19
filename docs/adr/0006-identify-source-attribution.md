# Identify source attribution

Status: accepted

## Context and decision

Identify lets an editor review and change a Plugin Proposal before applying it. Until now nearly every applied value was stamped with the plugin's source, even when the editor typed something else, so the next ordinary Scan let the auto-enricher overwrite the edit (plugin versus plugin is a priority tie and the newcomer wins). The same edit made through the Edit form receives `manual` and survives. Identify now assigns Metadata Provenance from the final persisted meaning of each selected field:

1. A final value that is semantically unchanged from the stored value is a no-op and keeps the stored value and source, including a stored `manual`.
2. A changed value that matches the Plugin Proposal is a Proposal Acceptance and receives `plugin:<scope>/<id>`.
3. Any other changed value is a manual edit and receives `manual`.

**The browser computes intent and the server computes no-op.** Neither side can decide all three states alone. The Plugin Proposal is never stored, so only the browser can compare against it. Stored values, canonicalization, and alias resolution live on the server, so only the server can detect a no-op. The form sends a per-field intent: `plugin` when the final value equals the proposal, `user` otherwise. Proposal equality is a trimmed raw comparison of the final value against the plugin's result, so editing and then restoring the proposal is a Proposal Acceptance with no special handling. The server first compares the canonicalized final value against the database (trimming, HTML stripping for Description, parsed dates, normalized language, nullable booleans that distinguish absence from `false`). Only when the value changed does it map the intent to a canonical source. Alias equivalence applies to the stored-versus-final comparison on the server and is never attempted against the proposal.

**Wire shape.** `PluginApplyPayload` carries a `sources` map keyed by the same keys the payload uses for values: the keys of `fields`, plus `file_name` for the file Name. Values come from a finite Go-owned `SourceIntent` type (`plugin` | `user`) generated to TypeScript (ADR 0004). An unknown intent is a validation error. A selected field with no entry is treated as `user`, because misattributing a plugin value as manual only protects it from Scans, while the reverse loses edits. Nothing client-provided is written verbatim into a source column. The earlier `file_name_source` field is folded into the map, and its "older clients" default branch is removed because the SPA ships with the server. Identifier objects inside `fields.identifiers` reserve an optional per-entry `source` intent with the same finite values. It is validated now and consumed when Identifier attribution lands.

**Sort title.** A Title change regenerates the sort title only when the stored `sort_title_source` is not `manual`, and stamps the regenerated value `filepath`, exactly like the Edit form. Identify never writes `manual` or a plugin source to `sort_title_source`. Sharing the Title's source would have stamped `manual` on a derived value, and the Edit form only regenerates the sort title when its source is not `manual`, so a later Title edit would silently stop updating it.

**Atomicity.** A value and its source are always written in the same `UpdateBook` or `UpdateFile` column set, so a partial failure cannot leave a value attributed to the wrong source. Relationship insert failures return an error instead of a warning, so a half-written collection is never attributed as if it were complete. Wrapping the whole apply path in a database transaction is out of scope.

**Explicit Clears leave no tombstone.** Clearing a value nulls both the value and its source column. The absence has no provenance, so a later Scan may repopulate it from embedded metadata. A source left on an empty slot would outrank file metadata and block that. Title cannot be cleared.

Earlier Identify clears did leave the plugin source on the absent value. Those rows are not migrated in this slice. A blanket "null the source wherever the value is absent" migration is wrong, because the Edit form stores a cleared value as absent plus `manual` on purpose, as a protected empty slot. A migration limited to plugin-sourced empty slots would be safe and can be added later. Until then, clearing an already-absent value through Identify still nulls a leftover source, so an editor can heal a stuck field by clearing it again.

**Relationships use aggregate provenance.** Authors and Narrators are ordered collections (Person identity and order are significant, plus role for Authors, where an empty role equals a nil role). Genres and Tags are unordered sets. The server resolves each entry through the normal find-or-create path and compares resolved IDs against the current relations, deleting and reinserting only when they differ. There is no per-entry provenance for Authors, Genres, Tags, Narrators, or Series memberships.

**Series membership gets its own source.** A nullable `books.series_source` holds aggregate provenance for the ordered membership collection and its Series Number groups (ADR 0005). It never describes a Series resource's name. It is backfilled by copying the first membership's `series.name_source`, which is the value the scanner already uses as the effective source, so existing Books keep their current Scan protection without inventing history. The scanner's proxy lookups are then deleted rather than kept as a fallback.

**Identifiers are the exception.** They keep two levels: aggregate provenance gates Scan replacement, and per-entry provenance records each entry's origin. An unchanged `(type, normalized value)` keeps its source, entries matching the proposal receive the plugin source, other new or edited entries become manual, and a whole collection equal to the proposal receives plugin aggregate provenance. Duplicate Identifier types are rejected before any delete or insert.

**Cover identity is the page number.** For page-based files, choosing the proposed page when it equals the stored `cover_page` is a no-op that skips re-extraction and preserves provenance. Image-based Covers have no identity check, so choosing the proposed image is always a Proposal Acceptance. Both paths write the complete Cover state (filename, MIME type, source, and page where applicable) in one column set. Image-based Cover provenance is informational because no Scan path gates on it.

## Considered options

- **Server-only attribution.** Rejected: the server never sees the Plugin Proposal, so it cannot tell an accepted value from an edited one. Sending the proposal alongside the final value would double the payload and duplicate the browser's comparison.
- **Browser-only attribution.** Rejected: the browser cannot canonicalize or resolve aliases the way the server does, so it would report no-ops as changes and downgrade stored `manual` sources.
- **The client sends canonical source strings.** Rejected: it lets a client write arbitrary strings into source columns, and it is how `files.name_source = 'user'` rows came to exist before they were migrated to `manual`.
- **Derived sort metadata shares the driving value's provenance.** Rejected for the reason given under Sort title.
- **A runtime fallback for `series_source`.** Rejected in favor of the backfill. A fallback would keep the conflation between a Series name's source and a Book's membership source alive in the scanner indefinitely.
- **Tombstones for cleared values.** Rejected: a suppression marker is a hidden permanent rule. Clearing should return the field to the state a fresh Scan would fill.
- **Content hashing for image Cover identity.** Rejected as out of scope. Nothing consults image Cover provenance today, so a false Proposal Acceptance costs nothing.

## Consequences

- Historical provenance is left alone. Whether a past Identify value was accepted or edited cannot be reconstructed, and generic `plugin` sources stay generic.
- The priority ranking, and `refresh` and `reset` behavior, do not change. A `manual` result from Identify is protected from ordinary Scans exactly like an Edit form change, and a Proposal Acceptance stays replaceable by another plugin.
- Partial removal from a relationship stays protected as a manual edit, while a complete clear may be repopulated by a Scan.
- Sidecars continue to omit source fields.
- The contract rolls out in slices: scalars first, then relationships, Series memberships, and Identifiers. Until a slice lands, its fields keep the previous behavior of stamping the plugin source.
