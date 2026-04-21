# Fetch chapters from Audible: design

**Date:** 2026-04-21
**Status:** Approved

## Overview

Users can fetch chapter titles and timestamps from Audible (via the Audnexus public API) and apply them to an M4B file's chapters in the existing chapter edit flow. Apply stages the data in the edit form without saving, so the user can spot-check with the existing per-chapter play buttons, tweak if needed, and commit with Save or discard with Cancel.

Scope: M4B files only. No viable external chapter APIs exist for EPUB or CBZ, so no generalized "chapter provider" abstraction.

## Motivation

Ripped audiobook files often ship with wrong or missing chapter data. Audible has authoritative chapter titles and timestamps for every book in its catalog. Audnexus (`api.audnex.us`) proxies this data as a free community service. Surfacing this inside Shisho removes the need for users to hand-edit every chapter or drop into external tools.

## Non-goals

- EPUB or CBZ chapter fetching (no viable public APIs).
- Audible search within Shisho (v1 requires the user to provide an ASIN; paste-from-URL is enough for the common workflow).
- Automatic enrichment during scans (user-initiated only in v1).
- Persistent disk cache across server restarts.

## Architecture

### New backend package: `pkg/audnexus/`

- `service.go`: HTTP client with a 5-second request timeout and an in-memory TTL cache keyed by ASIN. TTL: 24 hours. Cache only successful responses; retries on errors should hit upstream. `sync.RWMutex` around a `map[string]*cachedResponse`. No eviction beyond TTL (one entry per ASIN, trivial footprint).
- `handlers.go` + `routes.go`: single endpoint `GET /api/audnexus/books/:asin/chapters`. Requires auth plus `books:write` permission (see Permissions below). Invalid ASIN format returns 400 without upstream call.
- `User-Agent` header: `Shisho/<version>` on every upstream request.
- Wired into `cmd/api/main.go` alongside other services.

### API surface

**Endpoint:** `GET /api/audnexus/books/:asin/chapters`

**ASIN validation:** 10 characters, alphanumeric. Reject with 400 `{ code: "invalid_asin" }` before any upstream call.

**Response (200):**
```json
{
  "asin": "B0036UC2LO",
  "is_accurate": true,
  "runtime_length_ms": 163938000,
  "brand_intro_duration_ms": 38000,
  "brand_outro_duration_ms": 62000,
  "chapters": [
    { "title": "Prelude", "start_offset_ms": 0, "length_ms": 272000 },
    { "title": "Prologue: To Kill", "start_offset_ms": 272000, "length_ms": 1063000 }
  ]
}
```

Field names use `snake_case` per project API conventions. This is a one-to-one passthrough of Audnexus fields (converted from the upstream camelCase).

**Error responses:**
- `400 { code: "invalid_asin" }`: ASIN format check failed.
- `404 { code: "not_found" }`: Audnexus returned 404.
- `504 { code: "timeout" }`: Upstream did not respond within 5s.
- `502 { code: "upstream_error" }`: Any other upstream failure (5xx, connection error, malformed JSON).

### Permissions

Endpoint requires `books:write` (not `books:read`). Rationale:

- The only legitimate use of fetched data is staging it into an editable chapter form, which requires write access to save.
- Keeps the UI and backend gate coherent: if the "Fetch from Audible" button isn't shown to this user, the endpoint isn't usable either.

### Caching

- In-memory, server lifetime only, no persistence.
- Keyed by ASIN (normalized to uppercase before lookup).
- TTL 24 hours (Audnexus data rarely changes).
- Only successes cached; errors pass through so retries work.

### Rate limiting

None in v1. Audnexus is community-run and generous, and self-hosted Shisho instances won't generate significant load. Revisit if it becomes an issue.

## Frontend

### New files

- `app/hooks/queries/audnexus.ts`: `useAudnexusChapters(asin)` tanstack query hook. `enabled: false` by default; triggered by dialog Fetch button.
- `app/components/files/FetchChaptersDialog.tsx`: the dialog component.
- `FileChaptersTab.tsx`: updated to mount the button in three places and wire the apply callback into `editedChapters` state.

### Entry points

The "Fetch from Audible" button appears in three surfaces (all M4B-only, all require `books:write`):

1. **View mode header**, alongside the existing Edit button.
2. **Empty state**, alongside the "Add chapter" button.
3. **Edit mode**, alongside the "Add chapter" button at the bottom of the chapter list.

All three entry points open the same dialog. On apply, the data is injected into `editedChapters` and the UI enters edit mode (if not already).

### Dialog states

Visual reference: `tmp/mockups/fetch-chapters.html` (committed snapshot of approved mockups).

**State 1: ASIN entry.**
- Single input (monospace font) labeled "Audible ID (ASIN)".
- Prefilled from `file.identifiers` when present; shows a checkmark + "Using this file's existing Audible ID." when prefilled.
- Fetch button disabled until input is 10 alphanumeric characters.
- Footer: "Powered by [Audnexus](https://audnex.us)" attribution link.

**State 2: Loading.** Spinner and "Looking up chapters on Audible…". Cancel available throughout.

**State 3: Result.** Shows:
- Book title and ASIN in the header.
- Duration comparison block:
  - Audible runtime (with intro and outro durations when nonzero)
  - Your file's duration
  - Chapter count comparison ("N from Audible · M in your file")
- Trim detection info callout (see Trim detection below).
- Single checkbox: "Offset chapters by intro duration (−<intro>s)". Checked or unchecked by auto-detection; user can flip.
- Overwrite warning when the parent edit form already has unsaved changes.
- Two apply buttons (see Apply behavior).

**State 4: Error.** Human-readable error message based on the error code. Retry button re-runs the fetch.

### Trim detection

Audible files that have had the Audible-branded intro and/or outro stripped (common with Libation rips) have durations that don't match Audnexus's `runtime_length_ms`. Chapter timestamps are offset relative to the original full file, so applying them as-is would be wrong by up to the intro duration.

**Detection algorithm (runs after a successful Audnexus fetch):**

1. Compute four candidate durations:
   - `runtime_length_ms` (intact)
   - `runtime_length_ms - brand_intro_duration_ms`
   - `runtime_length_ms - brand_outro_duration_ms`
   - `runtime_length_ms - brand_intro_duration_ms - brand_outro_duration_ms`
2. Pick the candidate closest to `file.audiobook_duration_seconds * 1000`.
3. If the closest candidate is within ±2000ms (2 seconds) tolerance:
   - If the matching candidate subtracts intro → set `applyIntroOffset = true`.
   - Otherwise → set `applyIntroOffset = false`.
4. If no candidate matches within tolerance: set `applyIntroOffset = false` (closest match default) and show a mismatch warning in the dialog.

**User-facing control:** a single checkbox, "Offset chapters by intro duration (−<intro>s)." The checkbox state is what determines the applied offset. All four detection candidates exist only to set the checkbox's initial value; they are not exposed in the UI.

**Rationale for the single checkbox:** Only two behaviors are actually possible when applying timestamps: subtract the intro duration or don't. The outro affects total file length but not chapter start times, so it does not map to a distinct offset. Exposing four radio options for two outcomes adds friction without value.

### Apply behavior

Two apply buttons:

**"Apply titles only"** (primary when available; disabled with tooltip when chapter counts differ):
- For each index `i`, replace `editedChapters[i].title` with `audnexus.chapters[i].title`.
- Leave `start_timestamp_ms` values untouched.
- Offset checkbox has no effect (timestamps are not touched).
- Disabled state tooltip: "Chapter counts don't match (N vs M)."

**"Apply titles + timestamps"** (primary when titles-only is disabled; secondary otherwise):
- Replace `editedChapters` entirely with a new array derived from Audnexus.
- For each Audnexus chapter:
  - `title = audnexus.chapters[i].title`
  - `start_timestamp_ms = audnexus.chapters[i].start_offset_ms - offset`, where `offset = brand_intro_duration_ms` when the checkbox is checked, `0` otherwise.
  - `children = []`, fresh `_editKey`.
- Negative post-offset timestamps are clamped to 0 (only possible for the first chapter).
- After injection, run the existing `normalizeChapterOrder` to sort by `start_timestamp_ms` defensively.

**Out-of-bounds handling:** timestamps that exceed `file.audiobook_duration_seconds * 1000` after offset are NOT clamped or dropped. The existing per-row validation in `ChapterRow` flags them and blocks Save, giving the user clear visual signal about which chapters need manual adjustment. Silent truncation would hide the problem.

**After apply:**
- Dialog closes.
- Component enters (or stays in) edit mode.
- User spot-checks with the existing per-chapter play buttons, adjusts timestamps if needed, Saves or Cancels.
- Cancel reverts to the pre-fetch state (empty chapters if the fetch was from empty state, prior chapters otherwise).

**Overwrite warning:** if the user re-opens the dialog with unsaved edits already staged (`hasChanges === true`), show a warning callout in the result view: "Unsaved changes will be overwritten." Apply buttons remain enabled. Warning is suppressed when `hasChanges === false`.

### Copy and naming

- Button label: **"Fetch from Audible"**.
- Dialog title: **"Fetch chapters from Audible"**.
- Attribution: small "Powered by [Audnexus](https://audnex.us)" link in the dialog footer.
- Input label: **"Audible ID (ASIN)"**. Long form on the label, short form in the dialog body and elsewhere.
- Copy contains no em dashes. Use colons for label:value structure and periods for sentence breaks.

## Error surfaces (frontend)

All errors render in State 4 of the dialog:

- `invalid_asin` / pre-fetch format check: "Check the ASIN format. It should be 10 alphanumeric characters."
- `not_found`: "We couldn't find this ASIN on Audible. Double-check the ID on the Audible book page."
- `timeout`: "Request timed out. Try again."
- `upstream_error`: "Couldn't reach Audible. Try again in a moment."

Retry re-runs the same fetch with the same ASIN.

## Testing

### Backend

- `pkg/audnexus/service_test.go` (`t.Parallel()` for pure function cases; serialized for cache tests that share state):
  - Happy path: upstream 200 → service returns parsed response.
  - 404 → `not_found` error.
  - Upstream 500 → `upstream_error`.
  - Timeout (stub server that sleeps past the 5s deadline) → `timeout`.
  - Invalid JSON → `upstream_error`.
  - Cache hit: second call with same ASIN doesn't hit upstream within TTL.
  - Cache miss after TTL: re-fetches.
  - ASIN normalization (lowercase input → uppercase cache key).
- `pkg/audnexus/handlers_test.go`:
  - Returns 200/404/504/502 for corresponding service states.
  - Returns 400 for invalid ASIN without calling upstream.
  - Enforces `books:write` permission (401 unauth, 403 without permission).

### Frontend

- `FetchChaptersDialog.test.tsx` (vitest + RTL):
  - Renders with ASIN prefilled from file identifier; checkmark note present.
  - Fetch button disabled until input is valid ASIN format.
  - Loading → result transition on successful fetch.
  - Trim detection auto-selection: parameterized table of four scenarios (intact, intro-only, outro-only, both).
  - Offset checkbox honors user flip: fetch response shows checked by default for Libation case, unchecked for intact.
  - "Apply titles only" disabled with tooltip when counts differ.
  - Overwrite warning appears only when `hasChanges === true`.
  - Apply buttons invoke callbacks with the expected payload shape.
- `FileChaptersTab.test.tsx` additions:
  - "Fetch from Audible" button visible only for M4B files.
  - Button hidden when user lacks `books:write`.
  - Dialog open/close wiring.
  - Apply-titles-only replaces titles by position without touching timestamps.
  - Apply-titles-and-timestamps replaces whole array with offset applied.
  - Out-of-bounds timestamps trigger existing validation (not clamped).

E2E tests deferred from v1.

## Docs

Required per CLAUDE.md's user-facing-change rule.

- `website/docs/`: add a new page (or section in an existing chapter-related page) covering:
  - What the feature does.
  - Where to find ASINs on Audible.
  - Trim detection behavior.
  - Apply modes.
  - Permission requirements.
  - Sidebar updated to list the page; cross-linked from the M4B format docs.
- `pkg/audnexus/CLAUDE.md`: new file following the pattern of other package CLAUDE.md files (service overview, caching behavior, error codes).
- `pkg/mp4/CLAUDE.md`: brief "External chapter source" section pointing to `pkg/audnexus/`.

## Open questions / follow-ups

- Per-user or per-IP rate limiting on the endpoint if Audnexus asks us to throttle.
- Persistent cache across server restarts (low priority; 24h TTL already minimizes re-fetches within a session).
- Search-by-title flow to find ASINs without visiting Audible (post-v1).
- Extending the dialog to other audiobook formats (MP3 sets, future M4A support).
