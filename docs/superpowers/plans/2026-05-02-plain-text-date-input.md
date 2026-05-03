# Plain-Text Date Input Site-Wide — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the calendar-based `<DatePicker>` with a plain text `<Input>` for release dates everywhere, matching the pattern already shipped in `IdentifyReviewForm`. Then remove the now-unused `DatePicker`, `Calendar`, and `react-day-picker` dependency.

**Architecture:** The `DatePicker` component (Radix Popover + `react-day-picker` calendar) is used in exactly one place: `FileEditDialog.tsx` for the Release Date field. `IdentifyReviewForm.tsx` already uses a plain `<Input placeholder="YYYY-MM-DD">` for the same field. This plan swaps the FileEditDialog usage, then deletes the dead components and dependency. The backend `dateValidator` regex already accepts `YYYY-MM-DD` strings, so no backend changes are needed.

**Tech Stack:** React, TypeScript, Vitest, pnpm

---

## File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `app/components/library/FileEditDialog.tsx` | Swap `DatePicker` for `Input` on release date |
| Modify | `app/components/library/FileEditDialog.test.tsx` | Add tests for plain-text date input |
| Delete | `app/components/ui/date-picker.tsx` | Remove unused calendar date picker |
| Delete | `app/components/ui/calendar.tsx` | Remove unused calendar wrapper |
| Modify | `app/index.css` | Remove `react-day-picker` import and `.rdp-*` theme styles |
| Modify | `package.json` | Remove `react-day-picker` dependency |

---

### Task 1: Add test for plain-text date input in FileEditDialog

**Files:**
- Modify: `app/components/library/FileEditDialog.test.tsx`

This task adds a test that verifies the release date field is a plain text input with the correct placeholder (`YYYY-MM-DD`), accepts free-form text, and includes the value in the save payload. The test is written against the current `DatePicker` implementation so it fails first (Red step of TDD).

- [ ] **Step 1: Write the failing test**

Add the following test block inside the existing `describe("FileEditDialog")` block in `app/components/library/FileEditDialog.test.tsx`, just before the closing `});` of the top-level describe. The tests use the existing `mockFile`, `renderDialog`, `createUser`, and `mockUpdateFile` fixtures already defined in the file.

```tsx
  describe("release date plain-text input", () => {
    it("renders a text input with YYYY-MM-DD placeholder", async () => {
      renderDialog();

      const input = screen.getByPlaceholderText("YYYY-MM-DD");
      expect(input).toBeInTheDocument();
      expect(input.tagName).toBe("INPUT");
    });

    it("submits typed date value in payload", async () => {
      const user = createUser();
      renderDialog();

      const input = screen.getByPlaceholderText("YYYY-MM-DD");
      await user.clear(input);
      await user.type(input, "1847-10-16");

      const saveButton = screen.getByRole("button", { name: /save/i });
      await user.click(saveButton);

      await waitFor(() => {
        expect(mockUpdateFile).toHaveBeenCalled();
      });
      const payload = mockUpdateFile.mock.calls[0][0];
      expect(payload.release_date).toBe("1847-10-16");
    });

    it("allows clearing the date field", async () => {
      const user = createUser();
      const fileWithDate: File = {
        ...mockFile,
        release_date: "2020-06-15T00:00:00Z",
      };
      const queryClient = createQueryClient();

      render(
        <QueryClientProvider client={queryClient}>
          <FileEditDialog
            file={fileWithDate}
            onOpenChange={vi.fn()}
            open={true}
          />
        </QueryClientProvider>,
      );

      const input = screen.getByPlaceholderText("YYYY-MM-DD");
      expect(input).toHaveValue("2020-06-15");

      await user.clear(input);

      const saveButton = screen.getByRole("button", { name: /save/i });
      await user.click(saveButton);

      await waitFor(() => {
        expect(mockUpdateFile).toHaveBeenCalled();
      });
      const payload = mockUpdateFile.mock.calls[0][0];
      expect(payload.release_date).toBe("");
    });
  });
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/robinjoseph/.worktrees/shisho/second-identify && pnpm vitest run app/components/library/FileEditDialog.test.tsx`

Expected: FAIL — the `DatePicker` renders a `<button>` trigger, not an `<input>` with placeholder `YYYY-MM-DD`. The `getByPlaceholderText("YYYY-MM-DD")` call will throw.

- [ ] **Step 3: Commit**

```bash
git add app/components/library/FileEditDialog.test.tsx
git commit -m "[Test] Add failing tests for plain-text date input in FileEditDialog"
```

---

### Task 2: Swap DatePicker for plain-text Input in FileEditDialog

**Files:**
- Modify: `app/components/library/FileEditDialog.tsx:14-15,70-79,124-126,424-428,949-957`

- [ ] **Step 1: Replace DatePicker with Input**

In `app/components/library/FileEditDialog.tsx`, remove the `DatePicker` import (line 14) and replace the Release Date section (lines 949-957).

Remove this import:

```tsx
import { DatePicker } from "@/components/ui/date-picker";
```

Replace the Release Date block (lines 949-957):

```tsx
              {/* Release Date */}
              <div className="space-y-2">
                <Label>Release Date</Label>
                <DatePicker
                  onChange={setReleaseDate}
                  placeholder="Pick a date"
                  value={releaseDate}
                />
              </div>
```

With:

```tsx
              {/* Release Date */}
              <div className="space-y-2">
                <Label>Release Date</Label>
                <Input
                  onChange={(e) => setReleaseDate(e.target.value)}
                  placeholder="YYYY-MM-DD"
                  value={releaseDate}
                />
              </div>
```

`Input` is already imported at line 24. No new imports needed.

- [ ] **Step 2: Remove the `formatDateForInput` helper**

The `formatDateForInput` function (lines 70-79) parses a date string via `new Date()` and extracts `YYYY-MM-DD`. With a plain text input, we still need this same behavior to initialize state from the ISO 8601 string the server returns (e.g., `"2020-06-15T00:00:00Z"` → `"2020-06-15"`). **Keep the function as-is.** No changes needed here.

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd /Users/robinjoseph/.worktrees/shisho/second-identify && pnpm vitest run app/components/library/FileEditDialog.test.tsx`

Expected: All three new tests PASS. The existing tests should also pass since the `Input` component's `onChange` handler produces the same `string` values that `DatePicker.onChange` did.

- [ ] **Step 4: Commit**

```bash
git add app/components/library/FileEditDialog.tsx
git commit -m "[Feature] Replace DatePicker with plain-text date input in FileEditDialog"
```

---

### Task 3: Delete DatePicker, Calendar, and react-day-picker

**Files:**
- Delete: `app/components/ui/date-picker.tsx`
- Delete: `app/components/ui/calendar.tsx`
- Modify: `app/index.css:3,91-188`
- Modify: `package.json:53,59` (dependency lines)

Now that `DatePicker` has zero importers, remove the dead code and its dependency.

- [ ] **Step 1: Verify no remaining imports**

Run: `grep -rn "DatePicker\|date-picker\|Calendar.*calendar" app/ --include="*.tsx" --include="*.ts" | grep -v node_modules | grep -v "\.test\."`

Expected: Zero results (the only importer was `FileEditDialog.tsx`, which no longer imports it).

- [ ] **Step 2: Delete the component files**

```bash
rm app/components/ui/date-picker.tsx app/components/ui/calendar.tsx
```

- [ ] **Step 3: Remove react-day-picker CSS from index.css**

In `app/index.css`, remove the `react-day-picker` import on line 3:

```css
@import "react-day-picker/src/style.css";
```

And remove the entire `.rdp-*` theme customization block (lines 91-188):

```css
/* React Day Picker theme customization */
.rdp-root {
  --rdp-accent-color: var(--primary);
```

...through...

```css
.rdp-chevron {
  fill: var(--muted-foreground);
}
```

- [ ] **Step 4: Remove react-day-picker from package.json**

```bash
cd /Users/robinjoseph/.worktrees/shisho/second-identify && pnpm remove react-day-picker
```

This removes it from `package.json` and updates `pnpm-lock.yaml`.

Note: **Do NOT remove `date-fns`** — it's still used by `ResyncButton.tsx`, `AdminJobs.tsx`, `JobDetail.tsx`, and `ListDetail.tsx` for `formatDistanceToNow` / `format`.

- [ ] **Step 5: Verify build and tests**

Run: `cd /Users/robinjoseph/.worktrees/shisho/second-identify && pnpm build && pnpm vitest run app/components/library/FileEditDialog.test.tsx`

Expected: Build succeeds with no errors. Tests pass.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "[Chore] Remove DatePicker, Calendar, and react-day-picker dependency"
```

---

### Task 4: Run full validation

- [ ] **Step 1: Run lint and type checks**

```bash
cd /Users/robinjoseph/.worktrees/shisho/second-identify && mise lint:js
```

Expected: No errors. The removed files have no remaining references.

- [ ] **Step 2: Run unit tests**

```bash
cd /Users/robinjoseph/.worktrees/shisho/second-identify && mise test:unit
```

Expected: All tests pass including the new FileEditDialog date tests.

- [ ] **Step 3: Visually verify in browser**

Start `mise start`, open the app at localhost, navigate to a book detail page, click a file's edit button, and verify:

1. The Release Date field renders as a plain text input (not a calendar button).
2. The placeholder reads `YYYY-MM-DD`.
3. Typing a date like `1847-10-16` works.
4. Clearing the field works.
5. Saving persists the value — re-opening the dialog shows the saved date.
6. No console errors.
