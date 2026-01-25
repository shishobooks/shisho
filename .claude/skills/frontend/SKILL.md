---
name: frontend
description: You MUST use this before working on any React/TypeScript frontend code or anything UI related. Covers Tanstack Query, API integration, component patterns, and UI conventions for Shisho.
user-invocable: false
---

# Shisho Frontend Development

This skill documents frontend patterns and conventions specific to Shisho.

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

## Testing

### Test Stack

| Level | Framework | Purpose |
|-------|-----------|---------|
| Unit + Component | Vitest + React Testing Library | Fast, native Vite integration |
| E2E | Playwright | Browser automation |

### Running Tests

```bash
yarn test           # Run all tests (unit + E2E via concurrently)
yarn test:unit      # Run Vitest unit/component tests with coverage
yarn test:e2e       # Run Playwright E2E tests
make test:js        # Run tests via Makefile (used in `make check`)
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

### E2E Tests

**See the `e2e-testing` skill for detailed E2E patterns**, including:
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

## Key Files

| Purpose | Location |
|---------|----------|
| Router | `app/router.tsx` |
| API client | `app/libraries/api.ts` |
| Query hooks | `app/hooks/queries/` |
| Generated types | `app/types/generated/` |
| Components | `app/components/` |
| Pages | `app/components/pages/` |
| Vitest config | `vitest.config.ts` |
| Playwright config | `playwright.config.ts` |
| Unit/component tests | `app/**/*.test.{ts,tsx}` |
| E2E tests | `e2e/*.spec.ts` |
