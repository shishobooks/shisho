# Unified publisher hierarchy instead of separate publisher and imprint tables

We had separate `publishers` and `imprints` tables with independent `publisher_id` and `imprint_id` fields on files. This caused three problems: (1) external sources like Goodreads list imprints as publishers, requiring manual correction; (2) the two fields could get out of sync since imprints are inherently tied to a publisher; (3) the publishing industry has multiple hierarchy levels (conglomerate → division → imprint) that don't map to a flat two-table model.

We chose to unify into a single `publishers` table with a self-referential `parent_id` for hierarchy. Imprints, divisions, and conglomerates are all publishers at different levels of the tree. A file references exactly one publisher at whatever level of specificity is known. Ancestor publishers are derived by walking up the tree via recursive CTE.

## Key decisions

- **Adjacency list** (not closure table) — hierarchy is shallow (3-4 levels), SQLite supports recursive CTEs, and we don't need heavy tree queries in hot paths.
- **Files attach at any level** — metadata sources vary in specificity; forcing leaf-node attachment would require placeholder nodes or lose data.
- **Clean break** — `imprint` removed entirely from sidecar files, plugin SDK, and format parser outputs. No backwards-compatibility shim. Justified by 0.0.X status.
- **Migration strategy** — copy imprints into publishers (skip on name conflict), copy imprint aliases into publisher aliases (skip on conflict), set file's publisher to the imprint value (more specific), drop imprint tables. No auto-generated hierarchy; parent relationships are a manual curation task.
- **Cycle prevention** — validate on write (walk ancestor chain), exclude invalid options from comboboxes.

## Considered options

- **Keep publisher + imprint as separate tables**: Status quo. Simple model but doesn't match industry reality, causes data sync issues, and external sources don't distinguish the two.
- **Single publisher field, no hierarchy**: Simpler, but loses the ability to see "all books under Penguin Random House" when files are attached to specific imprints.
- **Closure table for hierarchy**: More powerful tree queries, but more complex to maintain (insert/move operations update many rows) for marginal benefit given shallow trees.
