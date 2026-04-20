---
sidebar_position: 20
---

# Gallery sort

The library gallery can be sorted by one or more fields — author, series, date added, and more — with each level ascending or descending. You can share a sorted view as a URL, save it as your default for that library, or reset back to the builtin default.

## Using the Sort sheet

Click **Sort** in the gallery toolbar to open the sort sheet. The sheet shows the current sort levels in priority order (top = primary). From the sheet you can:

- **Add a level** — in the "Then by…" section at the bottom of the sheet, click the field you want to sort by next.
- **Remove a level** — click the × on its row.
- **Change direction** — click the ↑ / ↓ button on its row to flip ascending/descending.
- **Reorder levels** — drag the grip handle on the left edge of each row.

A dot on the Sort button means your current sort differs from your saved default.

## Available sort fields

| Field | Sorts by |
|-------|----------|
| Title | Book title |
| Author | Primary author's name (books with no author sort to the end) |
| Series | Primary series name, then series number within each series (the within-series order is always ascending — "Stormlight #1 before #2" — even when you pick **Series, descending**, which only flips the series-name ordering) |
| Date added | When the book was first scanned into the library |
| Date released | Release date from the primary file's metadata |
| Page count | Page count from the primary file |
| Duration | Audiobook duration from the primary file |

For Date released, Page count, and Duration: if the primary file doesn't have a value, Shisho falls back to any other file on the book that does. Books with no value on any of their files sort to the end, regardless of ascending/descending.

## URL-addressable sorts

Non-default sorts live in the URL as `?sort=field:dir,field:dir`. You can share or bookmark a sorted view and it reloads in the same order.

Examples:

```
?sort=author:asc
?sort=author:asc,series:asc
?sort=date_added:desc
```

When the URL has no `sort` parameter, the gallery uses your saved default for that library (or the builtin default — **Date added, newest first** — if you haven't saved one).

## Saving a default for this library

When your current sort differs from your saved default, the sort sheet shows a **Save as default** button. Clicking it:

1. Saves your current sort as the new default for this library.
2. Clears the `?sort=` parameter from the URL — you're now viewing the default.

Defaults are per-user per-library, so each library can have its own saved sort and each user's saved sorts are their own.

## Resetting to the default

When a non-default sort is active, a **reset to default** link appears next to the sort chip row. Clicking it clears the `?sort=` parameter and returns the gallery to your saved default.

## How this affects other surfaces

Your saved library sort also applies to the OPDS feeds and the eReader browser for that library. See [OPDS](./opds.md) and [eReader browser](./ereader-browser.md) for details.
