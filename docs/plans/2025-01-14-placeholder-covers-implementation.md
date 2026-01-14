# Placeholder Cover Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace text-based "no cover" fallbacks with visually appealing SVG placeholder images for books and audiobooks.

**Architecture:** Create SVG placeholder assets with inline rendering via a React component. Use CSS-based theme switching (hidden/block with dark: prefix) to avoid JavaScript theme detection. Integrate into all cover display components using state-based conditional rendering.

**Tech Stack:** React, TypeScript, TailwindCSS, inline SVG

---

## Task 1: Create SVG Assets Directory

**Files:**
- Create: `app/assets/placeholders/` directory

**Step 1: Create the directory**

```bash
mkdir -p app/assets/placeholders
```

**Step 2: Verify directory exists**

```bash
ls -la app/assets/
```
Expected: Shows `placeholders/` directory

**Step 3: Commit**

```bash
git add app/assets/placeholders/.gitkeep 2>/dev/null || echo "Empty dir - will commit with files"
```

Note: Git doesn't track empty directories. The commit will happen with Task 2.

---

## Task 2: Create Book Placeholder SVGs

**Files:**
- Create: `app/assets/placeholders/placeholder-book-light.svg`
- Create: `app/assets/placeholders/placeholder-book-dark.svg`

**Step 1: Create light mode book placeholder**

Create `app/assets/placeholders/placeholder-book-light.svg`:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 300">
  <defs>
    <linearGradient id="bg-light" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" style="stop-color: oklch(0.95 0.03 280)"/>
      <stop offset="100%" style="stop-color: oklch(0.90 0.05 280)"/>
    </linearGradient>
  </defs>
  <rect width="200" height="300" fill="url(#bg-light)"/>
  <g transform="translate(70, 120)" stroke="oklch(0.65 0.08 280)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none">
    <!-- Book icon: closed book with spine -->
    <path d="M0 0 L0 60 L50 60 L50 5 L45 0 Z"/>
    <path d="M0 0 L45 0"/>
    <path d="M45 0 L45 5 L50 5"/>
    <path d="M5 0 L5 60"/>
  </g>
</svg>
```

**Step 2: Create dark mode book placeholder**

Create `app/assets/placeholders/placeholder-book-dark.svg`:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 300">
  <defs>
    <linearGradient id="bg-dark" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" style="stop-color: oklch(0.25 0.05 280)"/>
      <stop offset="100%" style="stop-color: oklch(0.20 0.03 280)"/>
    </linearGradient>
  </defs>
  <rect width="200" height="300" fill="url(#bg-dark)"/>
  <g transform="translate(70, 120)" stroke="oklch(0.55 0.08 280)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none">
    <!-- Book icon: closed book with spine -->
    <path d="M0 0 L0 60 L50 60 L50 5 L45 0 Z"/>
    <path d="M0 0 L45 0"/>
    <path d="M45 0 L45 5 L50 5"/>
    <path d="M5 0 L5 60"/>
  </g>
</svg>
```

**Step 3: Verify SVGs render correctly**

Open each SVG in a browser to verify:
- Light: Pale lavender gradient with dark purple book icon
- Dark: Dark muted purple gradient with lighter book icon

**Step 4: Commit**

```bash
git add app/assets/placeholders/placeholder-book-light.svg app/assets/placeholders/placeholder-book-dark.svg
git commit -m "[Assets] Add book placeholder SVGs for light and dark modes"
```

---

## Task 3: Create Audiobook Placeholder SVGs

**Files:**
- Create: `app/assets/placeholders/placeholder-audiobook-light.svg`
- Create: `app/assets/placeholders/placeholder-audiobook-dark.svg`

**Step 1: Create light mode audiobook placeholder**

Create `app/assets/placeholders/placeholder-audiobook-light.svg`:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 300">
  <defs>
    <linearGradient id="bg-light" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" style="stop-color: oklch(0.95 0.03 280)"/>
      <stop offset="100%" style="stop-color: oklch(0.90 0.05 280)"/>
    </linearGradient>
  </defs>
  <rect width="300" height="300" fill="url(#bg-light)"/>
  <g transform="translate(120, 120)" stroke="oklch(0.65 0.08 280)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none">
    <!-- Headphones icon -->
    <path d="M0 35 L0 25 C0 10 13 0 30 0 C47 0 60 10 60 25 L60 35"/>
    <rect x="0" y="35" width="12" height="20" rx="2"/>
    <rect x="48" y="35" width="12" height="20" rx="2"/>
  </g>
</svg>
```

**Step 2: Create dark mode audiobook placeholder**

Create `app/assets/placeholders/placeholder-audiobook-dark.svg`:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 300 300">
  <defs>
    <linearGradient id="bg-dark" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" style="stop-color: oklch(0.25 0.05 280)"/>
      <stop offset="100%" style="stop-color: oklch(0.20 0.03 280)"/>
    </linearGradient>
  </defs>
  <rect width="300" height="300" fill="url(#bg-dark)"/>
  <g transform="translate(120, 120)" stroke="oklch(0.55 0.08 280)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none">
    <!-- Headphones icon -->
    <path d="M0 35 L0 25 C0 10 13 0 30 0 C47 0 60 10 60 25 L60 35"/>
    <rect x="0" y="35" width="12" height="20" rx="2"/>
    <rect x="48" y="35" width="12" height="20" rx="2"/>
  </g>
</svg>
```

**Step 3: Verify SVGs render correctly**

Open each SVG in a browser to verify:
- Light: Pale lavender gradient with dark purple headphones icon
- Dark: Dark muted purple gradient with lighter headphones icon
- Square aspect ratio (300x300)

**Step 4: Commit**

```bash
git add app/assets/placeholders/placeholder-audiobook-light.svg app/assets/placeholders/placeholder-audiobook-dark.svg
git commit -m "[Assets] Add audiobook placeholder SVGs for light and dark modes"
```

---

## Task 4: Create CoverPlaceholder Component

**Files:**
- Create: `app/components/library/CoverPlaceholder.tsx`

**Step 1: Create the component**

Create `app/components/library/CoverPlaceholder.tsx`:

```tsx
import { cn } from "@/lib/utils";

interface CoverPlaceholderProps {
  variant: "book" | "audiobook";
  className?: string;
}

// Book icon SVG paths (60x60 centered in viewBox)
const BookIcon = () => (
  <g
    transform="translate(70, 120)"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    fill="none"
  >
    <path d="M0 0 L0 60 L50 60 L50 5 L45 0 Z" />
    <path d="M0 0 L45 0" />
    <path d="M45 0 L45 5 L50 5" />
    <path d="M5 0 L5 60" />
  </g>
);

// Headphones icon SVG paths (60x55 centered in viewBox)
const HeadphonesIcon = () => (
  <g
    transform="translate(120, 120)"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    fill="none"
  >
    <path d="M0 35 L0 25 C0 10 13 0 30 0 C47 0 60 10 60 25 L60 35" />
    <rect x="0" y="35" width="12" height="20" rx="2" />
    <rect x="48" y="35" width="12" height="20" rx="2" />
  </g>
);

function CoverPlaceholder({ variant, className }: CoverPlaceholderProps) {
  const isBook = variant === "book";
  const viewBox = isBook ? "0 0 200 300" : "0 0 300 300";

  return (
    <div className={cn("relative w-full h-full", className)}>
      {/* Light mode SVG */}
      <svg
        className="absolute inset-0 w-full h-full dark:hidden"
        viewBox={viewBox}
        xmlns="http://www.w3.org/2000/svg"
      >
        <defs>
          <linearGradient id="bg-light" x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor="oklch(0.95 0.03 280)" />
            <stop offset="100%" stopColor="oklch(0.90 0.05 280)" />
          </linearGradient>
        </defs>
        <rect width="100%" height="100%" fill="url(#bg-light)" />
        <g stroke="oklch(0.65 0.08 280)">
          {isBook ? <BookIcon /> : <HeadphonesIcon />}
        </g>
      </svg>

      {/* Dark mode SVG */}
      <svg
        className="absolute inset-0 w-full h-full hidden dark:block"
        viewBox={viewBox}
        xmlns="http://www.w3.org/2000/svg"
      >
        <defs>
          <linearGradient id="bg-dark" x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor="oklch(0.25 0.05 280)" />
            <stop offset="100%" stopColor="oklch(0.20 0.03 280)" />
          </linearGradient>
        </defs>
        <rect width="100%" height="100%" fill="url(#bg-dark)" />
        <g stroke="oklch(0.55 0.08 280)">
          {isBook ? <BookIcon /> : <HeadphonesIcon />}
        </g>
      </svg>
    </div>
  );
}

export default CoverPlaceholder;
```

**Step 2: Verify the component compiles**

Run: `yarn lint:types`
Expected: No type errors related to CoverPlaceholder

**Step 3: Commit**

```bash
git add app/components/library/CoverPlaceholder.tsx
git commit -m "[UI] Add CoverPlaceholder component with theme-aware SVG rendering"
```

---

## Task 5: Integrate CoverPlaceholder into BookItem.tsx

**Files:**
- Modify: `app/components/library/BookItem.tsx:87-110`

**Reference:** See `@docs/plans/2025-01-14-placeholder-covers-design.md` for the integration pattern.

**Step 1: Add import and state**

At the top of BookItem.tsx, add the import:

```tsx
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
```

Inside the `BookItem` function, add state for tracking cover load errors:

```tsx
const [coverError, setCoverError] = useState(false);
```

**Step 2: Update the cover rendering logic**

Replace the current cover rendering section (around lines 87-110) with state-based conditional rendering:

Find the existing code pattern:
```tsx
<img
  alt={`${book.title} Cover`}
  className={cn(
    "w-full object-cover rounded-sm border-neutral-300 dark:border-neutral-600 border-1",
    aspectClass,
  )}
  onError={(e) => {
    (e.target as HTMLImageElement).style.display = "none";
    (e.target as HTMLImageElement).nextElementSibling!.textContent =
      "no cover";
  }}
  src={`/api/books/${book.id}/cover`}
/>
<div className="hidden">{/* fallback text element */}</div>
```

Replace with:
```tsx
{!coverError ? (
  <img
    alt={`${book.title} Cover`}
    className={cn(
      "w-full object-cover rounded-sm border-neutral-300 dark:border-neutral-600 border-1",
      aspectClass,
    )}
    onError={() => setCoverError(true)}
    src={`/api/books/${book.id}/cover`}
  />
) : (
  <CoverPlaceholder
    variant={isAudiobook ? "audiobook" : "book"}
    className={cn(
      "rounded-sm border border-neutral-300 dark:border-neutral-600",
      aspectClass,
    )}
  />
)}
```

Note: The `isAudiobook` variable should be derived from the existing logic that determines `aspectClass`. If no such variable exists, derive it from `coverFile?.file_type === "M4B"`.

**Step 3: Handle the case where coverFile is null**

If `coverFile` is null (no cover available at all), show placeholder immediately:

Check if there's early return logic or conditional around the cover. If coverFile can be null, wrap the cover display:

```tsx
{coverFile ? (
  // existing img/placeholder conditional
) : (
  <CoverPlaceholder
    variant={library.cover_aspect_ratio?.startsWith("audiobook") ? "audiobook" : "book"}
    className={cn(
      "rounded-sm border border-neutral-300 dark:border-neutral-600",
      aspectClass,
    )}
  />
)}
```

**Step 4: Verify the changes work**

Run: `yarn lint`
Expected: No errors

Run: `yarn start` (or verify dev server is running)
Navigate to a book without a cover to confirm placeholder displays correctly.

**Step 5: Commit**

```bash
git add app/components/library/BookItem.tsx
git commit -m "[UI] Use CoverPlaceholder in BookItem for missing covers"
```

---

## Task 6: Integrate CoverPlaceholder into BookDetail.tsx

**Files:**
- Modify: `app/components/pages/BookDetail.tsx:270-295`

**Step 1: Add import and state**

At the top of BookDetail.tsx, add the import:

```tsx
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
```

Add state for cover error tracking (near other useState calls):

```tsx
const [coverError, setCoverError] = useState(false);
```

**Step 2: Update the cover rendering logic**

Find the existing cover image section (around lines 276-290):

```tsx
<img
  alt={`${book.title} Cover`}
  className="w-full h-full object-cover rounded-md border border-border"
  onError={(e) => {
    (e.target as HTMLImageElement).style.display = "none";
    (
      e.target as HTMLImageElement
    ).nextElementSibling!.textContent = "No cover available";
  }}
  src={`/api/books/${book.id}/cover?t=${coverCacheBuster}`}
/>
<div className="hidden text-center text-muted-foreground"></div>
```

Replace with:
```tsx
{!coverError ? (
  <img
    alt={`${book.title} Cover`}
    className="w-full h-full object-cover rounded-md border border-border"
    onError={() => setCoverError(true)}
    src={`/api/books/${book.id}/cover?t=${coverCacheBuster}`}
  />
) : (
  <CoverPlaceholder
    variant={isAudiobook ? "audiobook" : "book"}
    className="rounded-md border border-border"
  />
)}
```

Note: `isAudiobook` should be derived from the existing display file logic. Check if it already exists in the component.

**Step 3: Reset coverError when book changes**

Add a useEffect to reset the error state when the book changes:

```tsx
useEffect(() => {
  setCoverError(false);
}, [book?.id]);
```

**Step 4: Verify the changes work**

Run: `yarn lint`
Expected: No errors

**Step 5: Commit**

```bash
git add app/components/pages/BookDetail.tsx
git commit -m "[UI] Use CoverPlaceholder in BookDetail for missing covers"
```

---

## Task 7: Integrate CoverPlaceholder into SeriesList.tsx

**Files:**
- Modify: `app/components/pages/SeriesList.tsx:88-115`

**Step 1: Add import**

At the top of SeriesList.tsx, add:

```tsx
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
```

**Step 2: Add state inside the map callback**

The series items are rendered in a map. For each series item, we need to track cover error state. Create a wrapper component or use a key-based approach.

Create a SeriesCard sub-component within the file (or extract to separate file if preferred):

```tsx
function SeriesCard({
  seriesItem,
  aspectClass,
  isAudiobook,
}: {
  seriesItem: SeriesWithBookCount;
  aspectClass: string;
  isAudiobook: boolean;
}) {
  const [coverError, setCoverError] = useState(false);

  return (
    <Link
      key={seriesItem.id}
      className="group block"
      to={`/series/${seriesItem.id}`}
    >
      <div
        className={cn(
          "rounded overflow-hidden bg-neutral-100 dark:bg-neutral-800 shadow-sm group-hover:shadow-md transition-shadow",
          aspectClass,
        )}
      >
        {!coverError ? (
          <img
            alt={`${seriesItem.name} Cover`}
            className={cn(
              "w-full object-cover rounded-sm border-neutral-300 dark:border-neutral-600 border-1",
              aspectClass,
            )}
            onError={() => setCoverError(true)}
            src={`/api/series/${seriesItem.id}/cover`}
          />
        ) : (
          <CoverPlaceholder
            variant={isAudiobook ? "audiobook" : "book"}
            className={cn(
              "rounded-sm border border-neutral-300 dark:border-neutral-600",
              aspectClass,
            )}
          />
        )}
      </div>
      <p className="mt-2 font-medium line-clamp-2">{seriesItem.name}</p>
      <div className="mt-1 text-sm line-clamp-1 text-neutral-500 dark:text-neutral-500">
        {seriesItem.book_count} {seriesItem.book_count === 1 ? "book" : "books"}
      </div>
    </Link>
  );
}
```

**Step 3: Use SeriesCard in the render**

Replace the existing map callback with the new SeriesCard component, passing the required props.

**Step 4: Verify the changes work**

Run: `yarn lint`
Expected: No errors

**Step 5: Commit**

```bash
git add app/components/pages/SeriesList.tsx
git commit -m "[UI] Use CoverPlaceholder in SeriesList for missing covers"
```

---

## Task 8: Integrate CoverPlaceholder into GlobalSearch.tsx

**Files:**
- Modify: `app/components/library/GlobalSearch.tsx:140-210`

**Step 1: Add import**

At the top of GlobalSearch.tsx, add:

```tsx
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
```

**Step 2: Create SearchResultCover sub-component**

Since each search result needs its own error state, create a sub-component:

```tsx
function SearchResultCover({
  type,
  id,
  thumbnailClasses,
  variant,
}: {
  type: "book" | "series";
  id: number;
  thumbnailClasses: string;
  variant: "book" | "audiobook";
}) {
  const [coverError, setCoverError] = useState(false);

  return (
    <div
      className={cn(
        "flex-shrink-0 bg-neutral-200 dark:bg-neutral-700 rounded overflow-hidden",
        thumbnailClasses,
      )}
    >
      {!coverError ? (
        <img
          alt=""
          className="w-full h-full object-cover"
          onError={() => setCoverError(true)}
          src={`/api/${type === "book" ? "books" : "series"}/${id}/cover`}
        />
      ) : (
        <CoverPlaceholder variant={variant} />
      )}
    </div>
  );
}
```

**Step 3: Use SearchResultCover for book results**

Replace the existing book cover rendering with:

```tsx
<SearchResultCover
  type="book"
  id={book.id}
  thumbnailClasses={thumbnailClasses}
  variant={isAudiobook ? "audiobook" : "book"}
/>
```

**Step 4: Use SearchResultCover for series results**

Replace the existing series cover rendering with:

```tsx
<SearchResultCover
  type="series"
  id={series.id}
  thumbnailClasses={thumbnailClasses}
  variant={isAudiobook ? "audiobook" : "book"}
/>
```

Note: Determine `isAudiobook` from the library's `cover_aspect_ratio` setting.

**Step 5: Verify the changes work**

Run: `yarn lint`
Expected: No errors

**Step 6: Commit**

```bash
git add app/components/library/GlobalSearch.tsx
git commit -m "[UI] Use CoverPlaceholder in GlobalSearch for missing covers"
```

---

## Task 9: Integrate CoverPlaceholder into FileEditDialog.tsx

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx:351-392`

**Step 1: Add import**

At the top of FileEditDialog.tsx, add:

```tsx
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
```

**Step 2: Update the cover display section**

Find the existing cover display (around line 360):

```tsx
{file.cover_mime_type ? (
  <img
    alt="File cover"
    className="w-full h-auto rounded border border-border"
    src={`/api/books/files/${file.id}/cover?t=${coverCacheBuster}`}
  />
) : (
  <div className="w-full aspect-square rounded border border-dashed border-border flex items-center justify-center text-muted-foreground text-xs bg-muted/30">
    No cover
  </div>
)}
```

Replace with:

```tsx
{file.cover_mime_type ? (
  <img
    alt="File cover"
    className="w-full h-auto rounded border border-border"
    src={`/api/books/files/${file.id}/cover?t=${coverCacheBuster}`}
  />
) : (
  <CoverPlaceholder
    variant={file.file_type === "M4B" ? "audiobook" : "book"}
    className="rounded border border-dashed border-border aspect-square"
  />
)}
```

**Step 3: Verify the changes work**

Run: `yarn lint`
Expected: No errors

**Step 4: Commit**

```bash
git add app/components/library/FileEditDialog.tsx
git commit -m "[UI] Use CoverPlaceholder in FileEditDialog for missing covers"
```

---

## Task 10: Final Verification and Cleanup

**Step 1: Run full lint and type check**

```bash
yarn lint
```
Expected: All checks pass

**Step 2: Run make check**

```bash
make check
```
Expected: All tests and lints pass

**Step 3: Manual testing checklist**

Test each integration point:
- [ ] BookItem gallery - book without cover shows book placeholder
- [ ] BookItem gallery - audiobook without cover shows audiobook placeholder
- [ ] BookDetail page - missing cover shows appropriate placeholder
- [ ] SeriesList - series without cover shows placeholder
- [ ] GlobalSearch - search results without covers show placeholders
- [ ] FileEditDialog - file without cover shows placeholder
- [ ] Theme toggle - placeholders switch between light/dark correctly

**Step 4: Update design document**

Update `docs/plans/2025-01-14-placeholder-covers-design.md` to mark implementation as complete (if there's a status field) or add implementation notes.

**Step 5: Final commit (if any cleanup needed)**

```bash
git add -A
git commit -m "[UI] Complete placeholder cover implementation"
```

---

## Summary

This implementation:
1. Creates 4 SVG placeholder files (book light/dark, audiobook light/dark)
2. Creates a `CoverPlaceholder` component with CSS-based theme switching
3. Integrates into 5 components: BookItem, BookDetail, SeriesList, GlobalSearch, FileEditDialog
4. Uses state-based conditional rendering instead of DOM manipulation
5. Maintains consistent styling with existing cover borders
