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

## Key Files

| Purpose | Location |
|---------|----------|
| Router | `app/router.tsx` |
| API client | `app/libraries/api.ts` |
| Query hooks | `app/hooks/queries/` |
| Generated types | `app/types/generated/` |
| Components | `app/components/` |
| Pages | `app/components/pages/` |
