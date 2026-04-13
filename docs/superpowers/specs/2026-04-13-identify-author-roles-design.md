# Identify Dialog: Author Role Display

## Problem

When a book has the same person assigned to multiple author roles (e.g., *Gotouge Koyoharu* as both Writer and Penciller), the identify dialog renders them as visually identical chips or comma-separated names, making the list look like duplicate entries. This surfaces in three places:

1. **Identify search result row** — `"Gotouge Koyoharu, Gotouge Koyoharu"` in the results list.
2. **Identify review form "Use current" summary** — same string in the muted current-value bar above the Authors field.
3. **Identify review form editable chips** — `[Gotouge Koyoharu ×] [Gotouge Koyoharu ×]`, with no way to tell which is which.

The book details page already handles this cleanly by appending a muted role label: `Gotouge Koyoharu (Writer)` / `Gotouge Koyoharu (Penciller)`. The identify flow should match.

## Scope

Display-only. Users still cannot assign or edit author roles from the identify dialog — role editing remains the responsibility of the book edit dialog. Adding a new author via the identify form continues to create an entry with no role, same as today.

## Non-goals

- Role editing UI inside the identify dialog.
- Extracting a fully reusable `<AuthorChip>` component. Display contexts differ (plain text vs. editable chip) and there is only one editable site, so a shared component would be over-abstraction.
- Any backend changes. Role data already flows through `AuthorEntry { name, role }` in `IdentifyReviewForm.tsx` and through `result.authors[].role` from plugin results — it is simply discarded at render time.

## Design

### 1. Shared role label utility

Create `app/utils/authorRoles.ts` exporting:

```ts
export function getAuthorRoleLabel(role: string | undefined | null): string | null;
```

Move the existing `getRoleLabel` implementation from `BookDetail.tsx` (currently lines 122–135) into this module, renamed to `getAuthorRoleLabel` to avoid collision with the existing user-permission-role helpers in `app/utils/roles.ts`. The function maps canonical role strings (`writer`, `penciller`, `inker`, `colorist`, `letterer`, `cover_artist`, `editor`, `translator`) to their capitalized display form, falls back to the raw role string for unknown values, and returns `null` for empty input.

Update `BookDetail.tsx` to import from the new module and delete the inline copy.

### 2. Identify review form — editable author chips

In `IdentifyReviewForm.tsx`, replace the generic `TagInput` currently used for the Authors field (around lines 802–814) with a new inline `AuthorTagInput` component, defined in the same file alongside `TagInput`. It takes the role-aware shape directly:

```ts
function AuthorTagInput({
  authors,
  onChange,
  disabled,
  placeholder,
}: {
  authors: AuthorEntry[];
  onChange: (authors: AuthorEntry[]) => void;
  disabled?: boolean;
  placeholder?: string;
}) { ... }
```

Behavior:

- Each chip renders the name plus, when `role` is set, a muted suffix `<span className="text-muted-foreground">({roleLabel})</span>`, matching BookDetail's visual treatment.
- Chips are keyed by index and removed by index, so same-name-different-role entries can be removed independently. This fixes a latent bug in the current `onChange` handler, which uses `authors.find((a) => a.name === name)` to preserve roles and cannot distinguish duplicates.
- The text input at the end still accepts new names on Enter and appends `{ name, role: undefined }`. Backspace on empty input removes the last entry.
- Disabled state is handled the same way as `TagInput`.

The existing `TagInput` component stays in place for the other fields (narrators, genres, tags) unchanged.

### 3. Identify review form — "Use current" summary

Update the `currentValue` prop passed to `FieldWrapper` for the Authors field (currently around line 794). Build the display string from `currentAuthors` by appending the role label suffix when present:

```
Gotouge Koyoharu (Writer), Gotouge Koyoharu (Penciller)
```

The comma-join remains; only each name gains an optional ` (Role)` suffix.

### 4. Identify search result row

In `IdentifyBookDialog.tsx`, update `resolveAuthors` (lines 154–158) so it returns a single formatted string (or `undefined`) instead of a `string[]`:

```ts
const resolveAuthors = (result: PluginSearchResult): string | undefined => {
  if (!result.authors || result.authors.length === 0) return undefined;
  return result.authors
    .map((a) => {
      const label = getAuthorRoleLabel(a.role);
      return label ? `${a.name} (${label})` : a.name;
    })
    .join(", ");
};
```

The one call site (around line 449) already renders the value inside a `<p>` — update it from `{authors.join(", ")}` to `{authors}`. The existing `hasAuthors = authors && authors.length > 0` check works unchanged for the new string shape.

## Testing

- **Unit test** for `getAuthorRoleLabel` in `app/utils/authorRoles.test.ts` covering: known roles, unknown role (falls through), `undefined`, and `null`.
- **No new component tests** for `IdentifyReviewForm` or `IdentifyBookDialog` — neither has component tests today, and the display logic is narrow enough that a unit test on the pure label helper plus manual verification covers the risk.
- Manual verification: load a CBZ book with duplicate-named authors in different roles and confirm all three sites render the role suffix.

## Docs

No documentation changes. This is purely visual polish of an existing feature; no config, API, or user-facing behavior is added or changed.

## Files touched

| File | Change |
|------|--------|
| `app/utils/authorRoles.ts` | New — `getAuthorRoleLabel` helper |
| `app/utils/authorRoles.test.ts` | New — unit test |
| `app/components/pages/BookDetail.tsx` | Replace inline `getRoleLabel` with `getAuthorRoleLabel` import |
| `app/components/library/IdentifyReviewForm.tsx` | Add `AuthorTagInput`, update Authors field wiring, update `currentValue` summary |
| `app/components/library/IdentifyBookDialog.tsx` | Update `resolveAuthors` return type + call site |
