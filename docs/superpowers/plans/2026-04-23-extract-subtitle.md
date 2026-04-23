# Extract Subtitle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a one-click "Extract subtitle" affordance to the book edit and identify-review forms that splits a combined `"Title: Subtitle"` string into separate fields.

**Architecture:** Introduce a pure helper (`extractSubtitleFromTitle`) in `app/utils/`, a thin presentational component (`ExtractSubtitleButton`) in `app/components/library/`, and wire that component into both `BookEditDialog` and `IdentifyReviewForm`. Render-gate inside the component so callers don't have to check the "has splittable colon" condition themselves.

**Tech Stack:** React 19, TypeScript, TailwindCSS, Vitest, React Testing Library.

**Spec:** `docs/superpowers/specs/2026-04-23-extract-subtitle-design.md`

## File Structure

- **Create** `app/utils/extractSubtitle.ts` — pure helper. Splits title on first `:`, trims both sides, returns `null` when the split would produce an empty side.
- **Create** `app/utils/extractSubtitle.test.ts` — unit tests for the helper.
- **Create** `app/components/library/ExtractSubtitleButton.tsx` — presentational button. Calls the helper; renders `null` when nothing to extract; fires callback on click.
- **Create** `app/components/library/ExtractSubtitleButton.test.tsx` — component tests.
- **Modify** `app/components/library/BookEditDialog.tsx` — add the button below the title `<Input>` (inside the Title `<div className="space-y-2">` block around lines 491-499).
- **Modify** `app/components/library/IdentifyReviewForm.tsx` — add the button between the title `FieldWrapper` and the subtitle `FieldWrapper` (around lines 856-869). Guard on `!isDisabled("title") && !isDisabled("subtitle")`.

---

### Task 1: Helper — tests first

**Files:**
- Create: `app/utils/extractSubtitle.test.ts`

- [ ] **Step 1: Write the failing tests**

```ts
import { describe, expect, it } from "vitest";

import { extractSubtitleFromTitle } from "./extractSubtitle";

describe("extractSubtitleFromTitle", () => {
  it("returns null when title has no colon", () => {
    expect(extractSubtitleFromTitle("Why We Sleep")).toBeNull();
  });

  it("returns null for empty string", () => {
    expect(extractSubtitleFromTitle("")).toBeNull();
  });

  it("returns null when colon is leading", () => {
    expect(extractSubtitleFromTitle(": Subtitle")).toBeNull();
  });

  it("returns null when colon is trailing", () => {
    expect(extractSubtitleFromTitle("Title:")).toBeNull();
  });

  it("returns null for a bare colon", () => {
    expect(extractSubtitleFromTitle(":")).toBeNull();
  });

  it("returns null when only whitespace on one side", () => {
    expect(extractSubtitleFromTitle("   : Subtitle")).toBeNull();
    expect(extractSubtitleFromTitle("Title :   ")).toBeNull();
  });

  it("splits on the first colon and trims both sides", () => {
    expect(
      extractSubtitleFromTitle(
        "Why We Sleep: Unlocking the Power of Sleep and Dreams",
      ),
    ).toEqual({
      title: "Why We Sleep",
      subtitle: "Unlocking the Power of Sleep and Dreams",
    });
  });

  it("trims surrounding whitespace", () => {
    expect(extractSubtitleFromTitle("  Foo  :  Bar  ")).toEqual({
      title: "Foo",
      subtitle: "Bar",
    });
  });

  it("preserves additional colons in the subtitle (splits on first only)", () => {
    expect(extractSubtitleFromTitle("Star Wars: Thrawn: Alliances")).toEqual({
      title: "Star Wars",
      subtitle: "Thrawn: Alliances",
    });
  });
});
```

- [ ] **Step 2: Run the tests and verify they fail**

Run:

```bash
pnpm vitest run app/utils/extractSubtitle.test.ts
```

Expected: all tests fail because `./extractSubtitle` does not yet exist (module-not-found error).

- [ ] **Step 3: Commit the failing tests**

```bash
git add app/utils/extractSubtitle.test.ts
git commit -m "[Test] Add failing tests for extractSubtitleFromTitle"
```

---

### Task 2: Helper — implementation

**Files:**
- Create: `app/utils/extractSubtitle.ts`

- [ ] **Step 1: Implement the helper**

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

- [ ] **Step 2: Run the tests and verify they pass**

Run:

```bash
pnpm vitest run app/utils/extractSubtitle.test.ts
```

Expected: all 9 tests pass.

- [ ] **Step 3: Commit the implementation**

```bash
git add app/utils/extractSubtitle.ts
git commit -m "[Frontend] Add extractSubtitleFromTitle helper"
```

---

### Task 3: Button component — tests first

**Files:**
- Create: `app/components/library/ExtractSubtitleButton.test.tsx`

- [ ] **Step 1: Write the failing component tests**

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ExtractSubtitleButton } from "./ExtractSubtitleButton";

describe("ExtractSubtitleButton", () => {
  it("renders nothing when title has no colon", () => {
    const onExtract = vi.fn();
    const { container } = render(
      <ExtractSubtitleButton onExtract={onExtract} title="Why We Sleep" />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when title has colon but split is empty on one side", () => {
    const onExtract = vi.fn();
    const { container } = render(
      <ExtractSubtitleButton onExtract={onExtract} title="Title:" />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders button when title has a splittable colon", () => {
    const onExtract = vi.fn();
    render(
      <ExtractSubtitleButton
        onExtract={onExtract}
        title="Why We Sleep: Unlocking the Power of Sleep and Dreams"
      />,
    );
    expect(
      screen.getByRole("button", { name: /extract subtitle/i }),
    ).toBeInTheDocument();
  });

  it("fires onExtract with the split values on click", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onExtract = vi.fn();
    render(
      <ExtractSubtitleButton
        onExtract={onExtract}
        title="Star Wars: Thrawn: Alliances"
      />,
    );
    await user.click(
      screen.getByRole("button", { name: /extract subtitle/i }),
    );
    expect(onExtract).toHaveBeenCalledWith("Star Wars", "Thrawn: Alliances");
  });
});
```

- [ ] **Step 2: Run the tests and verify they fail**

Run:

```bash
pnpm vitest run app/components/library/ExtractSubtitleButton.test.tsx
```

Expected: all tests fail because `./ExtractSubtitleButton` does not yet exist (module-not-found error).

- [ ] **Step 3: Commit the failing tests**

```bash
git add app/components/library/ExtractSubtitleButton.test.tsx
git commit -m "[Test] Add failing tests for ExtractSubtitleButton"
```

---

### Task 4: Button component — implementation

**Files:**
- Create: `app/components/library/ExtractSubtitleButton.tsx`

- [ ] **Step 1: Implement the component**

```tsx
import { extractSubtitleFromTitle } from "@/utils/extractSubtitle";

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
        className="text-xs text-muted-foreground hover:text-foreground cursor-pointer"
        onClick={() => onExtract(split.title, split.subtitle)}
        type="button"
      >
        Extract subtitle
      </button>
    </div>
  );
}
```

- [ ] **Step 2: Run the component tests and verify they pass**

Run:

```bash
pnpm vitest run app/components/library/ExtractSubtitleButton.test.tsx
```

Expected: all 4 tests pass.

- [ ] **Step 3: Commit the implementation**

```bash
git add app/components/library/ExtractSubtitleButton.tsx
git commit -m "[Frontend] Add ExtractSubtitleButton component"
```

---

### Task 5: Wire into BookEditDialog

**Files:**
- Modify: `app/components/library/BookEditDialog.tsx` (Title block around lines 491-499)

- [ ] **Step 1: Add the import**

In the existing `@/components/...` import block near the top of the file (alongside the other `@/components/library/...` imports if present, otherwise in alpha order), add:

```tsx
import { ExtractSubtitleButton } from "@/components/library/ExtractSubtitleButton";
```

- [ ] **Step 2: Render the button below the Title `<Input>`**

Locate the Title block (currently lines 491-499):

```tsx
{/* Title */}
<div className="space-y-2">
  <Label htmlFor="title">Title</Label>
  <Input
    id="title"
    onChange={(e) => setTitle(e.target.value)}
    value={title}
  />
</div>
```

Replace with:

```tsx
{/* Title */}
<div className="space-y-2">
  <Label htmlFor="title">Title</Label>
  <Input
    id="title"
    onChange={(e) => setTitle(e.target.value)}
    value={title}
  />
  <ExtractSubtitleButton
    onExtract={(t, s) => {
      setTitle(t);
      setSubtitle(s);
    }}
    title={title}
  />
</div>
```

- [ ] **Step 3: Manually verify the dev server**

Run `mise start` (or confirm it's already running), open a book in the library, click Edit, and enter a title like `Why We Sleep: Unlocking the Power of Sleep and Dreams`. Confirm:

- The "Extract subtitle" text appears below the title input, right-aligned, muted.
- Clicking it changes Title to `Why We Sleep` and Subtitle to `Unlocking the Power of Sleep and Dreams`.
- After the click, the button disappears (title no longer has `:`).
- Clearing the title or entering one without `:` keeps the button hidden.
- Clicking Save persists the change; clicking Cancel (via the Unsaved Changes dialog) rolls it back.

- [ ] **Step 4: Run the frontend lint and typecheck**

Run:

```bash
pnpm lint:eslint && pnpm lint:types
```

Expected: clean exit.

- [ ] **Step 5: Commit**

```bash
git add app/components/library/BookEditDialog.tsx
git commit -m "[Frontend] Wire ExtractSubtitleButton into BookEditDialog"
```

---

### Task 6: Wire into IdentifyReviewForm

**Files:**
- Modify: `app/components/library/IdentifyReviewForm.tsx` (Title/Subtitle blocks around lines 856-884)

- [ ] **Step 1: Add the import**

Add to the import block at the top of the file (alpha order within `@/components/library/...` imports):

```tsx
import { ExtractSubtitleButton } from "@/components/library/ExtractSubtitleButton";
```

- [ ] **Step 2: Render the button between the Title and Subtitle `FieldWrapper` blocks**

Locate the Title and Subtitle blocks (currently lines 856-884):

```tsx
{/* Title */}
<FieldWrapper
  currentValue={book.title || undefined}
  disabled={isDisabled("title")}
  field="title"
  onUseCurrent={() => setTitle(book.title)}
  status={defaults.title.status}
>
  <Input
    disabled={isDisabled("title")}
    onChange={(e) => setTitle(e.target.value)}
    value={title}
  />
</FieldWrapper>

{/* Subtitle */}
<FieldWrapper
  currentValue={book.subtitle || undefined}
  disabled={isDisabled("subtitle")}
  field="subtitle"
  onUseCurrent={() => setSubtitle(book.subtitle ?? "")}
  status={defaults.subtitle.status}
>
  <Input
    disabled={isDisabled("subtitle")}
    onChange={(e) => setSubtitle(e.target.value)}
    value={subtitle}
  />
</FieldWrapper>
```

Insert the button between the two `FieldWrapper` blocks so the rendered output becomes:

```tsx
{/* Title */}
<FieldWrapper
  currentValue={book.title || undefined}
  disabled={isDisabled("title")}
  field="title"
  onUseCurrent={() => setTitle(book.title)}
  status={defaults.title.status}
>
  <Input
    disabled={isDisabled("title")}
    onChange={(e) => setTitle(e.target.value)}
    value={title}
  />
</FieldWrapper>

{!isDisabled("title") && !isDisabled("subtitle") && (
  <ExtractSubtitleButton
    onExtract={(t, s) => {
      setTitle(t);
      setSubtitle(s);
    }}
    title={title}
  />
)}

{/* Subtitle */}
<FieldWrapper
  currentValue={book.subtitle || undefined}
  disabled={isDisabled("subtitle")}
  field="subtitle"
  onUseCurrent={() => setSubtitle(book.subtitle ?? "")}
  status={defaults.subtitle.status}
>
  <Input
    disabled={isDisabled("subtitle")}
    onChange={(e) => setSubtitle(e.target.value)}
    value={subtitle}
  />
</FieldWrapper>
```

- [ ] **Step 3: Manually verify in the dev server**

With `mise start` running, trigger the Identify flow on a book. When a metadata provider returns a combined title (or if you edit the title field in the review form to include a `:`), confirm:

- "Extract subtitle" appears between the Title and Subtitle rows.
- Clicking it populates subtitle from the title's right-hand side and trims the title.
- The button hides after a successful extraction (no more `:`).
- If title editing is disabled (`isDisabled("title") === true`) or subtitle editing is disabled, the button is not rendered.

- [ ] **Step 4: Run the frontend lint and typecheck**

Run:

```bash
pnpm lint:eslint && pnpm lint:types
```

Expected: clean exit.

- [ ] **Step 5: Commit**

```bash
git add app/components/library/IdentifyReviewForm.tsx
git commit -m "[Frontend] Wire ExtractSubtitleButton into IdentifyReviewForm"
```

---

### Task 7: Final validation

**Files:**
- None (verification only)

- [ ] **Step 1: Run the full check suite**

Run:

```bash
mise check:quiet
```

Expected: one-line summary with all checks passing (Go tests, Go lint, JS tests, JS lint).

- [ ] **Step 2: If anything fails, fix and recommit**

For any failing step, read its output, fix the underlying cause, and create a new commit describing the fix. Do not proceed until `mise check:quiet` is green.

---

## Self-Review Summary

- **Spec coverage:**
  - Shared helper with render-gate logic → Task 1-2.
  - Shared button component with render-gate and callback → Task 3-4.
  - BookEditDialog integration → Task 5.
  - IdentifyReviewForm integration with `isDisabled` guards → Task 6.
  - Unit tests for helper → Task 1.
  - Component tests for button → Task 3.
  - Final check suite → Task 7.
  - Spec explicitly says no docs needed → not in plan, correct.
- **No placeholders** — all code steps show full code; all commands are exact.
- **Type consistency** — helper signature `{ title: string; subtitle: string } | null` is used identically in the button component and all test assertions. Component props name `title` / `onExtract` match across tests, implementation, and both consumer call sites.
