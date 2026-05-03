# Style Consistency Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> Check the project's root CLAUDE.md and any relevant subdirectory CLAUDE.md files for rules that apply to your work.

**Goal:** Standardize all frontend styling on patterns established by the Identify review form — semantic color tokens, consistent spacing, uniform selection states, and matching border radii.

**Architecture:** Pure CSS class replacements across ~30 files. No behavioral changes, no new components. Every change swaps hardcoded/inconsistent Tailwind classes for the project's semantic design tokens.

**Tech Stack:** React, TailwindCSS, shadcn/ui

**Reference patterns:**
- Page titles: `text-2xl font-semibold`
- Dialog titles: `text-sm font-semibold`
- Selection: `border-primary bg-primary/5` (cards), `border-primary bg-primary/5 text-primary` (toggle chips)
- Inline warnings: `rounded-md bg-destructive/10 border border-destructive/20 p-3`
- Page danger zones: Match `PluginDangerZone.tsx` — `space-y-3 rounded-md border border-destructive/40 p-4 md:p-6` with title `text-lg font-semibold text-destructive`
- Dialog body spacing: `space-y-6`
- Card/section padding: `p-4 md:p-6`
- Page header margin: `mb-6 md:mb-8`
- Border radius (page components): `rounded-md`
- Hover backgrounds: `hover:bg-muted/50`

---

### Task 1: Hardcoded colors — Layout, Navigation, Logo, Spinner

These files use `dark:bg-neutral-*`, `dark:text-violet-*`, or `text-gray-*` instead of semantic tokens. The dark mode primary CSS variable (`oklch(0.8 0.15 280)`) already produces the same violet shade, making these overrides redundant.

**Files:**
- Modify: `app/components/layout/topNavClasses.ts:4`
- Modify: `app/components/layout/Sidebar.tsx:35,76`
- Modify: `app/components/library/MobileDrawer.tsx:38,157,228,257,269`
- Modify: `app/components/library/Logo.tsx:7,68`
- Modify: `app/components/library/LoadingSpinner.tsx:6`

- [ ] **Step 1: Fix topNavClasses.ts**

Line 4 — remove `dark:bg-neutral-900`:
```ts
// Before
"sticky top-0 z-30 border-b border-border bg-background dark:bg-neutral-900"
// After
"sticky top-0 z-30 border-b border-border bg-background"
```

- [ ] **Step 2: Fix Sidebar.tsx**

Line 35 — NavItem active state, remove violet overrides:
```tsx
// Before
? "bg-primary/10 text-primary dark:bg-violet-500/20 dark:text-violet-300"
// After
? "bg-primary/10 text-primary"
```

Line 76 — sidebar container, remove dark neutral override:
```tsx
// Before
"sticky top-14 flex h-[calc(100vh-3.5rem)] shrink-0 flex-col border-r border-border bg-muted/30 transition-all duration-200 md:top-16 md:h-[calc(100vh-4rem)] dark:bg-neutral-900/50",
// After
"sticky top-14 flex h-[calc(100vh-3.5rem)] shrink-0 flex-col border-r border-border bg-muted/30 transition-all duration-200 md:top-16 md:h-[calc(100vh-4rem)]",
```

- [ ] **Step 3: Fix MobileDrawer.tsx**

Line 38 — nav item active (same pattern as Sidebar):
```tsx
// Before
? "bg-primary/10 text-primary dark:bg-violet-500/20 dark:text-violet-300"
// After
? "bg-primary/10 text-primary"
```

Line 157 — drawer container background:
```tsx
// Before
dark:bg-neutral-900
// After
(remove, keep bg-background or bg-muted/30 only — check the full class string)
```

Lines 228, 257 — nav active states (same replacement as line 38).

Line 269 — count text:
```tsx
// Before
? "text-primary/70 dark:text-violet-300/70"
// After
? "text-primary/70"
```

- [ ] **Step 4: Fix Logo.tsx**

Line 7:
```tsx
// Before
className={cn("text-primary dark:text-violet-300", className)}
// After
className={cn("text-primary", className)}
```

Line 68:
```tsx
// Before
"align-super font-normal text-primary dark:text-violet-300 ml-0.5",
// After
"align-super font-normal text-primary ml-0.5",
```

- [ ] **Step 5: Fix LoadingSpinner.tsx**

Line 6:
```tsx
// Before
className="w-8 h-8 text-gray-200 animate-spin dark:text-gray-600 fill-primary dark:fill-violet-300"
// After
className="w-8 h-8 text-muted-foreground/20 animate-spin fill-primary"
```

- [ ] **Step 6: Commit**

```bash
git add app/components/layout/topNavClasses.ts app/components/layout/Sidebar.tsx app/components/library/MobileDrawer.tsx app/components/library/Logo.tsx app/components/library/LoadingSpinner.tsx
git commit -m "[Frontend] Replace hardcoded colors with semantic tokens in layout and navigation"
```

---

### Task 2: Hardcoded colors — GlobalSearch, LibraryListPicker

**Files:**
- Modify: `app/components/library/GlobalSearch.tsx:85,330,331,376,377,418,419`
- Modify: `app/components/library/LibraryListPicker.tsx:80,92,103,131,143,156,162`

- [ ] **Step 1: Fix GlobalSearch.tsx**

Line 85 — keyboard shortcut badge:
```tsx
// Before
bg-neutral-200 dark:bg-neutral-700
// After
bg-muted
```

Lines 330-331, 376-377, 418-419 — search result group backgrounds (3 pairs):
```tsx
// Before
bg-neutral-100 dark:bg-neutral-800
// After
bg-muted
```

- [ ] **Step 2: Fix LibraryListPicker.tsx**

Line 80 — library row active:
```tsx
// Before
? "bg-primary/10 text-primary dark:bg-primary/15 dark:text-violet-300"
// After
? "bg-primary/10 text-primary"
```

Line 92 — library icon active:
```tsx
// Before
? "bg-primary text-primary-foreground dark:bg-violet-400 dark:text-neutral-900"
// After
? "bg-primary text-primary-foreground"
```

Line 103 — check icon:
```tsx
// Before
<Check className="h-4 w-4 shrink-0 text-primary dark:text-violet-300" />
// After
<Check className="h-4 w-4 shrink-0 text-primary" />
```

Line 131 — list row active (same as line 80 pattern):
```tsx
// Before
? "bg-primary/10 text-primary dark:bg-primary/15 dark:text-violet-300"
// After
? "bg-primary/10 text-primary"
```

Line 143 — list icon active (same as line 92 pattern):
```tsx
// Before
? "bg-primary text-primary-foreground dark:bg-violet-400 dark:text-neutral-900"
// After
? "bg-primary text-primary-foreground"
```

Line 156 — list count text:
```tsx
// Before
${isActive ? "text-primary/70 dark:text-violet-300/70" : "text-muted-foreground"}
// After
${isActive ? "text-primary/70" : "text-muted-foreground"}
```

Line 162 — check icon (same as line 103):
```tsx
// Before
<Check className="h-4 w-4 shrink-0 text-primary dark:text-violet-300" />
// After
<Check className="h-4 w-4 shrink-0 text-primary" />
```

- [ ] **Step 3: Commit**

```bash
git add app/components/library/GlobalSearch.tsx app/components/library/LibraryListPicker.tsx
git commit -m "[Frontend] Replace hardcoded colors with semantic tokens in search and picker"
```

---

### Task 3: Hardcoded colors — Entity list pages & detail pages

**Files:**
- Modify: `app/components/pages/GenresList.tsx:78`
- Modify: `app/components/pages/TagsList.tsx:78`
- Modify: `app/components/pages/ImprintsList.tsx:76`
- Modify: `app/components/pages/PublishersList.tsx:76`
- Modify: `app/components/pages/PersonList.tsx:69`
- Modify: `app/components/pages/SeriesList.tsx:94,104,128`
- Modify: `app/components/pages/ListsIndex.tsx:69,96`
- Modify: `app/components/pages/PersonDetail.tsx:283`
- Modify: `app/components/pages/ImprintDetail.tsx:172`
- Modify: `app/components/pages/PublisherDetail.tsx:172`
- Modify: `app/components/pages/BookDetail.tsx:1246`
- Modify: `app/components/library/BookItem.tsx:262,273,357`

- [ ] **Step 1: Fix entity list page hover backgrounds**

All follow the same pattern — replace hardcoded hover with semantic token:

GenresList.tsx:78, TagsList.tsx:78, ImprintsList.tsx:76, PublishersList.tsx:76:
```tsx
// Before
hover:bg-neutral-100 dark:hover:bg-neutral-800
// After
hover:bg-muted/50
```

PersonList.tsx:69, ListsIndex.tsx:69,96, PersonDetail.tsx:283:
```tsx
// Before
hover:bg-neutral-50 dark:hover:bg-neutral-800
// After
hover:bg-muted/50
```

- [ ] **Step 2: Fix entity list page borders**

PersonList.tsx:69, PersonDetail.tsx:283:
```tsx
// Before
border-neutral-200 dark:border-neutral-700
// After
border-border
```

SeriesList.tsx:94,104:
```tsx
// Before
border-neutral-300 dark:border-neutral-600
// After
border-border
```

BookItem.tsx:262,273:
```tsx
// Before
border-neutral-300 dark:border-neutral-600
// After
border-border
```

- [ ] **Step 3: Fix hardcoded text colors**

SeriesList.tsx:128:
```tsx
// Before
text-neutral-500 dark:text-neutral-500
// After
text-muted-foreground
```

BookItem.tsx:357:
```tsx
// Before
text-neutral-500
// After
text-muted-foreground
```

- [ ] **Step 4: Fix violet overrides in detail pages**

ImprintDetail.tsx:172 and PublisherDetail.tsx:172:
```tsx
// Before
className="border-l-4 border-l-primary dark:border-l-violet-300 pl-4 py-2"
// After
className="border-l-4 border-l-primary pl-4 py-2"
```

BookDetail.tsx:1246 — author link:
```tsx
// Before
className="text-sm font-medium text-primary hover:text-primary/80 hover:underline dark:text-violet-300 dark:hover:text-violet-400"
// After
className="text-sm font-medium text-primary hover:text-primary/80 hover:underline"
```

- [ ] **Step 5: Commit**

```bash
git add app/components/pages/GenresList.tsx app/components/pages/TagsList.tsx app/components/pages/ImprintsList.tsx app/components/pages/PublishersList.tsx app/components/pages/PersonList.tsx app/components/pages/SeriesList.tsx app/components/pages/ListsIndex.tsx app/components/pages/PersonDetail.tsx app/components/pages/ImprintDetail.tsx app/components/pages/PublisherDetail.tsx app/components/pages/BookDetail.tsx app/components/library/BookItem.tsx
git commit -m "[Frontend] Replace hardcoded colors with semantic tokens in list and detail pages"
```

---

### Task 4: Hardcoded colors — Job & log components

**Files:**
- Modify: `app/components/pages/AdminJobs.tsx:29`
- Modify: `app/components/pages/JobDetail.tsx:38,53`
- Modify: `app/components/library/LogViewer.tsx:21,177`

- [ ] **Step 1: Fix AdminJobs.tsx**

Line 29 — job status badge "queued"/"cancelled" color (gray):
```tsx
// Before
bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400
// After
bg-muted text-muted-foreground
```

- [ ] **Step 2: Fix JobDetail.tsx**

Line 38 — log level "trace" color:
```tsx
// Before
bg-gray-500/20 text-gray-400
// After
bg-muted text-muted-foreground
```

Line 53 — log level "info" badge:
```tsx
// Before
bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400
// After
bg-muted text-muted-foreground
```

- [ ] **Step 3: Fix LogViewer.tsx**

Line 21 — log level TRACE:
```tsx
// Before
bg-gray-100 text-gray-700 dark:bg-gray-800/50 dark:text-gray-400
// After
bg-muted text-muted-foreground
```

Line 177 — log viewer container dark bg:
```tsx
// Before
dark:bg-neutral-950/50
// After
(remove the dark override, keep only the base background class)
```

- [ ] **Step 4: Commit**

```bash
git add app/components/pages/AdminJobs.tsx app/components/pages/JobDetail.tsx app/components/library/LogViewer.tsx
git commit -m "[Frontend] Replace hardcoded gray colors with semantic tokens in job and log components"
```

---

### Task 5: Page titles — Standardize to `text-2xl font-semibold`

Three groups need changing:
1. Entity detail pages: `text-3xl font-bold` → `text-2xl font-semibold`
2. Admin pages: `text-xl md:text-2xl font-semibold` → `text-2xl font-semibold`
3. Book/File/List detail: `text-2xl md:text-3xl font-semibold/font-bold` → `text-2xl font-semibold`

**Files:**
- Modify: `app/components/pages/GenreDetail.tsx:176`
- Modify: `app/components/pages/ImprintDetail.tsx:126`
- Modify: `app/components/pages/PersonDetail.tsx:188`
- Modify: `app/components/pages/PublisherDetail.tsx:126`
- Modify: `app/components/pages/SeriesDetail.tsx:179`
- Modify: `app/components/pages/TagDetail.tsx:176`
- Modify: `app/components/pages/AdminCache.tsx:67`
- Modify: `app/components/pages/AdminJobs.tsx:138`
- Modify: `app/components/pages/AdminLibraries.tsx:99`
- Modify: `app/components/pages/AdminLogs.tsx:58`
- Modify: `app/components/pages/AdminUsers.tsx:133`
- Modify: `app/components/pages/AdminSettings.tsx:72`
- Modify: `app/components/pages/ListsIndex.tsx:116`
- Modify: `app/components/pages/BookDetail.tsx:1101`
- Modify: `app/components/pages/FileDetail.tsx:191`
- Modify: `app/components/pages/ListDetail.tsx:218`

- [ ] **Step 1: Fix entity detail page titles**

GenreDetail.tsx:176, ImprintDetail.tsx:126, PersonDetail.tsx:188, PublisherDetail.tsx:126, SeriesDetail.tsx:179, TagDetail.tsx:176:
```tsx
// Before
<h1 className="text-3xl font-bold min-w-0 break-words">
// After
<h1 className="text-2xl font-semibold min-w-0 break-words">
```

- [ ] **Step 2: Fix admin page titles**

AdminCache.tsx:67, AdminJobs.tsx:138, AdminLibraries.tsx:99, AdminLogs.tsx:58, AdminUsers.tsx:133, AdminSettings.tsx:72, ListsIndex.tsx:116:
```tsx
// Before
<h1 className="text-xl md:text-2xl font-semibold mb-1 md:mb-2">
// After
<h1 className="text-2xl font-semibold mb-1 md:mb-2">
```

- [ ] **Step 3: Fix book/file/list detail titles**

BookDetail.tsx:1101, FileDetail.tsx:191:
```tsx
// Before
<h1 className="text-2xl md:text-3xl font-semibold">
// After
<h1 className="text-2xl font-semibold">
```

ListDetail.tsx:218:
```tsx
// Before
<h1 className="text-2xl md:text-3xl font-bold min-w-0 break-words">
// After
<h1 className="text-2xl font-semibold min-w-0 break-words">
```

- [ ] **Step 4: Commit**

```bash
git add app/components/pages/GenreDetail.tsx app/components/pages/ImprintDetail.tsx app/components/pages/PersonDetail.tsx app/components/pages/PublisherDetail.tsx app/components/pages/SeriesDetail.tsx app/components/pages/TagDetail.tsx app/components/pages/AdminCache.tsx app/components/pages/AdminJobs.tsx app/components/pages/AdminLibraries.tsx app/components/pages/AdminLogs.tsx app/components/pages/AdminUsers.tsx app/components/pages/AdminSettings.tsx app/components/pages/ListsIndex.tsx app/components/pages/BookDetail.tsx app/components/pages/FileDetail.tsx app/components/pages/ListDetail.tsx
git commit -m "[Frontend] Standardize all page titles to text-2xl font-semibold"
```

---

### Task 6: Selection states — Standardize to `border-primary bg-primary/5`

Four components need updating. Checkboxes (BookItem, BookDetail) and image-ring selections (PagePicker) are intentionally different patterns and should NOT change.

**Files:**
- Modify: `app/components/library/BookSelectionList.tsx:119-122`
- Modify: `app/components/library/MergeBooksDialog.tsx:190-193`
- Modify: `app/components/library/FilterSheet.tsx:276-281`
- Modify: `app/components/library/CoverGalleryTabs.tsx:172-177`

- [ ] **Step 1: Fix BookSelectionList.tsx**

Lines 119-122 — add border to base, use `border-primary bg-primary/5` when selected:
```tsx
// Before
className={cn(
  "flex items-start gap-3 p-2 rounded-md cursor-pointer transition-colors overflow-hidden",
  isSelected ? "bg-primary/10" : "hover:bg-muted/50",
)}
// After
className={cn(
  "flex items-start gap-3 p-2 rounded-md border cursor-pointer transition-colors overflow-hidden",
  isSelected ? "border-primary bg-primary/5" : "border-transparent hover:bg-muted/50",
)}
```

- [ ] **Step 2: Fix MergeBooksDialog.tsx**

Lines 190-193 — same pattern as BookSelectionList:
```tsx
// Before
className={cn(
  "flex items-start gap-3 p-2 rounded-md cursor-pointer transition-colors",
  isSelected ? "bg-primary/10" : "hover:bg-muted/50",
)}
// After
className={cn(
  "flex items-start gap-3 p-2 rounded-md border cursor-pointer transition-colors",
  isSelected ? "border-primary bg-primary/5" : "border-transparent hover:bg-muted/50",
)}
```

- [ ] **Step 3: Fix FilterSheet.tsx**

Lines 276-281 — change from solid fill to subtle highlight:
```tsx
// Before
isSelected
  ? "border-primary bg-primary text-primary-foreground"
  : "border-border bg-card hover:bg-accent",
// After
isSelected
  ? "border-primary bg-primary/5 text-primary"
  : "border-border bg-card hover:bg-accent",
```

- [ ] **Step 4: Fix CoverGalleryTabs.tsx**

Lines 172-177 — change pill tabs from solid fill to subtle highlight:
```tsx
// Before
file.id === selectedFileId
  ? "bg-primary border-primary text-primary-foreground"
  : "border-border text-muted-foreground hover:bg-accent hover:text-foreground",
// After
file.id === selectedFileId
  ? "border-primary bg-primary/5 text-primary"
  : "border-border text-muted-foreground hover:bg-accent hover:text-foreground",
```

- [ ] **Step 5: Commit**

```bash
git add app/components/library/BookSelectionList.tsx app/components/library/MergeBooksDialog.tsx app/components/library/FilterSheet.tsx app/components/library/CoverGalleryTabs.tsx
git commit -m "[Frontend] Standardize card selection states to border-primary bg-primary/5"
```

---

### Task 7: Border radius + Warning cards + Danger zones

Fix `rounded-lg` → `rounded-md` in page components, standardize inline warning cards to `rounded-md bg-destructive/10 border border-destructive/20 p-3`, and align LibrarySettings danger zone to PluginDangerZone.

**Files (border radius):**
- Modify: `app/components/library/MoveFilesDialog.tsx:100,110`
- Modify: `app/components/library/MergeBooksDialog.tsx:155,257`
- Modify: `app/components/library/MergeIntoDialog.tsx:92`
- Modify: `app/components/library/SelectionToolbar.tsx:271`
- Modify: `app/components/library/BulkDownloadToast.tsx:19`
- Modify: `app/components/library/GlobalSearch.tsx:481`
- Modify: `app/components/library/RescanDialog.tsx:85`
- Modify: `app/components/pages/BookDetail.tsx:1571`
- Modify: `app/components/pages/ListsIndex.tsx:69`

**Files (warning cards):**
- Modify: `app/components/files/FetchChaptersDialog.tsx:136,222,290`
- Modify: `app/components/library/DeleteConfirmationDialog.tsx:174`
- Modify: `app/components/library/DeleteLibraryDialog.tsx:68`

**Files (danger zone):**
- Modify: `app/components/pages/LibrarySettings.tsx:425-428`

- [ ] **Step 1: Fix rounded-lg → rounded-md**

In each file, find `rounded-lg` and replace with `rounded-md`. These are in card/container elements, not the dialog/sheet primitives:

MoveFilesDialog.tsx:100 — `rounded-lg` → `rounded-md`
MoveFilesDialog.tsx:110 — `rounded-lg` → `rounded-md`
MergeBooksDialog.tsx:155 — `rounded-lg` → `rounded-md`
MergeBooksDialog.tsx:257 — `rounded-lg` → `rounded-md`
MergeIntoDialog.tsx:92 — `rounded-lg` → `rounded-md`
SelectionToolbar.tsx:271 — `rounded-lg` → `rounded-md`
BulkDownloadToast.tsx:19 — `rounded-lg` → `rounded-md`
GlobalSearch.tsx:481 — `rounded-lg` → `rounded-md`
RescanDialog.tsx:85 — `rounded-lg` → `rounded-md`
BookDetail.tsx:1571 — `rounded-lg` → `rounded-md`
ListsIndex.tsx:69 — `rounded-lg` → `rounded-md`

- [ ] **Step 2: Standardize inline warning cards**

FetchChaptersDialog.tsx lines 136, 222, 290 — border opacity `/50` → `/20`, padding `px-4 py-3` → `p-3`:
```tsx
// Before
className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3"
// After
className="rounded-md border border-destructive/20 bg-destructive/10 p-3"
```

DeleteConfirmationDialog.tsx:174 — already `rounded-md` and `p-3`, just fix border opacity (already `/20`, verify):
```tsx
// Verify it matches: rounded-md bg-destructive/10 border border-destructive/20 p-3
```

DeleteLibraryDialog.tsx:68 — already `rounded-md` and `p-3`, verify border opacity (already `/20`, verify):
```tsx
// Verify it matches: rounded-md bg-destructive/10 border border-destructive/20 p-3
```

- [ ] **Step 3: Fix LibrarySettings danger zone to match PluginDangerZone**

LibrarySettings.tsx:425-428:
```tsx
// Before
<section className="max-w-2xl mt-8 border border-destructive/40 rounded-md p-4 md:p-6">
  <h2 className="text-base md:text-lg font-semibold text-destructive mb-1">
    Danger Zone
  </h2>
// After
<section className="max-w-2xl mt-8 space-y-3 rounded-md border border-destructive/40 p-4 md:p-6">
  <h2 className="text-lg font-semibold text-destructive">
    Danger zone
  </h2>
```

Key changes: add `space-y-3`, remove responsive `text-base md:text-lg` → flat `text-lg`, remove `mb-1` (spacing handled by `space-y-3`), lowercase "zone".

- [ ] **Step 4: Commit**

```bash
git add app/components/library/MoveFilesDialog.tsx app/components/library/MergeBooksDialog.tsx app/components/library/MergeIntoDialog.tsx app/components/library/SelectionToolbar.tsx app/components/library/BulkDownloadToast.tsx app/components/library/GlobalSearch.tsx app/components/library/RescanDialog.tsx app/components/pages/BookDetail.tsx app/components/pages/ListsIndex.tsx app/components/files/FetchChaptersDialog.tsx app/components/library/DeleteConfirmationDialog.tsx app/components/library/DeleteLibraryDialog.tsx app/components/pages/LibrarySettings.tsx
git commit -m "[Frontend] Standardize border radius, warning cards, and danger zone styling"
```

---

### Task 8: Spacing — Dialog bodies, card padding, page header margins

**Files (DialogBody space-y-4 → space-y-6):**
- Modify: `app/components/plugins/CapabilitiesWarning.tsx:53`
- Modify: `app/components/library/MoveToPositionDialog.tsx:71`
- Modify: `app/components/library/MetadataEditDialog.tsx:160`
- Modify: `app/components/library/MoveFilesDialog.tsx:98`
- Modify: `app/components/library/CreateListDialog.tsx:146`
- Modify: `app/components/library/IdentifyBookDialog.tsx:199`
- Modify: `app/components/library/DeleteConfirmationDialog.tsx:172`
- Modify: `app/components/library/AddToListDialog.tsx:191`
- Modify: `app/components/library/DeleteLibraryDialog.tsx:67`
- Modify: `app/components/library/MergeIntoDialog.tsx:91`
- Modify: `app/components/library/MergeBooksDialog.tsx:171,255`
- Modify: `app/components/pages/SecuritySettings.tsx:469,738`
- Modify: `app/components/pages/UserDetail.tsx:452`

**Files (flat p-6 → p-4 md:p-6):**
- Modify: `app/components/pages/LibrarySettings.tsx:257`
- Modify: `app/components/pages/CreateLibrary.tsx:175`
- Modify: `app/components/pages/UserSettings.tsx:83`
- Modify: `app/components/pages/UserDetail.tsx:283`
- Modify: `app/components/pages/CreateUser.tsx:157`

**Files (flat mb-8 → mb-6 md:mb-8):**
- Modify: `app/components/pages/LibrarySettings.tsx:250`
- Modify: `app/components/pages/CreateLibrary.tsx:168`
- Modify: `app/components/pages/CreateUser.tsx:152`
- Modify: `app/components/pages/UserSettings.tsx:74`
- Modify: `app/components/pages/SecuritySettings.tsx:167`
- Modify: `app/components/pages/UserDetail.tsx:270`
- Modify: `app/components/pages/SeriesDetail.tsx:177`
- Modify: `app/components/pages/ImprintDetail.tsx:124`
- Modify: `app/components/pages/GenreDetail.tsx:174`
- Modify: `app/components/pages/TagDetail.tsx:174`
- Modify: `app/components/pages/PublisherDetail.tsx:124`
- Modify: `app/components/pages/PersonDetail.tsx:186`
- Modify: `app/components/pages/Login.tsx:72`
- Modify: `app/components/pages/Setup.tsx:162`

- [ ] **Step 1: Fix all DialogBody space-y-4 → space-y-6**

In every file listed above, replace `space-y-4` with `space-y-6` on the `<DialogBody>` className. Where the className has additional classes (e.g., `"space-y-4 min-w-0"`), only replace `space-y-4` → `space-y-6`.

- [ ] **Step 2: Fix flat p-6 → p-4 md:p-6**

LibrarySettings.tsx:257:
```tsx
// Before: "max-w-2xl space-y-6 border border-border rounded-md p-6"
// After:  "max-w-2xl space-y-6 border border-border rounded-md p-4 md:p-6"
```

CreateLibrary.tsx:175:
```tsx
// Before: "max-w-2xl space-y-6 border border-border rounded-md p-6"
// After:  "max-w-2xl space-y-6 border border-border rounded-md p-4 md:p-6"
```

UserSettings.tsx:83:
```tsx
// Before: "border border-border rounded-md p-6"
// After:  "border border-border rounded-md p-4 md:p-6"
```

UserDetail.tsx:283:
```tsx
// Before: "max-w-2xl space-y-6 border border-border rounded-md p-6"
// After:  "max-w-2xl space-y-6 border border-border rounded-md p-4 md:p-6"
```

CreateUser.tsx:157:
```tsx
// Before: "max-w-2xl space-y-6 border border-border rounded-md p-6"
// After:  "max-w-2xl space-y-6 border border-border rounded-md p-4 md:p-6"
```

- [ ] **Step 3: Fix flat mb-8 → mb-6 md:mb-8**

In every file listed above, find the page header `<div>` with `mb-8` and replace with `mb-6 md:mb-8`. Be careful to preserve other classes on the same element (e.g., `"flex flex-col items-center mb-8"` → `"flex flex-col items-center mb-6 md:mb-8"`).

- [ ] **Step 4: Commit**

```bash
git add app/components/plugins/CapabilitiesWarning.tsx app/components/library/MoveToPositionDialog.tsx app/components/library/MetadataEditDialog.tsx app/components/library/MoveFilesDialog.tsx app/components/library/CreateListDialog.tsx app/components/library/IdentifyBookDialog.tsx app/components/library/DeleteConfirmationDialog.tsx app/components/library/AddToListDialog.tsx app/components/library/DeleteLibraryDialog.tsx app/components/library/MergeIntoDialog.tsx app/components/library/MergeBooksDialog.tsx app/components/pages/SecuritySettings.tsx app/components/pages/UserDetail.tsx app/components/pages/LibrarySettings.tsx app/components/pages/CreateLibrary.tsx app/components/pages/UserSettings.tsx app/components/pages/CreateUser.tsx app/components/pages/SeriesDetail.tsx app/components/pages/ImprintDetail.tsx app/components/pages/GenreDetail.tsx app/components/pages/TagDetail.tsx app/components/pages/PublisherDetail.tsx app/components/pages/PersonDetail.tsx app/components/pages/Login.tsx app/components/pages/Setup.tsx
git commit -m "[Frontend] Standardize dialog body, card padding, and page header spacing"
```

---

### Task 9: Gallery select button + CLAUDE.md docs

**Files:**
- Modify: `app/components/pages/Home.tsx:608-617`
- Modify: `app/CLAUDE.md`

- [ ] **Step 1: Update gallery select button to size="sm"**

Home.tsx lines 608-617 — add `size="sm"` to both the Select and Cancel buttons:
```tsx
// Before
<Button onClick={exitSelectionMode} variant="outline">
  Cancel
</Button>
// After
<Button onClick={exitSelectionMode} size="sm" variant="outline">
  Cancel
</Button>

// Before
<Button onClick={enterSelectionMode} variant="outline">
  <CheckSquare className="h-4 w-4" />
  Select
</Button>
// After
<Button onClick={enterSelectionMode} size="sm" variant="outline">
  <CheckSquare className="h-4 w-4" />
  Select
</Button>
```

Keep the button in its current position (right side of toolbar).

- [ ] **Step 2: Update app/CLAUDE.md patterns**

Update the "Detail Page Patterns" section header example:
```tsx
// Before
<h1 className="text-3xl font-bold min-w-0 break-words">{name}</h1>
// After
<h1 className="text-2xl font-semibold min-w-0 break-words">{name}</h1>
```

Update the "Admin Page Headers" section example:
```tsx
// Before
<h1 className="text-xl md:text-2xl font-semibold mb-1 md:mb-2">
// After
<h1 className="text-2xl font-semibold mb-1 md:mb-2">
```

- [ ] **Step 3: Commit**

```bash
git add app/components/pages/Home.tsx app/CLAUDE.md
git commit -m "[Frontend] Update gallery select button size and sync CLAUDE.md patterns"
```

---

### Task 10: Verify

- [ ] **Step 1: Run JS lint to verify no syntax/type issues**

```bash
mise lint:js
```

Expected: All checks pass.

- [ ] **Step 2: Run unit tests**

```bash
mise test:unit
```

Expected: All tests pass.

- [ ] **Step 3: Visual spot-check in browser**

Start the dev server with `mise start` and verify:
- Sidebar, top nav, and mobile drawer look correct in both light and dark mode
- Entity detail page titles are visually consistent across Genre, Series, Person, etc.
- Admin page titles are consistent
- Book detail, file detail, list detail titles match
- Filter, sort, size, and select buttons on the gallery page are the same size
- Filter chips in FilterSheet use subtle highlight instead of solid fill
- Cover gallery tabs use subtle highlight
- BookSelectionList and MergeBooksDialog selection states show border + light background
- Warning cards in dialogs have consistent rounded corners and border opacity
- LibrarySettings danger zone matches PluginDangerZone
- Dialog body spacing is consistent
- Card sections have responsive padding
- GlobalSearch dropdown uses semantic background colors
- LoadingSpinner looks correct in both themes
