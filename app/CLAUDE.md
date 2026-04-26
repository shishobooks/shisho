# Shisho Frontend Development

This file documents frontend patterns and conventions specific to Shisho.

## Stack

- React 19 with TypeScript
- TailwindCSS for styling (dark/light theme support)
- Tanstack Query for server state
- Vite for bundling
- Radix UI primitives with shadcn/ui patterns

## Architecture

### React Router (`app/router.tsx`)

- Single page app with client-side routing
- Main route loads Home page with book gallery

### State Management

- **Tanstack Query** for server state (books, jobs, libraries)
- **React Context** for theme management
- No global client state management library

### API Integration

- `app/libraries/api.ts` contains HTTP client functions
- Query hooks in `app/hooks/queries/` wrap API calls with Tanstack Query
- TypeScript types auto-imported from `app/types/generated/`

**IMPORTANT - List Limits:**
- **Default list limit is 50** - All list endpoints have a max limit of 50 items per request
- **Always use server-side search** - Never rely on client-side filtering for searchable lists; always pass search queries to the API. This ensures users can find items beyond the initial 50 loaded.

### React Query Cache Invalidation

When a mutation modifies a resource (update/delete/merge), invalidate related queries so the UI refreshes.

**Cross-resource invalidation is required**: When metadata entities (genres, tags, series, people, publishers, imprints) are modified, also invalidate `ListBooks` and `RetrieveBook` queries since books display this metadata.

**Pattern:**
```typescript
import { QueryKey as BooksQueryKey } from "./books";

// In mutation onSuccess:
onSuccess: () => {
  queryClient.invalidateQueries({ queryKey: [QueryKey.ListGenres] });
  // Also invalidate book queries since they display genre info
  queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
  queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
}
```

### UI Components

- Custom components in `app/components/` using Radix UI primitives
- Tailwind CSS for styling with dark/light theme support
- Components follow shadcn/ui patterns
- Add new shadcn components using `npx shadcn@latest add`

### Tabbed Navigation (Deep Linking Required)

**All tabbed navigation MUST be deeply linked via URL parameters.** Tabs should never use local state alone (`defaultValue` / `useState`). Instead, sync tab state with the URL so tabs are bookmarkable and support browser back/forward.

**Pattern:**
1. Add `/:tab?` to the route in `app/router.tsx`
2. Extract the tab param with `useParams()`
3. Validate against allowed values, defaulting to the first tab
4. Use `navigate()` in `onValueChange` to update the URL
5. Pass controlled `value` and `onValueChange` to `<Tabs>`

```tsx
const validTabs = ["details", "settings"] as const;
type TabValue = (typeof validTabs)[number];

const MyPage = () => {
  const { tab } = useParams<{ tab?: string }>();
  const navigate = useNavigate();

  const activeTab: TabValue = validTabs.includes(tab as TabValue)
    ? (tab as TabValue)
    : "details";

  const handleTabChange = (value: string) => {
    if (value === "details") {
      navigate("/my-page"); // Clean URL for default tab
    } else {
      navigate(`/my-page/${value}`);
    }
  };

  return (
    <Tabs onValueChange={handleTabChange} value={activeTab}>
      <TabsTrigger value="details">Details</TabsTrigger>
      <TabsTrigger value="settings">Settings</TabsTrigger>
      {/* ... */}
    </Tabs>
  );
};
```

**When tabs are distinct routes:** Some pages give each tab its own explicit route (e.g., `AdminPlugins` has `/settings/plugins`, `/settings/plugins/installed`, `/settings/plugins/discover` in `app/router.tsx`). Explicit routes enable cleaner bookmarks and allow `<Navigate>` redirects for legacy paths (e.g., `/settings/plugins/browse` → `/settings/plugins/discover`). In that case, derive `activeTab` from `useLocation()` instead of `useParams()`:

```tsx
const location = useLocation();
const activeTab = location.pathname.endsWith("/discover") ? "discover" : "installed";
```

Use the `useParams` pattern by default; reach for `useLocation` only when tabs are distinct top-level routes.

### Page Titles

**All pages MUST set a browser title** using the `usePageTitle` hook. This improves UX by showing meaningful titles in browser tabs and history.

**Hook:** `app/hooks/usePageTitle.ts`

```tsx
import { usePageTitle } from "@/hooks/usePageTitle";

// Static title for list pages
const GenresList = () => {
  usePageTitle("Genres");
  // ...
};

// Dynamic title for detail pages
const BookDetail = () => {
  const { data: book } = useBook(id);
  usePageTitle(book?.title);
  // ...
};
```

**Title Format:** `{Page Title} - Shisho` (e.g., "The Great Gatsby - Shisho")

**Guidelines:**
- List pages: Use plural noun (e.g., "Genres", "Tags", "Users & Roles")
- Detail pages: Use entity name/title from data (e.g., book title, username, series name)
- Settings pages: Use descriptive name (e.g., "User Settings", "Server Settings")
- Call hook early in component, before any early returns if using static title
- For dynamic titles, pass `undefined` while loading - hook handles this gracefully

## Handling Long Text in UI

When displaying user-generated content that may be long (names, titles, etc.):

### Dialogs
- Use `overflow-x-hidden` on `DialogContent` to prevent horizontal scrolling
- Avoid `overflow-hidden` on inner containers as it clips focus rings

### Dialog Headers
- Add `pr-8` to `DialogHeader` to leave room for the close button
- Let titles wrap naturally rather than truncating

### Page Headers with Buttons
```tsx
<div className="flex items-start justify-between gap-4">
  <h1 className="min-w-0 break-words">{title}</h1>
  <div className="shrink-0">{buttons}</div>
</div>
```

### Badges with Long Text
```tsx
<Badge className="max-w-full">
  <span className="truncate" title={text}>{text}</span>
  <button className="shrink-0">×</button>
</Badge>
```

### Flex Containers with Truncation
- Parent needs `min-w-0` for `truncate` to work on children

### Dropdowns/Command Items
```tsx
<CommandItem>
  <Icon className="shrink-0" />
  <span className="truncate" title={text}>{text}</span>
</CommandItem>
```

## Mobile Responsiveness

### Breakpoints

Tailwind breakpoints used in Shisho:
- `sm:` (640px) - Small tablets, large phones in landscape
- `md:` (768px) - Tablets, where desktop sidebar appears
- `lg:` (1024px) - Desktop

### Page Headers with Actions

**Never put title and action buttons on the same row.** On mobile, long titles wrap while buttons stay at top, creating awkward layouts.

```tsx
// Good - stacked layout
<div className="flex flex-col gap-3 mb-6">
  <h1 className="text-2xl md:text-3xl font-semibold">{title}</h1>
  <div className="flex items-center gap-2">
    <Button size="sm" variant="outline">
      <Edit className="h-4 w-4 sm:mr-2" />
      <span className="hidden sm:inline">Edit</span>
    </Button>
  </div>
</div>

// Bad - side-by-side causes layout issues
<div className="flex items-start justify-between">
  <h1 className="text-3xl">{title}</h1>
  <Button>Edit</Button>
</div>
```

### Admin Page Headers

For settings/admin pages with title, description, and action buttons:

```tsx
<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6 md:mb-8">
  <div>
    <h1 className="text-xl md:text-2xl font-semibold mb-1 md:mb-2">
      Page Title
    </h1>
    <p className="text-sm md:text-base text-muted-foreground">
      Description text here.
    </p>
  </div>
  <div className="flex items-center gap-2 shrink-0">
    <Button size="sm">
      <Plus className="h-4 w-4 sm:mr-2" />
      <span className="hidden sm:inline">Add Item</span>
    </Button>
  </div>
</div>
```

### Icon-Only Buttons on Mobile

Use `hidden sm:inline` pattern for button text:

```tsx
<Button size="sm" variant="outline">
  <Edit className="h-4 w-4 sm:mr-2" />
  <span className="hidden sm:inline">Edit</span>
</Button>
```

### List Row Items

For rows with multiple pieces of info (stats, actions), stack on mobile:

```tsx
// File/item row with stats
<div className="py-2 space-y-1">
  {/* Primary row - always horizontal */}
  <div className="flex items-center gap-2">
    <Badge>{type}</Badge>
    <span className="truncate min-w-0 flex-1">{name}</span>
  </div>

  {/* Stats row - separate line gives name room to breathe */}
  <div className="flex items-center gap-2 text-xs text-muted-foreground pl-6">
    <span>8h 44m</span>
    <span className="text-muted-foreground/50">·</span>
    <span>64 kbps</span>
    <span className="text-muted-foreground/50">·</span>
    <span>244.8 MB</span>
    {/* Action buttons */}
  </div>
</div>
```

### Dot Separators for Stats

Use middle dots (·) with faded styling to separate inline stats:

```tsx
<span>{duration}</span>
<span className="text-muted-foreground/50">·</span>
<span>{bitrate}</span>
<span className="text-muted-foreground/50">·</span>
<span>{fileSize}</span>
```

### Cover Images

Center and constrain cover images on mobile:

```tsx
<div className="w-48 sm:w-64 lg:w-full mx-auto lg:mx-0">
  <img className="w-full h-full object-cover" src={cover} />
</div>
```

## Cover Image Freshness

Cover URLs include a `?v=${cacheKey}` query parameter that bumps whenever the underlying data is refetched. The backend emits `Cache-Control: private, no-cache` + `Last-Modified` and returns `304 Not Modified` on conditional GET, but the query-param is the primary freshness mechanism. Both layers together give reliable updates across browsers.

### Why URL-based busting is required

Chromium and Firefox maintain an in-memory image cache (the HTML spec's "list of available images") that is **separate from the HTTP cache**. When an `<img>` element's `src` matches a URL previously rendered in the session, the browser serves the cached decoded bitmap without hitting HTTP — bypassing `Cache-Control: no-cache` entirely. Stable URLs + `Cache-Control` alone are insufficient.

References:
- [whatwg/fetch#1088](https://github.com/whatwg/fetch/issues/1088) — browsers reusing `no-cache` cached images
- [Mozilla bug 1719583](https://bugzilla.mozilla.org/show_bug.cgi?id=1719583) — Firefox image cache persists across `fetch({cache: 'reload'})`

### Rules

- **Append `?v=${cacheKey}`** to cover URLs. `cacheKey` must be a value that changes when the underlying data is refetched (e.g., `bookQuery.dataUpdatedAt`), **not** `entity.updated_at` (which doesn't bump on file-cover changes).
- **For pages that mutate covers** (BookDetail, FileEditDialog), also add `key={cacheKey}` to the `<img>` tag. React remounting combined with URL change gives reliable refresh.
- **For child components that render covers**, accept a `cacheKey?: number` prop. Parents pass their query's `dataUpdatedAt` or the relevant entity's `updated_at` (for file covers, since `file.updated_at` bumps reliably on cover mutations).
- **Source selection by endpoint:**
  - `/api/books/:id/cover` (book cover, selected from files) — parent book query's `dataUpdatedAt` (or a cascaded `cacheKey` prop).
  - `/api/books/files/:id/cover` (specific file's cover) — `file.updated_at` (reliable) or the parent query's `dataUpdatedAt`.
  - `/api/series/:id/cover` (series cover, from first book) — parent series query's `dataUpdatedAt`.

### Checklist for new cover components

- [ ] Cover URL includes `?v=${cacheKey}` with a value that bumps when data is refetched
- [ ] `cacheKey` source is reliable (`query.dataUpdatedAt` or `file.updated_at` — NOT `book.updated_at` for cover purposes)
- [ ] For mutation-capable pages, `<img key={cacheKey}>` for React remount
- [ ] Cover-mutating mutations invalidate the query whose `dataUpdatedAt` drives the key

### Breadcrumbs

Make breadcrumbs responsive with truncation:

```tsx
<nav className="text-xs sm:text-sm text-muted-foreground">
  <ol className="flex items-center gap-1 sm:gap-2 flex-wrap">
    <li className="shrink-0">
      <Link to="/">Home</Link>
    </li>
    <li className="shrink-0">›</li>
    <li className="truncate max-w-[120px] sm:max-w-none">
      <Link to="/book">{longBookTitle}</Link>
    </li>
    <li className="shrink-0">›</li>
    <li className="truncate">{currentPage}</li>
  </ol>
</nav>
```

### Card/Section Padding

Reduce padding on mobile:

```tsx
<div className="border rounded-md p-4 md:p-6">
  <h2 className="text-base md:text-lg font-semibold mb-3 md:mb-4">
    Section Title
  </h2>
</div>
```

### Config/Settings Rows

Stack label and value on mobile for long values:

```tsx
<div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-1 sm:gap-4">
  <span className="text-sm font-medium">{label}</span>
  <span className="text-xs sm:text-sm text-muted-foreground font-mono break-all sm:break-normal">
    {value}
  </span>
</div>
```

### Mobile Navigation

- Sidebar is hidden on mobile (`hidden md:block`)
- Use `MobileDrawer` component with hamburger menu in header
- Drawer slides in from left with backdrop blur
- `MobileNavContext` provides `isOpen`, `open`, `close`, `toggle`

### Form Inputs on Mobile

Remove focus ring glow for cleaner mobile appearance:

```tsx
<Input
  className={cn(
    fullWidth && "focus-visible:ring-0 focus-visible:border-border"
  )}
/>
```

### Fixed Position Dropdowns

When a dropdown is inside an `overflow-hidden` container (like collapsible sections), use fixed positioning:

```tsx
<div
  className={cn(
    "bg-background border rounded-lg shadow-lg z-50",
    isMobile
      ? "fixed left-4 right-4 top-28"
      : "absolute top-full mt-2 left-0 w-80"
  )}
>
```

### Tabs on Mobile

Make tabs scrollable and use smaller text:

```tsx
<TabsList className="w-full justify-start overflow-x-auto">
  <TabsTrigger className="text-xs sm:text-sm" value="tab1">
    Tab 1
  </TabsTrigger>
  <TabsTrigger className="text-xs sm:text-sm" value="tab2">
    Longer Tab Name
  </TabsTrigger>
</TabsList>
```

### Spacing Patterns

Use responsive spacing throughout:

```tsx
// Margins
className="mb-6 md:mb-8"
className="mb-1 md:mb-2"

// Gaps
className="gap-4 md:gap-8"
className="space-y-4 md:space-y-6"

// Padding
className="py-3 md:py-4 px-4 md:px-6"
```

## Testing

### Test Stack

| Level | Framework | Purpose |
|-------|-----------|---------|
| Unit + Component | Vitest + React Testing Library | Fast, native Vite integration |
| E2E | Playwright | Browser automation |

### Running Tests

```bash
mise test:unit      # Run Vitest unit/component tests with coverage
mise test:js        # Run all JS tests (unit + E2E, used in `mise check`)
mise test:e2e       # Run E2E tests (Chromium + Firefox in parallel)
```

### Test File Locations

- **Unit/Component tests**: Colocated with source files as `*.test.ts(x)`
- **E2E tests**: Separate `e2e/` directory

### Writing Unit Tests

```typescript
import { describe, expect, it } from "vitest";
import { myFunction } from "./myFile";

describe("myFunction", () => {
  it("returns expected value", () => {
    expect(myFunction("input")).toBe("output");
  });
});
```

### Writing Component Tests

```typescript
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import MyComponent from "./MyComponent";

describe("MyComponent", () => {
  it("renders correctly", () => {
    render(<MyComponent prop="value" />);
    expect(screen.getByText("expected text")).toBeInTheDocument();
  });
});
```

### Fake Timers and `userEvent`

- `vitest.setup.ts` enables fake timers globally with `shouldAdvanceTime: true`
- When a test uses `userEvent.setup()`, pass `advanceTimers: vi.advanceTimersByTime` so clicks and typing don't stall or hit the 5s test timeout under heavy load

```typescript
const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
```

### E2E Tests

**See `e2e/CLAUDE.md` for detailed E2E patterns**, including:
- Test independence via `beforeAll` hooks
- Test-only API endpoints (`ENVIRONMENT=test`)
- Common pitfalls (shared database, toast assertions, redirect expectations)

### Coverage

- Collected automatically with unit tests (V8 provider)
- Reports: text (console), lcov, HTML in `coverage/`
- Not enforced, tracked for visibility

## Metadata Edit Dialogs

### First-Class Metadata Fields

All editable metadata fields should be treated as first-class citizens. Don't add confusing helper text that implies the field is secondary or derived.

**Don't do this:**
```tsx
<Label htmlFor="name">Name</Label>
<Input id="name" value={name} onChange={...} />
<p className="text-muted-foreground">
  Leave empty to use the title from file metadata.
</p>
```

**Do this instead:**
```tsx
<Label htmlFor="name">Name</Label>
<Input id="name" value={name} onChange={...} />
```

**Why:** Helper text like "Leave empty to use..." implies the field is optional or secondary. This confuses users about what the field does and what happens when they clear it. Metadata fields should be straightforward - what you enter is what you get.

### Field Clearing Behavior

When a user clears a metadata field, the cleared value should be saved (not revert to some default). The scanner will repopulate the field from the source file on the next scan if needed.

## Unsaved Changes Protection (Required)

**All forms that create or update data MUST have unsaved changes protection.** This prevents users from accidentally losing their work when navigating away or closing a dialog.

### When Protection is Required

- Create forms (CreateUser, CreateLibrary, Setup)
- Edit forms (UserDetail, LibrarySettings, BookEditDialog)
- Any dialog or page where users enter data and click "Save"

### When Protection is NOT Required

- Action dialogs that execute immediately (merge, move, delete confirmation)
- Quick actions with no pending state (add to list popover)
- View-only pages
- Settings that apply immediately on change (theme selection)

### Pattern 1: Dialog Forms (FormDialog)

For forms inside dialogs, use `FormDialog` instead of `Dialog`:

```tsx
import { FormDialog } from "@/components/ui/form-dialog";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";

const MyEditDialog = ({ open, onOpenChange, initialData }) => {
  const [name, setName] = useState("");
  const [initialValues, setInitialValues] = useState<{ name: string } | null>(null);

  // Initialize form when dialog opens
  useEffect(() => {
    if (open && initialData) {
      setName(initialData.name);
      setInitialValues({ name: initialData.name });
    }
  }, [open, initialData]);

  // Compute hasChanges by comparing current values to initial
  const hasChanges = useMemo(() => {
    if (!initialValues) return false;
    return name !== initialValues.name;
  }, [name, initialValues]);

  // useFormDialogClose handles closing after save (waits for hasChanges to update)
  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const handleSave = async () => {
    await mutation.mutateAsync({ name });
    setInitialValues({ name }); // Update initial values so hasChanges becomes false
    requestClose(); // Close after state updates
  };

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        {/* form fields */}
        <Button onClick={handleSave}>Save</Button>
      </DialogContent>
    </FormDialog>
  );
};
```

**Key points:**
- `FormDialog` wraps `Dialog` and intercepts close attempts when `hasChanges` is true
- Shows `UnsavedChangesDialog` confirmation before discarding
- Also adds `beforeunload` handler for browser close/refresh
- `useFormDialogClose` ensures dialog closes only after `hasChanges` updates to false

### Pattern 2: Full-Page Forms (useUnsavedChanges)

For forms on full pages (not dialogs), use the `useUnsavedChanges` hook:

```tsx
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";

const MySettingsPage = () => {
  const [name, setName] = useState("");
  const [initialValues, setInitialValues] = useState<{ name: string } | null>(null);
  const [isInitialized, setIsInitialized] = useState(false);

  // Reset when navigating to different entity
  useEffect(() => {
    setIsInitialized(false);
  }, [entityId]);

  // Initialize form when data loads
  useEffect(() => {
    if (query.isSuccess && query.data && !isInitialized) {
      setName(query.data.name);
      setInitialValues({ name: query.data.name });
      setIsInitialized(true);
    }
  }, [query.isSuccess, query.data, isInitialized]);

  // Compute hasChanges
  const hasChanges = useMemo(() => {
    if (!initialValues || !isInitialized) return false;
    return name !== initialValues.name;
  }, [name, initialValues, isInitialized]);

  // Hook provides blocker dialog state and handlers
  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(hasChanges);

  const handleSave = async () => {
    await mutation.mutateAsync({ name });
    setInitialValues({ name }); // hasChanges becomes false
  };

  return (
    <>
      {/* form content */}
      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />
    </>
  );
};
```

**Key points:**
- `useUnsavedChanges` blocks SPA navigation via `react-router`'s `useBlocker`
- Also adds `beforeunload` handler for browser close/refresh
- Must render `UnsavedChangesDialog` and wire up the handlers

### Pattern 3: Child Components with Unsaved State

When a child component has its own save button and unsaved state (like LibraryPluginsTab inside LibrarySettings), expose the changes via callback:

**Child component:**
```tsx
interface Props {
  onHasChangesChange?: (hasChanges: boolean) => void;
}

const ChildEditor = ({ onHasChangesChange }) => {
  const [localData, setLocalData] = useState(null);

  const hasChanges = localData !== null && /* comparison logic */;

  useEffect(() => {
    onHasChangesChange?.(hasChanges);
  }, [hasChanges, onHasChangesChange]);

  // ... rest of component
};
```

**Parent component:**
```tsx
const ParentPage = () => {
  const [childHasChanges, setChildHasChanges] = useState(false);

  // Combine parent form changes with child changes
  const hasChanges = formHasChanges || childHasChanges;

  const { showBlockerDialog, ... } = useUnsavedChanges(hasChanges);

  return (
    <>
      <ChildEditor onHasChangesChange={setChildHasChanges} />
      {/* ... */}
    </>
  );
};
```

### Pattern 4: Tab Switching with Unsaved Changes

When a page has tabs and one tab has inline editing, intercept tab changes:

```tsx
const [pendingTabChange, setPendingTabChange] = useState<string | null>(null);

const handleTabChange = (value: string) => {
  if (hasChanges) {
    setPendingTabChange(value);
    return;
  }
  navigateToTab(value);
};

const handleConfirmTabChange = () => {
  if (pendingTabChange) {
    setIsEditing(false);
    navigateToTab(pendingTabChange);
    setPendingTabChange(null);
  }
};

return (
  <>
    <Tabs onValueChange={handleTabChange} value={activeTab}>
      {/* ... */}
    </Tabs>
    <UnsavedChangesDialog
      onDiscard={handleConfirmTabChange}
      onStay={() => setPendingTabChange(null)}
      open={pendingTabChange !== null}
    />
  </>
);
```

### Comparing Values with Arrays/Objects

For complex state with arrays or objects, use `fast-deep-equal`:

```tsx
import equal from "fast-deep-equal";

const hasChanges = useMemo(() => {
  if (!initialValues) return false;
  return (
    name !== initialValues.name ||
    !equal(selectedItems, initialValues.selectedItems)
  );
}, [name, selectedItems, initialValues]);
```

### Checklist for New Forms

- [ ] Form uses `FormDialog` (dialogs) or `useUnsavedChanges` (pages)
- [ ] `initialValues` state stores values when form loads
- [ ] `hasChanges` computed by comparing current state to initial values
- [ ] After successful save, update `initialValues` so `hasChanges` becomes false
- [ ] `UnsavedChangesDialog` rendered with proper handlers
- [ ] Child components with own save buttons expose `onHasChangesChange`
- [ ] Tab switching intercepted if tabs have inline editing

## UI/UX Consistency Requirements

### List Page Patterns

All list pages (Books, Series, People, Genres, Tags) should follow consistent patterns:

**Required Elements:**
1. **Page header** with title and subtitle in `<div className="mb-6">`
2. **Search input** with `max-w-xs` and appropriate placeholder
3. **Item count display**: Show "Showing X-Y of Z [items]" above the list **only when total > 0** (hide when empty to avoid "Showing 1-0 of 0")
4. **Loading state**: Use `<LoadingSpinner />` component, not raw text
5. **Pagination**: Use shadcn/ui `Pagination` components, never raw `<button>` elements

**Use the Gallery Component for Grid Layouts:**
For pages displaying items in a grid (books, series), use the `Gallery` component which provides:
- Consistent "Showing X-Y of Z" count
- Pagination with proper shadcn/ui components
- Loading state handling

```tsx
<Gallery
  isLoading={query.isLoading}
  isSuccess={query.isSuccess}
  itemLabel="books"
  items={query.data?.items ?? []}
  itemsPerPage={24}
  renderItem={renderItem}
  total={query.data?.total ?? 0}
/>
```

**For List-Based Pages (People, Genres, Tags):**
Even though these pages don't use Gallery, they should still:
- Show "Showing X-Y of Z [items]" count **only when total > 0**
- Use `<LoadingSpinner />` for loading states
- Use shadcn/ui Pagination components
- Have consistent empty state messages that differentiate between "no results" and "no results matching search"

**Item Count Pattern:**
```tsx
{total > 0 && (
  <div className="mb-4 text-sm text-muted-foreground">
    Showing {offset + 1}-{Math.min(offset + limit, total)} of {total} items
  </div>
)}
```

**Empty State Messages:**
```tsx
// With search context
{searchQuery
  ? "No people found matching your search."
  : "No people in this library yet."}

// Without search context (less ideal)
"No genres found"
```

### Detail Page Patterns

All metadata detail pages (Series, Person, Genre, Tag) follow a consistent structure:

**Header Section:**
```tsx
<div className="mb-8">
  <div className="flex items-start justify-between gap-4 mb-2">
    <h1 className="text-3xl font-bold min-w-0 break-words">{name}</h1>
    <div className="flex gap-2 shrink-0">
      <Button onClick={() => setEditOpen(true)} size="sm" variant="outline">
        <Edit className="h-4 w-4 mr-2" />
        Edit
      </Button>
      <Button onClick={() => setMergeOpen(true)} size="sm" variant="outline">
        <GitMerge className="h-4 w-4 mr-2" />
        Merge
      </Button>
      {canDelete && (
        <Button onClick={() => setDeleteOpen(true)} size="sm" variant="outline">
          <Trash2 className="h-4 w-4 mr-2" />
          Delete
        </Button>
      )}
    </div>
  </div>
  {/* Optional: Sort name if different */}
  {sortName !== name && (
    <p className="text-muted-foreground mb-2">Sort name: {sortName}</p>
  )}
  <Badge variant="secondary">{count} book{count !== 1 ? "s" : ""}</Badge>
</div>
```

**Content Sections:**
```tsx
<section className="mb-10">
  <h2 className="text-xl font-semibold mb-4">Books in Series</h2>
  {/* ... content ... */}
</section>
```

**Empty States:**
```tsx
<div className="text-center py-8 text-muted-foreground">
  This [entity] has no associated books.
</div>
```

### Always Use Button Component

**Never use raw `<button>` elements.** Always use the shadcn/ui `Button` component for:
- Consistent styling across the app
- Built-in cursor-pointer behavior
- Proper disabled states
- Accessibility features

```tsx
// Bad - raw button
<button className="px-3 py-1 rounded-md border">Previous</button>

// Good - Button component
<Button variant="outline" size="sm">Previous</Button>
```

### Cursor Styles for Interactive Elements

**All clickable elements MUST have `cursor-pointer`**. This is a fundamental UX requirement that signals interactivity to users.

**Components that need `cursor-pointer`:**
- Buttons (already in base `buttonVariants`)
- Checkboxes
- Select triggers and items
- Tab triggers
- Command items (in dropdowns/comboboxes)
- Dropdown menu items (including checkbox/radio items and sub-triggers)
- Dialog close buttons
- Pagination links
- Any custom clickable element (raw `<button>` or clickable `<div>`)

**Pattern for shadcn/ui components:**
When adding or modifying UI components, ensure `cursor-pointer` is in the base className:

```tsx
// Good - cursor-pointer included
className={cn(
  "flex items-center justify-center cursor-pointer",
  "disabled:cursor-not-allowed disabled:opacity-50",
  className,
)}

// Bad - missing cursor-pointer
className={cn(
  "flex items-center justify-center",
  "disabled:cursor-not-allowed disabled:opacity-50",
  className,
)}
```

**Pattern for raw buttons:**
When using raw `<button>` elements outside of the Button component, always add `cursor-pointer`:

```tsx
// Good
<button className="px-4 py-2 rounded-md cursor-pointer" onClick={...}>

// Bad
<button className="px-4 py-2 rounded-md" onClick={...}>
```

**Why this matters:**
- Users rely on cursor changes to understand what's clickable
- Missing cursor-pointer feels broken/unresponsive
- Consistency across the UI is essential for professional UX

### Dynamic Class Composition with `cn()`

**Always use `cn()` from `@/libraries/utils` for dynamic className composition.** Never use template literals to concatenate class strings.

```tsx
// Good - cn()
<span className={cn("inline-flex h-5 w-5 rounded-sm", colorClass)}>

// Good - cn() with conditionals
<button className={cn(
  "h-8 rounded-md border px-3 text-xs cursor-pointer",
  isSelected
    ? "border-primary bg-primary text-primary-foreground"
    : "border-border bg-card hover:bg-accent",
)}>

// Bad - template literal
<span className={`inline-flex h-5 w-5 rounded-sm ${colorClass}`}>
```

`cn()` wraps `clsx` + `tailwind-merge`, so it handles conditional classes, deduplication, and Tailwind conflict resolution. Template literals bypass all of that.

## Sortable List Row Keys

**Sortable lists (dnd-kit-backed `SortableList` and consumers like `SortableEntityList`, `FileChaptersTab`) MUST use stable client-side row keys that survive reorder/remove.** Index-based keys (`${index}`) and content-based keys (`${item.name}-${index}`) both change on every reorder, which confuses dnd-kit's drag tracking — the active drag's identity changes mid-gesture, causing flicker, dropped drags, or rows that mutate the wrong sibling after sorting.

**Pattern:** Assign each row a stable id when it first enters the list (mount or append) and preserve it across reorder/remove. `FileChaptersTab` uses an `EditedChapter._editKey` field generated by a module-level monotonic counter (`nextEditKey()`); `SortableEntityList` uses the same counter pattern but stores the key in a `useRef<WeakMap<T, string>>` keyed by item reference (so callers don't need to inject a `_key` field on their own types).

**Don't** rely on labels, indices, or any field that changes during normal editing as the sortable id. Use a server-side stable id (e.g., `chapter.id`) only when every row actually has one — newly-added rows that haven't been persisted yet need a client-side counter or WeakMap-tracked id.

## Known Radix UI Issues

### Dialog + DropdownMenu pointer-events Bug

**Problem:** When a Dialog is triggered from a DropdownMenu item, Radix's DismissableLayer incorrectly sets `pointer-events: none` on the body during unmount, leaving the page unclickable after the dialog closes.

**Solution:** Already fixed globally in `app/components/ui/dialog.tsx`. The custom `Dialog` wrapper includes:
1. A cleanup effect that clears `pointer-events` when `open` changes to `false`
2. An unmount cleanup effect for conditionally rendered dialogs

The 300ms delay ensures cleanup runs after Radix's buggy unmount effects complete.

**If you encounter similar issues:**
1. Use browser DevTools to check if `pointer-events: none` is stuck on `<body>`
2. Use a MutationObserver or setter trap to identify what's setting the style
3. Add a delayed cleanup effect that runs after Radix's effects complete

**Related:** DropdownMenu components that trigger dialogs should also have `onCloseAutoFocus={(e) => e.preventDefault()}` on `DropdownMenuContent` to prevent focus management conflicts.

### asChild trigger components must forwardRef

**Problem:** When a Radix `XxxTrigger asChild` wraps a custom React function component (instead of a direct `<Button>` or DOM element), the component must be a `forwardRef` that spreads incoming props onto the underlying button. Otherwise:

- For **floating** primitives (`Popover`, `DropdownMenu`, `HoverCard`, `Tooltip` with positioning, `ContextMenu`): the popper has no DOM ref to anchor to, so Floating UI falls back to the document origin `(0, 0)` and the content renders **off-screen** (often above the viewport). The trigger's onClick still fires — the component appears to do nothing.
- For **non-floating** primitives (`Sheet`, `Drawer`, `Dialog`): the panel still renders correctly because it's positioned relative to the viewport, not the trigger. But focus management on close can't restore focus to the trigger, and screen reader / keyboard semantics suffer.

This bug is **invisible in jsdom unit tests** — Radix's positioning math doesn't run there. Caught only in a real browser.

**Required pattern for any custom component used as an asChild trigger:**

```tsx
import { forwardRef } from "react";

export const MyButton = forwardRef<
  HTMLButtonElement,
  { isDirty: boolean } & React.ComponentPropsWithoutRef<typeof Button>
>(({ isDirty, ...props }, ref) => (
  <Button ref={ref} {...props}>
    {/* ... */}
  </Button>
));
MyButton.displayName = "MyButton";
```

Three things matter:
1. `forwardRef` — receives the ref from Radix's Slot
2. `ref={ref}` on the underlying `<Button>` — passes the ref to a DOM element (Button itself is forwardRef'd)
3. `{...props}` — Radix's Slot adds `onClick`, `aria-expanded`, `aria-controls`, `data-state` etc. via `React.cloneElement`; these must reach the button

Direct `<Button>` (the shadcn/ui primitive) is already forwardRef'd, so the common case `<PopoverTrigger asChild><Button>...</Button></PopoverTrigger>` works without ceremony. The footgun is when you wrap that Button in a custom presentational component (`SizeButton`, `SortButton`, `FilterButton`).

**Existing forwardRef'd trigger components:** `SizeButton`, `SortButton`, `FilterButton`. Follow the same shape if you add another.

## Key Files

| Purpose | Location |
|---------|----------|
| Router | `app/router.tsx` |
| API client | `app/libraries/api.ts` |
| Query hooks | `app/hooks/queries/` |
| Page title hook | `app/hooks/usePageTitle.ts` |
| Unsaved changes hook | `app/hooks/useUnsavedChanges.ts` |
| Form dialog close hook | `app/hooks/useFormDialogClose.ts` |
| FormDialog component | `app/components/ui/form-dialog.tsx` |
| UnsavedChangesDialog | `app/components/ui/unsaved-changes-dialog.tsx` |
| Generated types | `app/types/generated/` |
| Components | `app/components/` |
| Pages | `app/components/pages/` |
| Vitest config | `vitest.config.ts` |
| Playwright config | `playwright.config.ts` |
| Unit/component tests | `app/**/*.test.{ts,tsx}` |
| E2E tests | `e2e/*.spec.ts` |
