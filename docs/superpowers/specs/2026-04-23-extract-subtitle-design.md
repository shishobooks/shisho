# Extract Subtitle from Title

## Problem

Book metadata often combines a book's subtitle into the title with a colon — e.g. `"Why We Sleep: Unlocking the Power of Sleep and Dreams"`. When this happens, editing the book manually is tedious: copy the part after the colon, delete it (and the colon) from the title, click into the subtitle field, paste.

This isn't something we can automate unconditionally — plenty of legitimate titles contain colons, for example series titling like `"Star Wars: Dark Force Rising"`. But because the combined-title case is common, a one-click "Extract subtitle" affordance on the edit form is a big quality-of-life improvement.

## Goals

- Give users a one-click way to split a combined `"Title: Subtitle"` string into the separate title and subtitle fields while editing a book.
- Make the affordance consistent across both places where a user can edit these fields: the book edit dialog and the identify-review form.
- Keep the change narrow — no new config, no API change, no schema change.

## Non-goals

- **No automatic splitting.** The split must remain an explicit user action.
- **No scan-time or identify-time auto-splitting.** This is strictly a UI helper for manual editing.
- **No heuristics for detecting "is this a series prefix vs. a subtitle?"** — the user decides by clicking (or not).

## UX

### Visibility

The "Extract subtitle" control is rendered **only** when the current title string contains a `:` with non-empty content on both sides (so `":foo"`, `"foo:"`, `":"` alone don't show the button). If the user clicks it, the title no longer has a colon, so the button disappears.

### Placement

Below the title input, right-aligned, muted link-style text. Matches the existing "Clear all" affordance pattern at `app/components/library/BookEditDialog.tsx:542`:

```
text-xs text-muted-foreground hover:text-foreground cursor-pointer
```

Placed below (not above) because in `IdentifyReviewForm`, the top-right of the title row is occupied by the "Changed" status badge and the row's right side is occupied by the "Use current" action. Below-right is the only location consistent across both forms.

Visual sketch:

```
Title
┌──────────────────────────────────────┐
│ Why We Sleep: Unlocking the Power... │
└──────────────────────────────────────┘
                       Extract subtitle
```

### Click behavior

- Split on **first** `:`.
- Trim whitespace on both sides.
- Set the title field to the left side.
- Set the subtitle field to the right side, **overwriting** any existing subtitle value.

Overwriting silently (rather than confirming or hiding when subtitle is non-empty) is fine because:

1. The form shows both fields on screen — the user immediately sees the result.
2. Both forms have unsaved-changes protection — a wrong click can be undone by Cancel.
3. A confirmation step adds friction to what should be a one-click action.

### Multiple colons

The split always happens on the **first** colon. Any remaining colons stay in the subtitle. This is almost always correct:

- `"Atomic Habits: An Easy & Proven Way to Build Good Habits: Tiny Changes, Remarkable Results"` → title `"Atomic Habits"`, subtitle `"An Easy & Proven Way… : Tiny Changes…"`
- `"Star Wars: Thrawn: Alliances"` → title `"Star Wars"`, subtitle `"Thrawn: Alliances"`

If the user disagrees with the split, they can edit further manually or hit Cancel.

## Implementation

### 1. Shared split helper

New file `app/utils/extractSubtitle.ts`:

```ts
export function extractSubtitleFromTitle(
  title: string,
): { title: string; subtitle: string } | null {
  const idx = title.indexOf(":");
  if (idx === -1) return null;
  const newTitle = title.slice(0, idx).trim();
  const newSubtitle = title.slice(idx + 1).trim();
  if (!newTitle || !newSubtitle) return null;
  return { title: newTitle, subtitle: newSubtitle };
}
```

Returning `null` when the split would produce an empty side (leading `:`, trailing `:`, or a bare `:`) doubles as the "should we render the button?" check — the consuming component can render iff the helper returns non-null.

### 2. Shared button component

New file `app/components/library/ExtractSubtitleButton.tsx`:

```tsx
interface Props {
  title: string;
  onExtract: (title: string, subtitle: string) => void;
}

export function ExtractSubtitleButton({ title, onExtract }: Props) {
  const split = extractSubtitleFromTitle(title);
  if (!split) return null;
  return (
    <div className="flex justify-end">
      <button
        type="button"
        className="text-xs text-muted-foreground hover:text-foreground cursor-pointer"
        onClick={() => onExtract(split.title, split.subtitle)}
      >
        Extract subtitle
      </button>
    </div>
  );
}
```

### 3. BookEditDialog integration

In `app/components/library/BookEditDialog.tsx`, inside the Title `<div className="space-y-2">` block (around lines 491-499), append the button after the `<Input>`:

```tsx
<ExtractSubtitleButton
  title={title}
  onExtract={(t, s) => {
    setTitle(t);
    setSubtitle(s);
  }}
/>
```

### 4. IdentifyReviewForm integration

In `app/components/library/IdentifyReviewForm.tsx`, after the title `FieldWrapper` block (around lines 856-869), add the same button. The form already has `setTitle` and `setSubtitle` state setters at lines 570-571.

The button sits between the Title FieldWrapper and the Subtitle FieldWrapper. `isDisabled("title")` should short-circuit rendering — if title editing is disabled for this field, the button should not appear. The `ExtractSubtitleButton` component itself doesn't need to know about this; the caller passes the button only when editing is allowed, e.g.:

```tsx
{!isDisabled("title") && !isDisabled("subtitle") && (
  <ExtractSubtitleButton
    title={title}
    onExtract={(t, s) => {
      setTitle(t);
      setSubtitle(s);
    }}
  />
)}
```

## Testing

### Unit tests for `extractSubtitleFromTitle`

`app/utils/extractSubtitle.test.ts`:

- No colon → returns `null`.
- Leading colon (`":foo"`) → returns `null`.
- Trailing colon (`"foo:"`) → returns `null`.
- Bare colon (`":"`) → returns `null`.
- Simple split (`"A: B"`) → `{ title: "A", subtitle: "B" }`.
- Surrounding whitespace (`"  Foo  :  Bar  "`) → `{ title: "Foo", subtitle: "Bar" }`.
- Multiple colons (`"Foo: Bar: Baz"`) → `{ title: "Foo", subtitle: "Bar: Baz" }` (first colon wins).
- Empty string → returns `null`.

### Component tests for `ExtractSubtitleButton`

`app/components/library/ExtractSubtitleButton.test.tsx`:

- Renders nothing when `title` has no colon.
- Renders button when `title` has a splittable colon.
- Fires `onExtract` with the correct split values on click.

## Docs

No user-facing docs need updating. This is a small UI affordance — no new config option, no API change, no change to user-facing metadata semantics, no change in scanner or identify-flow behavior.

## Out of scope / follow-ups

- Applying the same logic automatically at scan-time or identify-time. Intentionally out of scope — we don't want to risk wrongly splitting a series-prefix title like `"Star Wars: Dark Force Rising"`.
- A similar affordance elsewhere (file-level metadata edit, bulk edit, etc.) — can be added later if needed.
