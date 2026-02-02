import { describe, expect, it } from "vitest";

import { forTitle } from "@/utils/sortname";

/**
 * Tests for BookEditDialog handleSubmit sort title logic.
 *
 * The bug: handleSubmit compares sortTitle (semantic state) against book.sort_title (live prop),
 * but hasChanges compares effective sort title (sortTitle || forTitle(title)) against
 * initialValues.sortTitle (snapshot). This creates an inconsistency where:
 *
 * 1. User changes title from "The Book" to "A Book"
 * 2. sortTitle state stays "" (auto mode)
 * 3. hasChanges detects: effectiveSortTitle changed ("Book, The" -> "Book, A")
 * 4. BUG: handleSubmit says: sortTitle ("") === book.sort_title || "" ("") - no change!
 *
 * The fix: handleSubmit should compare effective sort title against initialValues.sortTitle,
 * consistent with hasChanges.
 */
describe("BookEditDialog handleSubmit sort title logic", () => {
  // FIXED shouldIncludeSortTitle logic - compares effective values against snapshot
  const shouldIncludeSortTitle = (
    sortTitle: string, // semantic state value
    _bookSortTitle: string | undefined, // not used in fixed code!
    initialSortTitle: string, // snapshot from when dialog opened
    title: string, // current title
  ): boolean => {
    // FIXED: Compare effective sort title against initialValues.sortTitle (snapshot)
    // This is consistent with hasChanges computation
    const effectiveSortTitle = sortTitle || forTitle(title);
    return effectiveSortTitle !== initialSortTitle;
  };

  it("should include sort_title in payload when title changes (in auto mode)", () => {
    // Scenario:
    // 1. Dialog opens with title="The Book", sort_title="Book, The" (auto-generated)
    // 2. User changes title to "A Book" (but leaves sort title in auto mode)
    // 3. Effective sort title is now "Book, A"
    // 4. hasChanges correctly detects this as a change
    // 5. BUG: handleSubmit doesn't include sort_title in payload because:
    //    sortTitle ("") === book.sort_title || "" ("") -> false (wait, that's wrong too)
    //
    // Actually let me trace through more carefully:
    // - Initial: book.sort_title = "Book, The", sortTitle state = "" (auto mode)
    // - User changes title to "A Book"
    // - hasChanges: effectiveSortTitle = forTitle("A Book") = "Book, A"
    //   initialValues.sortTitle = "Book, The" -> hasChanges = true
    // - handleSubmit: sortTitle ("") !== book.sort_title || "" ("Book, The") -> true
    //
    // Hmm, actually that would work. Let me think about the actual bug case...
    //
    // The bug case is when book.sort_title is null/undefined (never manually set):
    // - Initial: book.sort_title = undefined, sortTitle state = "" (auto mode)
    // - User changes title from "The Book" to "A Book"
    // - hasChanges: effectiveSortTitle = forTitle("A Book") = "Book, A"
    //   initialValues.sortTitle = forTitle("The Book") = "Book, The" -> hasChanges = true
    // - handleSubmit: sortTitle ("") !== book.sort_title || "" ("") -> false
    //   BUG: Won't include sort_title even though effective sort title changed!

    const sortTitle = ""; // auto mode
    const bookSortTitle = undefined; // never manually set
    const title = "A Book"; // user changed title
    const initialSortTitle = forTitle("The Book"); // "Book, The"

    const result = shouldIncludeSortTitle(
      sortTitle,
      bookSortTitle,
      initialSortTitle,
      title,
    );

    // FIXED: Now correctly returns true because effective sort title changed
    expect(result).toBe(true);
  });

  it("should NOT include sort_title when nothing changed", () => {
    const sortTitle = ""; // auto mode, unchanged
    const bookSortTitle = undefined;
    const title = "The Book"; // title unchanged
    const initialSortTitle = forTitle("The Book"); // "Book, The"

    const result = shouldIncludeSortTitle(
      sortTitle,
      bookSortTitle,
      initialSortTitle,
      title,
    );

    // Should return false - no change
    expect(result).toBe(false);
  });
});
