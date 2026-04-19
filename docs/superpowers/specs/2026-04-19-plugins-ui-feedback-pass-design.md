# Plugins UI Feedback Pass (April 2026)

## Summary

Address seven pieces of feedback on the redesigned plugin UI. The changes
cluster into four layers: frontend polish, a backend serialization fix, a
small repository-schema extension to decouple plugin homepages from release
URLs, and documentation/data updates on the official plugin repository.

## Problems

1. **Padding:** Plugin rows look too tall — `p-4` (16px) is too much vertical
   space for the content density.
2. **No official marker:** Nothing in the UI distinguishes plugins from the
   official repository from those added by users.
3. **UTC date bug:** Release dates are off by one day — `"2026-04-12"` is
   parsed as midnight UTC, which renders as the previous day in US timezones.
4. **Coupled release URL:** `buildGitHubReleaseUrl` in `PluginVersionHistory`
   synthesizes release links by appending `/releases/tag/{tag}` to the
   plugin's `homepage`. This forces `homepage` to be a GitHub repo root,
   preventing per-plugin specific homepages. The uncommitted `tag`
   per-version field is a band-aid that doesn't fix the coupling.
5. **Missing logos:** The backend drops `imageUrl` from the
   `/plugins/available` response — the struct in `handler.go:699-709`
   doesn't include it. Every plugin falls back to hashed-color initials.
6. **Disorienting back button:** `PluginDetail`'s back button always
   navigates to `/settings/plugins` regardless of how the user arrived,
   which is confusing for plugins reached via deep link or the Discover tab.
7. **Generic per-plugin homepage:** All official plugins share the same
   `homepage` value (the repo root) because updating it would break release
   URL construction.

## Changes

### Layer 1 — Frontend polish (no schema changes)

**`app/components/plugins/PluginRow.tsx`**
- Change `p-4` → `py-3 px-4`. Keeps horizontal breathing room for the logo
  and text; tightens vertical density.

**`app/components/pages/PluginDetail.tsx`**
- Remove the `<ChevronLeft /> Plugins` back button block (lines 87–96).
- Replace with a breadcrumb trail: `Plugins / {plugin name}` where `Plugins`
  links to `/settings/plugins`. Follow the pattern in
  `app/components/library/LibraryBreadcrumbs.tsx` (same styling and
  truncation behavior). Rendered inline; no need to extract a shared
  component unless a second callsite appears.

**`app/components/plugins/PluginVersionCard.tsx`**
- Fix `formatReleaseDate` timezone bug. Detect date-only inputs via regex
  `^\d{4}-\d{2}-\d{2}$` and construct the Date from components:
  ```ts
  const m = raw.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  const d = m
    ? new Date(+m[1], +m[2] - 1, +m[3])
    : new Date(raw);
  ```
  RFC3339 inputs (with explicit timezone) continue through `new Date(raw)`
  unchanged. The relative-days calculation is unaffected.

**Official badge — `PluginRow.tsx` and `PluginDetailHero.tsx`**
- Render `<BadgeCheck />` from `lucide-react` inline with the author name
  when `isOfficial === true`.
- Color: `text-primary` (matches the "Latest" badge family).
- Size: `h-3.5 w-3.5` in PluginRow, `h-4 w-4` in PluginDetailHero.
- Add `isOfficial?: boolean` prop to PluginRow; source from the available
  plugin response.
- Placement: rendered inside the author span, between the `·` separator and
  the name: `· <BadgeCheck /> Shisho Team`. Use `inline-flex items-center
  gap-1` on the span to line up the glyph with the text baseline.

### Layer 2 — Backend response fields

**`pkg/plugins/handler.go`**

Add two fields to `availablePluginResponse`:
```go
type availablePluginResponse struct {
    Scope       string                   `json:"scope"`
    ID          string                   `json:"id"`
    Name        string                   `json:"name"`
    Overview    string                   `json:"overview"`
    Description string                   `json:"description"`
    Author      string                   `json:"author"`
    Homepage    string                   `json:"homepage"`
    ImageURL    string                   `json:"imageUrl"`     // NEW
    IsOfficial  bool                     `json:"is_official"`  // NEW
    Versions    []AnnotatedPluginVersion `json:"versions"`
    Compatible  bool                     `json:"compatible"`
}
```

In `listAvailable` (handler.go:747-757), populate both:
```go
ImageURL:   p.ImageURL,
IsOfficial: repo.IsOfficial,
```

`IsOfficial` is already on `plugin_repositories.is_official` and surfaced on
`Repository.IsOfficial`, so this is a pure pass-through. No new DB queries.

**`app/hooks/queries/plugins.ts`**

Update the `AvailablePlugin` TS interface to mirror:
```ts
export interface AvailablePlugin {
  scope: string;
  id: string;
  name: string;
  overview: string;
  description: string;
  author: string;
  homepage: string;
  imageUrl: string;       // already declared; backend now actually sends it
  is_official: boolean;   // NEW
  versions: PluginVersion[];
  compatible: boolean;
}
```

### Layer 3 — Repository schema: decouple release URL from homepage

**Remove the uncommitted `tag` field**

On this branch, `pkg/plugins/repository.go` and
`app/hooks/queries/plugins.ts` currently carry an uncommitted `Tag` field on
`PluginVersion`. Revert both:

- Go: remove the `Tag string \`json:"tag,omitempty"\`` field.
- TS: remove the `tag?: string` field.
- `PluginVersionHistory.tsx`: revert `buildGitHubReleaseUrl` to take a version
  — we're replacing it entirely in the next step.
- `website/docs/plugins/repositories.md`: revert the `tag` documentation
  added in 3be89b4.

**Add `releaseUrl` per version**

```go
type PluginVersion struct {
    // ... existing fields ...
    // ReleaseURL is an optional explicit URL for this version's release page
    // (e.g. GitHub release, GitLab tag). When unset the frontend does not
    // render a "View release" link.
    ReleaseURL   string        `json:"releaseUrl,omitempty"`
    Capabilities *Capabilities `json:"capabilities,omitempty"`
}
```

```ts
export interface PluginVersion {
  // ... existing fields ...
  releaseUrl?: string;
  capabilities?: PluginCapabilities;
}
```

**Rewire `PluginVersionHistory.tsx` and `PluginVersionCard.tsx`**

- Delete `buildGitHubReleaseUrl` entirely — no more URL synthesis.
- Rename `PluginVersionCard`'s `gitHubReleaseUrl?: string` prop to
  `releaseUrl?: string`. The card renders `version.releaseUrl` verbatim
  when present.
- `PluginVersionHistory` passes `releaseUrl={v.releaseUrl}` to each card
  (no more homepage lookup).

**Rename the link label.** Current label is "View release on GitHub" —
with `releaseUrl` being a generic URL (GitHub, GitLab, Codeberg, anywhere),
rename to **"View release"** in `PluginVersionCard.tsx`.

**Behavior matrix:**

| `version.releaseUrl` | Display |
|----------------------|---------|
| Present | "View release" link pointing at `version.releaseUrl` |
| Missing/empty | No link rendered |

This is fully backward-compatible: existing repos without `releaseUrl` just
lose the auto-synthesized link (which was already wrong for any repo whose
tags don't match `v{version}`, per feedback #4).

### Layer 4 — Documentation and out-of-scope data changes

**`website/docs/plugins/repositories.md`**
- Document the `releaseUrl` field on plugin versions.
- Remove the (already-uncommitted) `tag` documentation.
- Clarify in the `homepage` description that it should be the plugin's
  specific landing page, not necessarily the repo root.

**Out of scope — tracked on Notion**
- Official repo (`shishobooks/plugins`): update each plugin's `homepage` to
  `…/tree/master/plugins/{plugin-id}`, and add `releaseUrl` per version.
  This is a data change, not a code change, and requires a separate PR in
  the plugins repo.

## Test plan

### Frontend unit tests (Vitest)
- `formatReleaseDate` — new cases:
  - `"2026-04-14"` → formatted date is "Apr 14, 2026" (not "Apr 13") when
    running in any US timezone. Use `vi.setSystemTime` + `process.env.TZ`
    scoping if needed to make this deterministic; otherwise assert the
    parsed Date's local calendar day equals 14.
  - RFC3339 input unchanged.
- `PluginRow` — renders `BadgeCheck` icon when `isOfficial={true}`; omits
  when `false`/undefined.
- `PluginDetailHero` — same behavior.
- `PluginVersionCard` — renders "View release" link when `releaseUrl`
  present; omits when absent. Delete the existing tests for `gitHubReleaseUrl`
  synthesis (they're obsolete).
- `PluginDetail` — breadcrumb renders "Plugins" as a link and the plugin
  name as the current item.

### Backend unit tests (Go)
- `listAvailable` handler test: assert response JSON contains `imageUrl`
  and `is_official` fields; assert `is_official` matches the source
  repository's flag.

### E2E (Playwright)
- No new specs required. Existing `e2e/plugins.spec.ts` routing/redirect
  smoke still passes.

### Manual verification
- Open Discover tab → rows look less tall; official plugins show the
  BadgeCheck icon; all plugins show their logos (no more OL fallback for
  plugins that ship an `imageUrl`).
- Open a plugin detail page → breadcrumb shows `Plugins / {name}`; release
  dates match the calendar date in the changelog header.

## Rollout

Single PR. All changes are backward-compatible:
- `releaseUrl` is additive; plugins without it simply don't get a release
  link (matching the existing behavior when homepage wasn't a GitHub URL).
- `tag` removal is safe because the field was never committed or released.
- `imageUrl` and `is_official` are additive response fields; existing
  frontend code already handles `imageUrl` gracefully when absent.

## Open questions

None at design-review time. If the official repo's data update (new
`releaseUrl` values, per-plugin `homepage`) isn't coordinated before merge,
the symptom is identical to today's state: homepage links to the repo root
and no release link appears. Nothing new breaks.
