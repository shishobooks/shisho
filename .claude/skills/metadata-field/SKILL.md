---
name: metadata-field
description: Use when adding, removing, or significantly modifying a metadata field on books or files (e.g., adding a new field like "language" to the File model). Enumerates the cross-stack touchpoints, enforces a discovery-based workflow that uses the codebase itself as the checklist, and includes a verification phase that catches parallel code paths that would otherwise be missed.
---

# Adding or Changing a Metadata Field

## Why this skill exists

Adding a metadata field to books or files in Shisho touches ~20-25 files across the stack. There are multiple parallel code paths that silently drop fields if you miss them (three separate JS→Go parsers, two merge functions, a filter function, file generators for four formats, the M4B freeform atom writer, the identify apply handler, OPDS feeds, Kobo sync, the frontend edit dialog, the gallery filter, the identify review form, and the plugin config labels — plus docs and tests for each).

A static checklist decays: code gets added, names drift, new touchpoints appear. This skill does NOT use a hardcoded list. It uses the **current state of the codebase** as the checklist by discovering touchpoints dynamically through grep.

## When to invoke

- Adding a new field to `models.File` or `models.Book`
- Adding a new field that flows through the scanner → sidecar → download pipeline
- Adding a field that plugins should be able to set
- Significantly modifying an existing field's data flow (e.g., changing nullability, adding plugin support)

Do NOT invoke for: adding a purely ephemeral UI state field, a handler-internal helper, or a field that lives entirely within one subsystem (e.g., a scan job internal field).

## The four phases

Work through these in order. Do not skip ahead — Phase 1 discovery informs Phase 2 planning, and Phase 4 verification is the killer step that catches missed parallel paths.

### Phase 1 — Discovery

**Pick a proxy field.** Choose an existing field that's as similar as possible to the one you're adding. This is the most important decision in the skill — a good proxy gives you a comprehensive checklist automatically; a bad proxy misses touchpoints.

Proxy selection guide:
- **New nullable string on files** (e.g., language, description) → use `publisher` or `url`
- **New nullable boolean on files** → use something nullable like a future feature. If none exists, use a nullable scalar like `release_date` and manually add the bool-specific branches
- **New date on files** → use `release_date`
- **New relation (many-to-many or belongs-to)** → use `publisher` (belongs-to) or `narrators` (has-many)
- **New book-level field** → use `subtitle` or `description`
- **New audiobook-specific field** → use `audiobook_codec` or similar

**Run discovery greps.** Replace `PROXY` with your chosen proxy field name (both the Go `PascalCase` and the JSON `snake_case` form). Paste the output into your notes — you'll need it for Phase 2.

```bash
# 1. Find every Go file that references the proxy field. This is the master
# list of backend touchpoints. Anywhere the proxy appears, the new field
# probably needs to appear too.
git grep -il 'Publisher\|publisher' pkg/

# 2. Narrow by layer:
git grep -l 'publisher' pkg/mediafile/        # ParsedMetadata struct
git grep -l 'Publisher' pkg/models/           # File/Book model
git grep -l 'Publisher\|publisher' pkg/migrations/
git grep -l 'Publisher' pkg/sidecar/          # Sidecar types + conversion
git grep -l 'Publisher' pkg/downloadcache/    # Fingerprint
git grep -l 'Publisher\|publisher' pkg/worker/ # Scanner (multiple merge/filter funcs)
git grep -l 'Publisher\|publisher' pkg/books/ # Edit API + validators + handlers
git grep -l 'Publisher' pkg/epub/ pkg/cbz/ pkg/mp4/ pkg/pdf/  # Parsers
git grep -l 'Publisher\|publisher' pkg/filegen/  # File generators
git grep -l 'Publisher' pkg/kepub/            # KePub CBZ path
git grep -l 'Publisher' pkg/opds/             # OPDS feed
git grep -l 'Publisher' pkg/kobo/             # Kobo sync

# 3. Plugin bridge — there are THREE parallel JS→Go parsers plus several
# merge/filter/persist functions. All must be updated in lockstep.
git grep -l 'Publisher\|publisher' pkg/plugins/
git grep -n 'parseSearchResponse\|parseParsedMetadata\|convertFieldsToMetadata' pkg/plugins/hooks.go pkg/plugins/handler.go
git grep -n 'persistMetadata\|mergeEnrichedMetadata\|filterMetadataFields' pkg/worker/scan_unified.go pkg/plugins/handler.go

# 4. Frontend touchpoints.
git grep -il 'publisher' app/
git grep -l 'publisher' app/components/library/FileEditDialog.tsx
git grep -l 'publisher' app/components/library/IdentifyReviewForm.tsx
git grep -l 'publisher' app/components/files/FileDetailsTab.tsx
git grep -l 'publisher' app/components/pages/BookDetail.tsx
git grep -l 'publisher' app/components/pages/Home.tsx  # gallery filter
git grep -l 'publisher' app/utils/format.ts            # METADATA_FIELD_LABELS
git grep -l 'publisher' app/hooks/queries/             # query types
git grep -l 'publisher' app/types/                     # TypeScript types (tygo-generated + custom)

# 5. Plugin SDK + manifest validation.
git grep -l 'publisher' packages/plugin-sdk/
git grep -n 'ValidMetadataFields' pkg/plugins/manifest.go

# 6. Documentation.
git grep -l 'publisher' website/docs/
git grep -l 'publisher' pkg/*/CLAUDE.md

# 7. Find every ParsedMetadata constructor (any place that builds the struct
# from scratch is a place that might need your new field set).
git grep -n 'ParsedMetadata{' pkg/

# 8. Find every File / Entity constructor pattern in tests.
git grep -l 'models\.File{' pkg/ --include='*_test.go'
```

Save the union of everything these commands return. That's your implementation surface area. If it surprises you, investigate before continuing — there may be recently-added touchpoints not covered by the proxy.

**Check for the M4B Freeform write-back trap.** If your field lives in `mp4.Metadata.Freeform` (e.g., `com.shisho:*` or `com.pilabor.tone:*` atoms), verify that `pkg/mp4/writer.go` `buildIlst` actually iterates the Freeform map. Historically the writer dropped freeform atoms silently.

### Phase 2 — Planning

Produce a written plan that maps each discovered file to a specific change. Group by layer:

**Data model**
- Migration: `ALTER TABLE` + partial index if the field will be filtered/searched
- Go model: field + `_source` tracking field
- Run `mise tygo` to regenerate TypeScript types

**Parsing & extraction**
- `pkg/mediafile/mediafile.go` — add to `ParsedMetadata`
- Per-format parsers (EPUB, CBZ, M4B, PDF) — extract where applicable
- Normalization helpers if needed (e.g., `NormalizeLanguage`)

**Persistence & round-trip**
- Sidecar types + conversion
- Download fingerprint struct + computation
- File generators — EVERY format, plus KePub CBZ path
- Write round-trip tests FIRST (Red), then implement generators (Green)

**Scanner**
- `scanFileCore` in `pkg/worker/scan_unified.go` — metadata AND sidecar source priority handling
- `mergeEnrichedMetadata` — copy field from enricher results
- `filterMetadataFields` — declared/enabled toggle handling
- `runMetadataEnrichers` path if there are other consumers

**Edit API**
- Validator struct (`UpdateFilePayload` or similar)
- Handler update logic
- Supplement downgrade clearing (preserve or clear?)
- Validation normalization

**Plugin bridge (the easy-to-miss trio)**
- `pkg/plugins/manifest.go` `ValidMetadataFields`
- `pkg/plugins/hooks.go` `parseParsedMetadata` (fileParser hook path)
- `pkg/plugins/hooks.go` `parseSearchResponse` (metadataEnricher hook path)
- `pkg/plugins/handler.go` `convertFieldsToMetadata` (identify apply path)
- `pkg/plugins/handler.go` `persistMetadata` — the apply actually writes the field
- `packages/plugin-sdk/metadata.d.ts` — TypeScript types

**Other consumers**
- OPDS entry population in `pkg/opds/service.go`
- Kobo sync in `pkg/kobo/handlers.go`
- Any other format-adjacent subsystem

**Frontend**
- `FileEditDialog` — state, initial values, hasChanges, save payload, UI
- `BookDetail` expandable metadata
- `FileDetailsTab`
- `Home` gallery filter (if applicable)
- `IdentifyReviewForm` — defaults, state, hasChanges, submit, UI, plus
  `PluginSearchResult` type
- `METADATA_FIELD_LABELS` in `app/utils/format.ts` for the plugin config toggles
- Query hooks if new endpoints are added
- Curated lookup tables (e.g., languages) if applicable

**Docs**
- `website/docs/metadata.md`
- `website/docs/sidecar-files.md`
- `website/docs/supported-formats.md`
- `website/docs/plugins/development.md`
- Relevant `pkg/*/CLAUDE.md` files

**Tests**
- Parser extraction tests per format
- Round-trip test for each file generator
- Sidecar round-trip test
- Scanner merge/filter tests
- Plugin bridge parser tests (all three)
- Edit API handler test (including downgrade)
- Frontend unit tests where applicable

### Phase 3 — Implementation order

Work in this order to minimize rework:

1. **Migration + model + `mise tygo`** — everything downstream needs the type
2. **`ParsedMetadata` field** — parsers and plugins both depend on it
3. **Parser extraction per format** — write extraction tests alongside
4. **Round-trip test for file generators (RED)** — write the test that asserts the field survives write → read. This should fail.
5. **File generators (GREEN)** — implement write-back until the round-trip test passes
6. **Sidecar + fingerprint** — simple additive changes
7. **Scanner integration** — metadata + sidecar source priority
8. **Edit API** — payload, handler, downgrade
9. **Plugin bridge** — all three JS→Go parsers + manifest validation + filter/merge/persist
10. **Other backend consumers** — OPDS, Kobo, etc.
11. **Frontend edit + display**
12. **Frontend identify review form + plugin config labels**
13. **Docs + CLAUDE.md**
14. **`mise check:quiet`** to verify

### Phase 4 — Verification (the killer step)

Before opening a PR, run these greps to find parallel code paths that reference your proxy field but NOT the new field. Any hit is a potential miss.

```bash
# For every file that mentions the proxy field, check whether it also
# mentions the new field. Anything that doesn't is a potential miss —
# investigate each one.
for f in $(git grep -il 'publisher' pkg/ app/); do
  if ! grep -qi 'NEW_FIELD' "$f"; then
    echo "MAYBE MISSING: $f"
  fi
done

# Extra safety nets:
# 1. Every ParsedMetadata constructor should set the new field (or
# intentionally leave it nil with a comment).
git grep -n 'ParsedMetadata{' pkg/

# 2. Every plugin test fixture JS that returns metadata should be
# updated if your field is returnable by plugins.
git grep -l 'return {' pkg/plugins/testdata/

# 3. Verify tests actually assert the new field end-to-end, not just
# compile. Run:
mise test
mise test:js
mise check:quiet
```

Investigate every "MAYBE MISSING" hit. Most will be false positives (test helpers, unrelated code). The ones that aren't are exactly the class of bug that takes multiple review rounds to catch.

## Red flags during the work

If any of these occur, stop and reassess:

- **You find a function that builds `ParsedMetadata` that you didn't know about.** There may be more parallel paths — re-run Phase 1 discovery.
- **You find that the proxy field is handled inconsistently in two places.** That's a pre-existing bug your new field is about to inherit. File a Notion task.
- **Your round-trip test passes trivially without reading the write path.** The test isn't actually exercising the round-trip — fix it.
- **You're tempted to skip the Phase 4 verification greps because "it works locally".** That's exactly how the bugs that required 9 review rounds on the language/abridged PR got through. Do the greps.

## Why this skill doesn't have a hardcoded checklist

A hardcoded checklist decays: it goes out of date as the codebase grows. Every time a new parallel code path is added (e.g., a third JS→Go parser, or a new frontend display component), a static checklist gets stale and misses touchpoints. The discovery-based approach self-corrects because it reads the current codebase state every time it runs. Trust the grep, not a historical list.

If you find that this skill is consistently missing a category, add a new grep to Phase 1, not a new bullet to Phase 2.
