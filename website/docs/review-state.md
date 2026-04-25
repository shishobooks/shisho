---
sidebar_position: 60
---

# Review State

Each book and file in your library has a **Reviewed** state that helps you track which books still need your attention. The library gallery offers a "Needs review" filter so you can work through books one at a time without losing track.

## Auto-flip

Files automatically flip to **Reviewed** when they have all the metadata fields you've marked as required. The defaults are:

- **All books:** authors, description, cover, genres
- **Audiobooks (additional):** narrators

If a file is missing any of these, it stays in your "Needs review" queue.

When you fill in a missing field — through the edit dialog, a plugin enrichment, or the identify dialog — the flag flips automatically. If you delete a field, the file returns to the queue (unless you've manually marked it).

## Manual Override

If you want to keep a book in the queue regardless of completeness, or sign off on a book that will never have certain fields, use the toggle on the book or file edit page. Manual choices are sticky in both directions and persist until you change them again.

## Filter and Badge

The library gallery shows a small **Needs review** badge on books that have at least one main file outstanding. Open the **Filter** sheet to switch between "All", "Needs review", and "Reviewed".

## Bulk Actions

When multi-selecting books in the gallery, use the **More** menu in the action bar to mark all selected books as reviewed (or needs review) at once.

## Configuring Required Fields

Admins can change the required-field set on the **Settings → Review Criteria** page. Saving updated criteria triggers a background recompute of every main file. If you have manual overrides, you'll see a prompt asking whether to clear them — pick what fits your situation.

## Cross-reference

- [Metadata](./metadata.md) — what each field means.
- [Plugins](./plugins/overview.md) — plugins fill in metadata that drives review state.
