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

## Known Radix UI Issues

### Dialog + DropdownMenu pointer-events Bug

**Problem:** When a Dialog is triggered from a DropdownMenu item, Radix's DismissableLayer incorrectly sets `pointer-events: none` on the body during unmount, leaving the page unclickable after the dialog closes.

**Solution:** Already fixed globally in `app/components/ui/dialog.tsx`. The custom `Dialog` wrapper includes a cleanup effect that clears `pointer-events` after a 300ms delay, ensuring it runs after Radix's buggy unmount effects complete.

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
