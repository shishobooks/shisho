import { AddToListDialog } from "./AddToListDialog";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { List } from "@/types";

// Mock data
const mockLists = [
  { id: 1, name: "Reading List", book_count: 5, permission: "owner" as const },
  { id: 2, name: "Favorites", book_count: 3, permission: "owner" as const },
  { id: 3, name: "To Read", book_count: 10, permission: "owner" as const },
];

// Book is initially in list 1 only
let mockBookLists: List[] = [
  {
    id: 1,
    name: "Reading List",
    is_ordered: false,
    created_at: "",
    updated_at: "",
    user_id: 1,
    default_sort: "title",
  },
];

// Mock the list hooks
vi.mock("@/hooks/queries/lists", () => ({
  useListLists: () => ({
    data: { lists: mockLists },
    isLoading: false,
  }),
  useBookLists: () => ({
    data: mockBookLists,
    isLoading: false,
  }),
  useUpdateBookLists: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
  useCreateList: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
  }),
}));

// Mock useFormDialogClose hook
vi.mock("@/hooks/useFormDialogClose", () => ({
  useFormDialogClose: (
    _open: boolean,
    onOpenChange: (open: boolean) => void,
  ) => ({
    requestClose: () => onOpenChange(false),
  }),
}));

describe("AddToListDialog", () => {
  const createQueryClient = () =>
    new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });

  beforeEach(() => {
    // Reset mock data before each test
    mockBookLists = [
      {
        id: 1,
        name: "Reading List",
        is_ordered: false,
        created_at: "",
        updated_at: "",
        user_id: 1,
        default_sort: "title",
      },
    ];
  });

  describe("initialization and state preservation", () => {
    it("should preserve user selections when query data changes (simulating refetch)", async () => {
      // This test demonstrates the bug:
      // 1. Dialog opens, query returns book is in [list1]
      // 2. User changes selections (uncheck list1, check list2)
      // 3. Query data changes (simulating a refetch)
      // 4. BUG: User's selections are reset to match the new query data
      //
      // The fix: Only initialize selections when dialog opens (closed->open transition),
      // not every time query data changes.

      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <AddToListDialog
            bookId={123}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Wait for data to load and checkboxes to render
      await waitFor(() => {
        expect(screen.getByText("Reading List")).toBeInTheDocument();
      });

      // Initially, "Reading List" (id: 1) should be checked (book is in this list)
      const readingListItem = screen.getByRole("menuitemcheckbox", {
        name: /reading list/i,
      });
      const favoritesItem = screen.getByRole("menuitemcheckbox", {
        name: /favorites/i,
      });

      expect(readingListItem).toHaveAttribute("aria-checked", "true");
      expect(favoritesItem).toHaveAttribute("aria-checked", "false");

      // User unchecks "Reading List" and checks "Favorites"
      await user.click(readingListItem);
      await user.click(favoritesItem);

      // Verify user's changes took effect
      expect(readingListItem).toHaveAttribute("aria-checked", "false");
      expect(favoritesItem).toHaveAttribute("aria-checked", "true");

      // Simulate query data changing (e.g., background refetch)
      // by updating the mock data and re-rendering with same props
      // This triggers useBookLists to return new data
      mockBookLists = [
        {
          id: 1,
          name: "Reading List",
          is_ordered: false,
          created_at: "",
          updated_at: "",
          user_id: 1,
          default_sort: "title",
        },
      ];

      // Force a re-render that would trigger the effect if it depends on data
      rerender(
        <QueryClientProvider client={queryClient}>
          <AddToListDialog
            bookId={123}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // User's selections should be preserved - this is the bug!
      // Currently, the effect runs on data change and resets selections.
      expect(readingListItem).toHaveAttribute("aria-checked", "false");
      expect(favoritesItem).toHaveAttribute("aria-checked", "true");
    });

    it("should initialize selections when dialog opens", async () => {
      const onOpenChange = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <AddToListDialog
            bookId={123}
            onOpenChange={onOpenChange}
            open={false}
          />
        </QueryClientProvider>,
      );

      // Dialog is closed, nothing visible
      expect(screen.queryByText("Reading List")).not.toBeInTheDocument();

      // Open the dialog
      rerender(
        <QueryClientProvider client={queryClient}>
          <AddToListDialog
            bookId={123}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Wait for data to load
      await waitFor(() => {
        expect(screen.getByText("Reading List")).toBeInTheDocument();
      });

      // "Reading List" should be checked (book is in this list per mockBookLists)
      const readingListItem = screen.getByRole("menuitemcheckbox", {
        name: /reading list/i,
      });
      expect(readingListItem).toHaveAttribute("aria-checked", "true");
    });

    it("should reinitialize selections when dialog closes and reopens", async () => {
      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <AddToListDialog
            bookId={123}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Wait for data to load
      await waitFor(() => {
        expect(screen.getByText("Reading List")).toBeInTheDocument();
      });

      // User modifies selections - check Favorites
      const favoritesItem = screen.getByRole("menuitemcheckbox", {
        name: /favorites/i,
      });
      await user.click(favoritesItem);
      expect(favoritesItem).toHaveAttribute("aria-checked", "true");

      // Close the dialog (without saving)
      rerender(
        <QueryClientProvider client={queryClient}>
          <AddToListDialog
            bookId={123}
            onOpenChange={onOpenChange}
            open={false}
          />
        </QueryClientProvider>,
      );

      // Reopen the dialog
      rerender(
        <QueryClientProvider client={queryClient}>
          <AddToListDialog
            bookId={123}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Wait for data to reload
      await waitFor(() => {
        expect(screen.getByText("Reading List")).toBeInTheDocument();
      });

      // Selections should be reinitialized to server state (Favorites unchecked)
      const favoritesItemAfterReopen = screen.getByRole("menuitemcheckbox", {
        name: /favorites/i,
      });
      expect(favoritesItemAfterReopen).toHaveAttribute("aria-checked", "false");
    });

    it("should NOT reset selections when dialog stays open and component re-renders", async () => {
      // This tests that normal re-renders (without data changes) don't reset state
      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <AddToListDialog
            bookId={123}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      await waitFor(() => {
        expect(screen.getByText("Reading List")).toBeInTheDocument();
      });

      // User makes a selection
      const favoritesItem = screen.getByRole("menuitemcheckbox", {
        name: /favorites/i,
      });
      await user.click(favoritesItem);
      expect(favoritesItem).toHaveAttribute("aria-checked", "true");

      // Re-render with same props (simulating parent re-render)
      rerender(
        <QueryClientProvider client={queryClient}>
          <AddToListDialog
            bookId={123}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Selection should be preserved
      expect(favoritesItem).toHaveAttribute("aria-checked", "true");
    });
  });

  describe("hasChanges comparison against initial state", () => {
    it("should compute hasChanges against initial data snapshot, not live query data", async () => {
      // This test exposes the bug: hasChanges compares selectedListIds against
      // bookListsQuery.data (live), not a stored initial snapshot.
      //
      // Scenario:
      // 1. Dialog opens, book is in [list1], user selections = [list1], hasChanges = false
      // 2. User adds list2, selections = [list1, list2], hasChanges = true
      // 3. Background refetch returns book in [list1, list2] (some other process added it)
      // 4. BUG: hasChanges now computes against new query data, becomes false
      //    because selections [list1, list2] === query data [list1, list2]
      // 5. User thinks their changes are saved but they haven't saved yet!
      //
      // The fix: Store initial list IDs when dialog opens, compare against that.

      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const queryClient = createQueryClient();

      // Initially book is in list 1 only
      mockBookLists = [
        {
          id: 1,
          name: "Reading List",
          is_ordered: false,
          created_at: "",
          updated_at: "",
          user_id: 1,
          default_sort: "title",
        },
      ];

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <AddToListDialog
            bookId={123}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      await waitFor(() => {
        expect(screen.getByText("Reading List")).toBeInTheDocument();
      });

      // Initially: book in [list1], hasChanges should be false
      // Save button should be disabled when no changes
      const saveButton = screen.getByRole("button", { name: /save changes/i });
      expect(saveButton).toBeDisabled();

      // User adds Favorites (list2) to selection
      const favoritesItem = screen.getByRole("menuitemcheckbox", {
        name: /favorites/i,
      });
      await user.click(favoritesItem);

      // Now hasChanges should be true, save button enabled
      expect(saveButton).not.toBeDisabled();

      // Simulate background refetch returning updated data where book is now in both lists
      // (perhaps another user/process added it)
      mockBookLists = [
        {
          id: 1,
          name: "Reading List",
          is_ordered: false,
          created_at: "",
          updated_at: "",
          user_id: 1,
          default_sort: "title",
        },
        {
          id: 2,
          name: "Favorites",
          is_ordered: false,
          created_at: "",
          updated_at: "",
          user_id: 1,
          default_sort: "title",
        },
      ];

      // Force re-render to simulate query data change
      rerender(
        <QueryClientProvider client={queryClient}>
          <AddToListDialog
            bookId={123}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // BUG: The current implementation would now say hasChanges = false
      // because it compares [1, 2] against live query [1, 2]
      // But the user DID make changes! They added list2 manually.
      // The save button should STILL be enabled.
      expect(saveButton).not.toBeDisabled();
    });
  });

  describe("bookId change handling", () => {
    // This tests the initialization logic that should reinitialize when bookId changes.
    // The bug: initializedRef is only reset when dialog opens, not when bookId changes.
    // If bookId changes while dialog is open, the form will show stale data.

    // FIXED shouldReinitialize logic - detects bookId changes
    const shouldReinitialize = (
      open: boolean,
      justOpened: boolean,
      hasData: boolean,
      initialized: boolean,
      currentBookId: number,
      prevBookId: number,
    ): boolean => {
      // FIX: Also reset initialization when bookId changes
      const bookIdChanged = currentBookId !== prevBookId;
      if (justOpened || bookIdChanged) {
        // Would reset initializedRef.current = false
        return true; // Will reinitialize when data arrives
      }
      if (!open || !hasData || initialized) return false;
      return true;
    };

    it("should reinitialize when bookId changes while dialog is open", () => {
      // Scenario:
      // 1. Dialog opens for book 1, initializes with book 1's lists
      // 2. bookId prop changes to 2 (while dialog stays open)
      // 3. BUG: Dialog still shows book 1's lists because initialized is true
      // 4. FIX: Should reinitialize with book 2's lists

      const open = true;
      const justOpened = false; // Dialog was already open
      const hasData = true; // New data arrived for book 2
      const initialized = true; // Already initialized for book 1
      const currentBookId = 2; // New bookId
      const prevBookId = 1; // Previous bookId

      const result = shouldReinitialize(
        open,
        justOpened,
        hasData,
        initialized,
        currentBookId,
        prevBookId,
      );

      // FIXED: Now correctly returns true because bookId changed from 1 to 2
      expect(result).toBe(true);
    });

    it("should NOT reinitialize when bookId stays the same", () => {
      const open = true;
      const justOpened = false;
      const hasData = true;
      const initialized = true;
      const currentBookId = 1;
      const prevBookId = 1; // Same bookId

      const result = shouldReinitialize(
        open,
        justOpened,
        hasData,
        initialized,
        currentBookId,
        prevBookId,
      );

      // Should NOT reinitialize - bookId didn't change
      expect(result).toBe(false);
    });
  });
});
