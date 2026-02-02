import { CreateListDialog } from "./CreateListDialog";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { List } from "@/types";

// Mock useFormDialogClose hook
vi.mock("@/hooks/useFormDialogClose", () => ({
  useFormDialogClose: (
    _open: boolean,
    onOpenChange: (open: boolean) => void,
  ) => ({
    requestClose: () => onOpenChange(false),
  }),
}));

describe("CreateListDialog", () => {
  const createQueryClient = () =>
    new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });

  const mockList: List = {
    id: 1,
    name: "My List",
    description: "A test list",
    is_ordered: false,
    created_at: "2024-01-01",
    updated_at: "2024-01-01",
    user_id: 1,
    default_sort: "title",
  };

  describe("initialization and state preservation (edit mode)", () => {
    it("should NOT reset form when list prop changes while dialog is open (preserves user edits)", async () => {
      // This test demonstrates the bug:
      // 1. Dialog opens in edit mode with list data
      // 2. User edits the name field
      // 3. List prop changes (e.g., background refetch returns updated data)
      // 4. BUG: Form resets to the new list data, losing user's edits
      //
      // The fix: Only initialize form when dialog opens (closed->open transition),
      // not every time list prop changes.

      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const onUpdate = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            list={mockList}
            onOpenChange={onOpenChange}
            onUpdate={onUpdate}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Wait for form to populate
      await waitFor(() => {
        expect(screen.getByDisplayValue("My List")).toBeInTheDocument();
      });

      // User edits the name
      const nameInput = screen.getByLabelText(/name/i);
      await user.clear(nameInput);
      await user.type(nameInput, "User Edited Name");

      // Verify user's edit is in place
      expect(screen.getByDisplayValue("User Edited Name")).toBeInTheDocument();

      // Simulate list prop changing (e.g., background refetch with same data)
      const updatedList = {
        ...mockList,
        name: "My List", // Same name from server
        updated_at: "2024-01-02", // Different timestamp indicates refetch
      };

      rerender(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            list={updatedList}
            onOpenChange={onOpenChange}
            onUpdate={onUpdate}
            open={true}
          />
        </QueryClientProvider>,
      );

      // User's edits should be preserved, NOT overwritten by the prop change
      // This is the bug - currently it resets to "My List"
      expect(screen.getByDisplayValue("User Edited Name")).toBeInTheDocument();
    });

    it("should initialize form when dialog opens in edit mode", async () => {
      const onOpenChange = vi.fn();
      const onUpdate = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            list={mockList}
            onOpenChange={onOpenChange}
            onUpdate={onUpdate}
            open={false}
          />
        </QueryClientProvider>,
      );

      // Dialog is closed, form not visible
      expect(screen.queryByDisplayValue("My List")).not.toBeInTheDocument();

      // Open the dialog
      rerender(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            list={mockList}
            onOpenChange={onOpenChange}
            onUpdate={onUpdate}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Form should be populated with list data
      await waitFor(() => {
        expect(screen.getByDisplayValue("My List")).toBeInTheDocument();
      });
      expect(screen.getByDisplayValue("A test list")).toBeInTheDocument();
    });

    it("should reinitialize form when dialog closes and reopens", async () => {
      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const onUpdate = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            list={mockList}
            onOpenChange={onOpenChange}
            onUpdate={onUpdate}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Wait for form to populate
      await waitFor(() => {
        expect(screen.getByDisplayValue("My List")).toBeInTheDocument();
      });

      // User edits the name
      const nameInput = screen.getByLabelText(/name/i);
      await user.clear(nameInput);
      await user.type(nameInput, "Edited Name");
      expect(screen.getByDisplayValue("Edited Name")).toBeInTheDocument();

      // Close the dialog (without saving)
      rerender(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            list={mockList}
            onOpenChange={onOpenChange}
            onUpdate={onUpdate}
            open={false}
          />
        </QueryClientProvider>,
      );

      // Reopen the dialog
      rerender(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            list={mockList}
            onOpenChange={onOpenChange}
            onUpdate={onUpdate}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Form should be reinitialized to original list data
      await waitFor(() => {
        expect(screen.getByDisplayValue("My List")).toBeInTheDocument();
      });
    });

    it("should initialize form with new list when opening with different list", async () => {
      const onOpenChange = vi.fn();
      const onUpdate = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            list={mockList}
            onOpenChange={onOpenChange}
            onUpdate={onUpdate}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Form shows first list
      await waitFor(() => {
        expect(screen.getByDisplayValue("My List")).toBeInTheDocument();
      });

      // Close dialog
      rerender(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            list={mockList}
            onOpenChange={onOpenChange}
            onUpdate={onUpdate}
            open={false}
          />
        </QueryClientProvider>,
      );

      // Open with different list
      const differentList: List = {
        id: 2,
        name: "Different List",
        description: "Another list",
        is_ordered: true,
        created_at: "2024-01-01",
        updated_at: "2024-01-01",
        user_id: 1,
        default_sort: "title",
      };

      rerender(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            list={differentList}
            onOpenChange={onOpenChange}
            onUpdate={onUpdate}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Form should show new list data
      await waitFor(() => {
        expect(screen.getByDisplayValue("Different List")).toBeInTheDocument();
      });
    });
  });

  describe("initialization and state preservation (create mode)", () => {
    it("should NOT reset form when component re-renders in create mode", async () => {
      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const onCreate = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            onCreate={onCreate}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // User types a name
      const nameInput = screen.getByLabelText(/name/i);
      await user.type(nameInput, "New List Name");
      expect(screen.getByDisplayValue("New List Name")).toBeInTheDocument();

      // Re-render with same props (simulating parent re-render)
      rerender(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            onCreate={onCreate}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // User's input should be preserved
      expect(screen.getByDisplayValue("New List Name")).toBeInTheDocument();
    });

    it("should reset form when dialog closes and reopens in create mode", async () => {
      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const onCreate = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            onCreate={onCreate}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // User types a name
      const nameInput = screen.getByLabelText(/name/i);
      await user.type(nameInput, "New List Name");
      expect(screen.getByDisplayValue("New List Name")).toBeInTheDocument();

      // Close dialog
      rerender(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            onCreate={onCreate}
            onOpenChange={onOpenChange}
            open={false}
          />
        </QueryClientProvider>,
      );

      // Reopen dialog
      rerender(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            onCreate={onCreate}
            onOpenChange={onOpenChange}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Form should be reset to empty
      const nameInputAfter = screen.getByLabelText(/name/i);
      expect(nameInputAfter).toHaveValue("");
    });
  });

  describe("hasChanges comparison against initial state", () => {
    it("should compute hasChanges against initial values, not live props (edit mode)", async () => {
      // This test exposes the bug: hasChanges compares form values against
      // live list prop values, not stored initial values.
      //
      // Scenario:
      // 1. Dialog opens with list { name: "Original" }, user changes name to "Edited"
      // 2. hasChanges = true (correct)
      // 3. List prop changes to { name: "Edited" } (background refetch or server update)
      // 4. BUG: hasChanges now = false because form "Edited" === prop "Edited"
      //    But user DID make changes!
      //
      // The fix: Store initial values when dialog opens, compare against that.

      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const onUpdate = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            list={mockList}
            onOpenChange={onOpenChange}
            onUpdate={onUpdate}
            open={true}
          />
        </QueryClientProvider>,
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("My List")).toBeInTheDocument();
      });

      // Initially, no changes - Save button should be disabled (due to canSave = name.trim() && hasChanges)
      // Actually the button is disabled when !hasChanges, let's check the button state
      const saveButton = screen.getByRole("button", { name: /save/i });

      // User changes name to "Different Name"
      const nameInput = screen.getByLabelText(/name/i);
      await user.clear(nameInput);
      await user.type(nameInput, "Different Name");

      // hasChanges should be true now, save button enabled
      await waitFor(() => {
        expect(saveButton).not.toBeDisabled();
      });

      // Simulate list prop changing to match user's edits (e.g., another session saved same change)
      const updatedList = {
        ...mockList,
        name: "Different Name", // Now matches what user typed
      };

      rerender(
        <QueryClientProvider client={queryClient}>
          <CreateListDialog
            isPending={false}
            list={updatedList}
            onOpenChange={onOpenChange}
            onUpdate={onUpdate}
            open={true}
          />
        </QueryClientProvider>,
      );

      // BUG: hasChanges would now be false because form value === prop value
      // But the user DID make changes from the initial state!
      // Save button should STILL be enabled
      expect(saveButton).not.toBeDisabled();
    });
  });
});
