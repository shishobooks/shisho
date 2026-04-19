# Plugins UI Redesign Design

## Overview

The current `/settings/plugins` page is a dense 4-tab configuration surface (`Installed · Browse · Order · Repositories`) with no plugin-level focus view, no changelog display, and a cramped row layout that pushes state indicators (enable/disable switch, update button, settings, uninstall) into 32px of trailing real estate. Capabilities with more than one badge clobber the actions column; changelogs live in backend data but never reach the UI; and users have no "canonical" place to read about a plugin before installing or updating it.

This spec redesigns the page around two ideas:

1. **Collapse the list to two tabs (`Installed` / `Discover`)** and move the power-user surfaces (`Order`, `Repositories`) behind a gear-icon "Advanced" button in the page header. The tab-label pill on `Installed` becomes the ambient update-available signal for the entire app.
2. **Add a plugin detail page** at `/settings/plugins/:scope/:id` that holds the per-plugin config, version history with rendered markdown changelogs, permissions (derived from manifest capabilities), and lifecycle actions (enable/disable, update, reload, uninstall).

Plus consistent logo treatment, an optional `releaseDate` field on version metadata, and docs updates covering both.

A visual reference (HTML mockup of the two tabs, detail page, and logo treatment) was produced during the brainstorm in `.superpowers/brainstorm/…/content/b-refined-v2.html`. That file is worktree-local and ephemeral — this spec is the durable source of truth. The mockup is useful as a quick visual companion during the implementation but is not required to understand any requirement below.

## Non-goals

- Sidebar nav echo of the update-count pill (deferred; spec mentions the hook but doesn't implement it).
- Multi-repository federated search / ranking.
- Confirm modals on `Update` or `Uninstall` actions — the in-place pattern is deliberate.
- Per-library plugin overrides beyond what already exists in `library_plugin_hook_configs` and `library_plugin_field_settings`.
- Reload-from-disk for anything other than `scope = "local"` plugins.

## 1. Information Architecture

### Current

`AdminPlugins.tsx` renders 4 tabs — Installed, Browse, Order, Repositories — wired to route `/settings/plugins/:tab?`. Every tab is a flat list inside the same page.

### New

Two top-level tabs only: **Installed** and **Discover** (`Browse` is renamed). `Order` and `Repositories` move behind a `⚙` icon button in the page head, which opens a dialog or drawer with the existing Order + Repositories UI stacked as sections. No functionality is removed; it's relocated.

Route layout:

- `/settings/plugins` — tab index, defaults to `Installed`
- `/settings/plugins/discover` — Discover tab selected
- `/settings/plugins/:scope/:id` — **new** plugin detail page (replaces `PluginConfigDialog` modal as the primary config surface; the modal is deleted)

Legacy deep links (`/settings/plugins/browse`, `/settings/plugins/order`, `/settings/plugins/repositories`):

- `/browse` redirects to `/discover` (same destination, renamed)
- `/order` and `/repositories` redirect to `/settings/plugins` (the new Advanced dialog is the destination but doesn't have its own URL — users land on the list tab, and we can optionally open the dialog via a `?advanced=order|repositories` query param on the redirect)

Deep links to `Order` / `Repositories` as their own pages are intentionally dropped — they're advanced surfaces and live inside a dialog.

### Tab-label pill (actionable, not decorative)

The pill next to `Installed` represents **count of plugins with an update available**, not total installed. It:

- renders **only** when count > 0
- uses an accent-tinted style (primary-color fill with reduced alpha + matching border) so it reads as actionable in both active and inactive tab states
- shows a tooltip on hover: `"{n} plugin{s} have an update available"` (correct pluralization)
- updates immediately when an update is applied (the plugin disappears from the "has updates" set, count decrements, pill disappears when count hits 0)

Discover gets no pill — total-available count is not actionable.

## 2. Installed Tab

### Row layout

```
[logo 40px]  Plugin Name  [Disabled badge?]  [Update X.Y.Z badge?]
             vX.Y.Z · [capability badge] [capability badge] · author
             (capabilities wrap to additional meta-line rows if too many)
                                                                        [Update btn?]  ›
```

Changes from current:

- **No row-level enable/disable switch.** Enable/disable moves to the detail page. Row is strictly a link (chevron confirms).
- **Disabled rows** are dimmed (`opacity: 0.45`, `filter: saturate(0.3)`), sorted to the bottom under a subtle separator, and carry an inline `Disabled` badge next to the name so state reads without a toggle.
- **Capabilities live on the meta line** as outline badges and wrap to additional lines naturally when there are many. No overflow into the actions column.
- **Update badge** sits next to the plugin name (`Update 1.5.0`). A primary `Update` button in the actions column triggers in-place update.
- **Entire row is a link** to the detail page. Action buttons use `stopPropagation` so clicks don't bubble.

### Sort order

1. Enabled plugins, alphabetical by display name
2. Separator (subtle 1px border)
3. Disabled plugins, alphabetical

### Update flow (in-place)

Clicking `Update` on a row:

1. Button swaps to a spinner + `Updating…` label, `pointer-events: none`
2. `POST /plugins/installed/:scope/:id/update` called with the target version
3. On success: update badge removes, version number in meta line rewrites to the new version, button removes, tab-label pill decrements (disappears if count → 0)
4. On failure: button reverts, error toast surfaces the server message
5. No modal, no nav, no banner. The row IS the flow.

### Page head

- Title: "Plugins"
- Description: existing copy
- Actions (top-right):
  - `Scan local` button — existing behavior (scans `pluginDir/local/` for new local plugins)
  - `⚙` icon button — opens the Advanced dialog (Order + Repositories)

### Removed UI

- The top-of-page "Update available" banner — the tab pill + row signal is the ambient cue.
- The enable/disable switch on rows — moved to detail.

## 3. Discover Tab

Shares the row component with Installed. Differences:

- **Filter row** above the list: a search input (name + description substring), a capability `<select>` (All / Metadata Enricher / Input Converter / File Parser / Output Generator), and a source `<select>` (All / shisho / community / any other configured repo scopes)
- **Actions column** shows either:
  - `Install` primary button — for available plugins
  - `Installed` disabled button with a success-outline tint + leading dot — for plugins already in the installed list (state is unmistakable at a glance; the in-row name does NOT need a duplicate "Installed" badge)
  - Disabled outline `Install` button with reduced opacity + an inline `Incompatible · needs Shisho ≥ X.Y.Z` warning badge next to the name — for incompatible plugins (`AvailablePlugin.minShishoVersion` > current Shisho version)
- Click row → the same detail page (`/settings/plugins/:scope/:id`). The page is polymorphic: see §4 for how it handles not-yet-installed plugins.

## 4. Plugin Detail Page — `/settings/plugins/:scope/:id`

New route + component. Replaces `PluginConfigDialog` as the primary per-plugin surface. Also serves as the detail view for uninstalled plugins reached via Discover.

### Load & polymorphism

On mount, the page fires both `GET /plugins/installed/:scope/:id` and `GET /plugins/available/:scope/:id` in parallel. The result is one of three states:

- **Installed** — `installed` fetch succeeds. Render the full page (hero with Update/Enable/icon-actions, Version history, Configuration, Permissions, Danger zone).
- **Available only** — `installed` returns 404, `available` succeeds. Render a reduced page: hero shows logo/name/meta/description + a primary `Install` button in place of the Update/Enable block. Version history renders (newest version is tagged `Latest` rather than `Available now`). Configuration, Permissions (derived from manifest declarations in the `AvailablePlugin`), and Danger zone are hidden — there's nothing installed to configure or uninstall.
- **Neither** — both 404 or an unknown plugin id. Show a simple "Plugin not found" empty state with a back-link.

After a successful Install click, refetch both queries. The page transitions to the installed state in place without a navigation event.

### Breadcrumb

`← Plugins` — navigates back to the list (preserves prior tab via query state).

### Hero card

```
[logo 64px]   Plugin Name  [Update available — X.Y.Z?]
              vX.Y.Z installed Apr 10, 2026 · by Author · homepage ↗
              [capability badges]
              Description paragraph.
                                                         [Update to X.Y.Z?]
                                                         Enabled  [toggle]
                                                         [🔄] [📄] [📂]
```

Right rail, top to bottom:

1. **Primary `Update to X.Y.Z` button** (only when update available). Same in-place flow as the row, but on the detail page it also rewrites the hero version meta and removes the update-available badge next to the name.
2. **Enable/disable row** — `Enabled` label + toggle. Uses the existing `PATCH /plugins/installed/:scope/:id` endpoint with `enabled: boolean`.
3. **Icon actions** — three `btn-icon` buttons, each with a `data-tooltip`:
   - **Reload from disk** (tooltip: "Reload plugin from disk") — only rendered when `plugin.scope === "local"`. Posts to `POST /plugins/installed/:scope/:id/reload` (existing endpoint). Non-local plugins simply don't show the button — no greyed-out state.
   - **View manifest** (tooltip: "View manifest") — opens a dialog or drawer showing the raw `manifest.json` with syntax highlighting. Read-only.
   - **Open plugin folder** (tooltip: "Open plugin folder") — copies the plugin's absolute path *as reported by the server* to the clipboard and shows a toast ("Plugin folder path copied"). Shisho is a browser app; we can't open Finder, but the path in the clipboard is the next best affordance. For `local` plugins this is the user's own dev folder (if the UI and server run on the same host) or a path inside the container (Docker). For installed plugins it's the server's plugin dir. Users running Docker will get a container-internal path — documented in §11 as a known limitation.

### Version history section

Renders `AvailablePlugin.Versions` for the plugin (sourced via `GET /plugins/available/:scope/:id`).

Each version rendered as a `version-card`:

```
vX.Y.Z   [Available now?]              Released <date> · <relative>
─────────────────────────────────────────────────────────────────────
<rendered markdown changelog>

[Update now?]  [View full diff on GitHub ↗?]
```

Rules:

- Versions newer than the currently installed one are highlighted with a tinted border + gradient background, tagged with an `Available now` badge. For **uninstalled** plugins viewed from Discover, the highest-versioned compatible entry uses a `Latest` badge (same tinted treatment) instead of `Available now`, and no per-version `Update now` button renders — installation is driven from the hero rail.
- The currently installed version is tagged with a green `Installed` pill
- Older versions (not `Available now`, not `Installed`) render with a plain border
- `Released <date>` line renders only if `version.releaseDate` is present (see §6). When present, also show relative time ("3 days ago") alongside the absolute date.
- **Changelog is rendered as markdown** using `react-markdown` (new dependency) with a restricted plugin set: headings (h2/h3), paragraphs, lists, inline code, code blocks, links (opens in new tab with `rel="noopener noreferrer"`), bold, emphasis. Sanitize with `rehype-sanitize` — plugin authors write these but we display them, so treat them as untrusted HTML.
- `View full diff on GitHub ↗` link is derived from the plugin's `homepage` field if it points to a GitHub repo; otherwise omit the link. No separate manifest field.
- "Show N older versions" collapsed row at the bottom; expand in-place.

### Configuration section

Unchanged from `PluginConfigDialog` conceptually — render the existing config schema as a form with `Save configuration` and `Reset to defaults`. Field types (`string`, `boolean`, `number`, `select`, `textarea`) already handled by the current dialog code. Move the existing form rendering code into a standalone `PluginConfigForm` component so it's reusable (detail page + tests).

For enrichers, the field-toggles UI (global + per-library) from the current dialog moves to a subsection below the config form. Same behavior.

### Permissions section

Derived from manifest capabilities — no new data required. Renders one `perm-row` per capability that implies permission:

- `httpAccess` → "Network access" with the list of domain patterns
- `fileAccess` → "Filesystem access" with the level (`read` / `readwrite`)
- `ffmpegAccess` → "FFmpeg access"
- `shellAccess` → "Shell commands" with the allowlist

This is read-only display. No toggles — capabilities are declared by the plugin; users can only uninstall if they don't want them.

### Danger zone

Single `danger-zone` block at the bottom:

- Heading: "Uninstall plugin"
- Sub: "Removes the plugin and its files. Plugin configuration will be discarded."
- Destructive-outline `Uninstall` button

Clicking `Uninstall` shows a confirm dialog (this is destructive; it IS an exception to the no-confirms rule), then calls `DELETE /plugins/installed/:scope/:id` and navigates back to `/settings/plugins`.

## 5. Plugin Logos

### Treatment

Every logo renders with identical styling regardless of source (repo-provided `imageUrl`, transparent SVG, or generated initial):

| Property | Value |
|----------|-------|
| Aspect ratio | 1:1 (forced via CSS `aspect-ratio: 1 / 1`) |
| Border radius | 6px at 40px, 10px at 56px, 12px at 64px |
| Backdrop | `oklch(1 0 0 / 5%)` muted fill |
| Inner padding | 6px at 40px (10px at 64px) — applied via `.logo-fit` wrapper so transparent logos get breathing room |
| Image fit | `object-fit: contain` |
| Sizes | 24px (future nav/pills), 40px (list rows), 64px (detail hero) |

### Source of truth

`AvailablePlugin.ImageUrl` already exists on the repo manifest. Installed plugins source their logo by joining back to the repo entry via `(scope, id)`. No new field on the installed-plugin manifest. If the plugin's source repo is no longer enabled/available, fall through to the initials fallback.

### Fallback

When `imageUrl` is missing, unreachable, or returns a non-image response: render one or two initials on a color hashed deterministically from `scope/id` (so the same plugin always renders the same color).

Initials derivation:

- If `id` contains a hyphen: first letter of the id + first letter after the first hyphen, uppercased (`google-books` → `GB`, `shisho-local-tagger` → `SL`)
- Else if `id` is a single word ≥ 2 chars: first two letters uppercased (`calibre` → `CA`, `audible` → `AU`)
- Else: the single letter uppercased (`c` → `C`)

Color palette: ~10 predefined hues (port the `.plugin-icon.gb`/`.plugin-icon.mb`/etc. utility classes from the mockup into a `getPluginFallbackColor(scope, id)` helper that returns one of the palette entries by hashing `scope + "/" + id` and modulo'ing into the palette).

### Broken image handling

On `<img>` error event, swap to the initials fallback. Implement inside a `<PluginLogo scope={...} id={...} imageUrl={...} size={40 | 64} />` component with internal `useState<boolean>` for the error case.

## 6. Manifest & Backend Deltas

### `releaseDate` — optional

The `PluginVersion` struct already has a `ReleaseDate` field stored as string. This spec:

- Keeps it **optional**. Plugins can omit it; the version card simply hides the "Released" line.
- Accepts RFC3339 (`2026-04-14T00:00:00Z`) OR date-only (`2026-04-14`). Parse both formats in the manifest loader (similar to the existing `releaseDate` parsing in `pkg/plugins/hooks.go` for enricher results).
- Add to the repository manifest schema documentation.

No migration needed. Existing repos without `releaseDate` continue to work.

### Local-plugin predicate

No new backend field. The UI and any reload-related gating uses `plugin.scope === "local"` directly — this is the existing convention (`pkg/plugins/handler.go:928` scans `pluginDir/local/` and tags those plugins with scope `local`; `pkg/plugins/CLAUDE.md` documents the structure as `{pluginDir}/{scope}/{id}/`).

### Update-count endpoint

The Installed-tab pill needs the count of plugins with updates available. `GET /plugins/installed` already returns each plugin's `update_available_version` (nullable). The tab component derives the pill count client-side from the existing `usePluginsInstalled()` query:

```ts
const updatesAvailable = plugins.filter(p => p.update_available_version).length;
```

No new endpoint. Query already runs.

### Manifest dialog endpoint

New endpoint `GET /plugins/installed/:scope/:id/manifest` reads `manifest.json` off disk and returns it as `application/json`. The full manifest is heavy and usually unread, so the list-item payload stays lean and the manifest is fetched on demand when the icon button is clicked. Reading directly off disk avoids stale-in-memory issues after a reload-from-disk.

### Open plugin folder

The icon button needs the plugin's absolute path on the server. Add a `folder_path` field to the plugin response (`GET /plugins/installed`). Backend already knows this: `filepath.Join(installer.PluginDir(), scope, id)`. Add to the `Plugin` struct's JSON serialization.

## 7. Frontend Implementation

### New components

- `PluginLogo` (`app/components/plugins/PluginLogo.tsx`) — the sized, square, radiused logo with broken-image fallback and initials generator
- `PluginRow` — the shared row layout used by both Installed and Discover tabs (polymorphic: accepts an `actions` render prop so Installed/Discover can inject their own button sets)
- `InstalledTab`, `DiscoverTab` — the per-tab lists; both use `PluginRow`
- `AdvancedPluginsDialog` — the dialog behind the gear icon; renders Order and Repositories sections (extract those from the existing `AdminPlugins.tsx`)
- `PluginDetailPage` (`app/components/pages/PluginDetail.tsx`) — the new route component
- `PluginConfigForm` — extracted from `PluginConfigDialog.tsx`, used by `PluginDetailPage`
- `PluginVersionCard` — single version card used inside the version history section
- `ChangelogMarkdown` — `react-markdown` + `rehype-sanitize` wrapper with plugin-appropriate styling; keeps the unsafe renderers off the main-app markdown surface
- `ManifestDialog` — shown by the "View manifest" icon button; fetches from the new endpoint, renders with a JSON syntax highlighter (already in the app; pick existing one)

### Deleted / retired

- `PluginConfigDialog.tsx` — replaced by `PluginDetailPage`'s config section. The `useDisclosure` + modal plumbing in `AdminPlugins.tsx` goes with it.
- The "Update available" page-level banner (if one exists in the current component) — row signal + tab pill replace it.

### Modified

- `AdminPlugins.tsx` — becomes a thin shell that renders either `InstalledTab` or `DiscoverTab` based on the route, plus the page head with the gear icon. Order + Repositories logic moves into `AdvancedPluginsDialog`.
- `app/hooks/queries/plugins.ts` — no new queries needed for the main redesign. May add one small query for the new manifest endpoint (`usePluginManifest(scope, id, { enabled })`).

### Router

Add a new route `/settings/plugins/:scope/:id` → `PluginDetailPage`. Keep the current `/settings/plugins/:tab?` → `AdminPlugins` (but now `:tab` only accepts `installed` | `discover`).

### New dependency

`react-markdown` + `rehype-sanitize`. Add to `package.json` `dependencies` (build-time — changelog is rendered at runtime in production bundles).

## 8. Docs

### `website/docs/plugins/development.md`

Add a **Logo** section under the manifest reference with:

- `imageUrl` field description and where it goes in the repository manifest (NOT the installed-plugin manifest)
- Recommendations: 128×128 minimum, 256×256 recommended; PNG or SVG (SVG preferred); 1:1 aspect; centered mark with ≥10% safe area (we apply 6px radius at 40px); any HTTPS URL (GitHub raw works)
- Rendered treatment (Shisho applies square crop + radius + muted backdrop for transparent logos)
- Fallback behavior (hashed-color initials)
- Example manifest snippet showing `imageUrl` placement

Add a **Release dates** section covering the `releaseDate` field on `PluginVersion` entries in the repo manifest:

- Optional field; when omitted, the UI hides the "Released" line on the version card
- Accepted formats: RFC3339 (`2026-04-14T00:00:00Z`) or date-only (`2026-04-14`)
- Example manifest snippet

### `website/docs/plugins/` (new or existing index)

Add a screenshot of the new detail page once the UI is built (this is a follow-up for the implementation plan, not the spec — noted so it's not forgotten).

## 9. Implementation Order (for the plan)

This is guidance for `writing-plans` — the actual plan will expand this:

1. Detail page route + scaffolding with hero (read-only) — verifiable in isolation
2. Extract `PluginConfigForm` from `PluginConfigDialog`; mount in detail page; delete the dialog
3. Version history + `ChangelogMarkdown`
4. Permissions + Danger zone
5. `PluginLogo` component + fallback; replace logo rendering everywhere
6. Row redesign (Installed + Discover share `PluginRow`)
7. Tab pill with update count + tooltip
8. Advanced dialog (Order + Repositories extract)
9. Icon actions: reload (local only), view manifest, open folder
10. Backend: `GET /plugins/installed/:scope/:id/manifest`, `folder_path` field
11. `releaseDate` parsing for both date-only and RFC3339
12. Docs updates
13. Cleanup: remove old banner, unused modal code, dead CSS

## 10. Testing

- **Backend unit tests:**
  - `releaseDate` parser accepts both formats, rejects garbage
  - Manifest endpoint returns file contents; 404 when plugin gone
  - `folder_path` field populated correctly for both local and installed plugins
- **Frontend unit tests (Vitest + Testing Library):**
  - `PluginLogo` renders `<img>` when `imageUrl` present; swaps to initials on error; initials generator produces deterministic output per `(scope, id)`
  - `PluginRow` renders with all capability combinations (0, 1, 3+ caps); disabled state applies dim class; Update badge + button show only when `update_available_version` set
  - Tab pill: no pill at 0, pill at > 0, tooltip text pluralizes
  - `ChangelogMarkdown` strips disallowed tags; renders headings + lists + code correctly
- **E2E (Playwright):**
  - Install plugin from Discover → appears in Installed
  - Click Update on Installed row → in-place version bump, pill decrements
  - Navigate to detail page → toggle enabled → row reflects state on back-nav
  - Uninstall from detail → confirm → returns to list, plugin gone
  - Local plugin shows reload button; repo-installed plugin does not

## 11. Risks & Open Questions

- **Markdown rendering surface area.** `react-markdown` + `rehype-sanitize` is safe by default, but plugin authors may put images or iframes in changelogs that get stripped. Document the allowlisted markdown subset in the logo/release-date docs section so authors don't waste effort on content that won't render.
- **`Open plugin folder` is clipboard-only.** Users on Docker especially won't have host access to the server's plugin dir, and the copied path will be the container-internal path (e.g. `/data/plugins/local/my-plugin`), not a host path. This is unavoidable from a browser app — we return what the server knows. Document this explicitly in the docs page so Docker users aren't confused when they paste the path into Finder and nothing happens.
- **Update-count pill staleness.** The pill is derived from `usePluginsInstalled()`. If a repository sync runs in the background and turns up new updates, the pill updates when the query refetches — so make sure the repository sync mutation invalidates `plugins.installed`. This is easy to miss; verify in implementation.
- **Detail-page fetch cost.** The detail page loads installed data + available-plugin data + config schema in parallel. Three round-trips on cold nav. Acceptable for a settings page but flag in the plan so it's not regressed later.
