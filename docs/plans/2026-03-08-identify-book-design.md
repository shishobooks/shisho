# Identify Book Dialog — Design

## Overview

Manual book identification UI that lets users search external metadata sources (via enricher plugins) and apply a selected result to a book.

## Backend (already built)

- `POST /plugins/search` — takes `{ query, book_id }`, calls all enricher plugins' `search()` hooks, returns aggregated `SearchResult[]`
- `POST /plugins/enrich` — takes `{ plugin_scope, plugin_id, book_id, provider_data }`, calls a specific plugin's `enrich()`, applies metadata, returns updated book
- Frontend query hooks (`usePluginSearch`, `usePluginEnrich`) already exist in `app/hooks/queries/plugins.ts`

## Trigger

New item in the book detail's overflow dropdown menu (the ⋮ menu): "Identify book" with a `Search` icon. Placed between "Refresh all metadata" and the "Merge" separator.

## Dialog Structure

Modal dialog (`max-w-3xl`) with:

1. **Header**: "Identify Book"
2. **Search bar**: Text input pre-filled with book title, plus a "Search" button. Auto-triggers search on dialog open. User can edit and re-search.
3. **Results area** (scrollable, `max-h-[60vh]`):
   - Loading: spinner
   - Empty: "No results found" message
   - Results: list of cards, each showing:
     - Cover thumbnail (from `image_url`, left side)
     - Title (bold), author(s), release date, publisher
     - Identifiers as small badges (e.g., ISBN)
     - Plugin source badge (subtle)
   - Selected result gets a highlight ring
4. **Footer**: Cancel + Apply. Apply disabled until a result is selected, shows loading during enrich.

## Flow

1. Dialog opens → search input pre-filled with book title → search fires immediately
2. User reviews results, optionally refines query and re-searches
3. User clicks a result to select it (highlighted)
4. User clicks "Apply" → enrich endpoint called → on success, dialog closes, book query invalidated
5. On error, show toast

## Not needed

- Unsaved changes protection (action dialog, not a form)
- New routes (dialog opens from existing book detail page)
