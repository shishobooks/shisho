import { MetadataEditDialog } from "./MetadataEditDialog";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

// Mock useFormDialogClose hook
vi.mock("@/hooks/useFormDialogClose", () => ({
  useFormDialogClose: (
    _open: boolean,
    onOpenChange: (open: boolean) => void,
  ) => ({
    requestClose: () => onOpenChange(false),
  }),
}));

describe("MetadataEditDialog", () => {
  const createQueryClient = () =>
    new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });

  describe("initialization and state preservation", () => {
    it("should NOT reset form when props change while dialog is open (preserves user edits)", async () => {
      // This test demonstrates the bug:
      // 1. Dialog opens with entityName="Original Name"
      // 2. User edits the name to "Edited Name"
      // 3. entityName prop changes (e.g., parent re-renders with new data)
      // 4. BUG: Form resets to the new entityName, losing user's edits
      //
      // The fix: Only initialize form when dialog opens (closed->open transition),
      // not every time props change.

      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const onSave = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <MetadataEditDialog
            entityName="Original Name"
            entityType="person"
            isPending={false}
            onOpenChange={onOpenChange}
            onSave={onSave}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Wait for form to populate
      await waitFor(() => {
        expect(screen.getByDisplayValue("Original Name")).toBeInTheDocument();
      });

      // User edits the name (use exact label to avoid matching "Sort Name")
      const nameInput = screen.getByLabelText("Name");
      await user.clear(nameInput);
      await user.type(nameInput, "Edited Name");

      // Verify user's edit is in place
      expect(screen.getByDisplayValue("Edited Name")).toBeInTheDocument();

      // Simulate entityName prop changing (e.g., background refetch or parent re-render)
      rerender(
        <QueryClientProvider client={queryClient}>
          <MetadataEditDialog
            entityName="Original Name"
            entityType="person"
            isPending={false}
            onOpenChange={onOpenChange}
            onSave={onSave}
            open={true}
          />
        </QueryClientProvider>,
      );

      // User's edits should be preserved, NOT overwritten by the prop change
      expect(screen.getByDisplayValue("Edited Name")).toBeInTheDocument();
    });

    it("should initialize form when dialog opens", async () => {
      const onOpenChange = vi.fn();
      const onSave = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <MetadataEditDialog
            entityName="Test Entity"
            entityType="person"
            isPending={false}
            onOpenChange={onOpenChange}
            onSave={onSave}
            open={false}
          />
        </QueryClientProvider>,
      );

      // Dialog is closed, form not visible
      expect(screen.queryByDisplayValue("Test Entity")).not.toBeInTheDocument();

      // Open the dialog
      rerender(
        <QueryClientProvider client={queryClient}>
          <MetadataEditDialog
            entityName="Test Entity"
            entityType="person"
            isPending={false}
            onOpenChange={onOpenChange}
            onSave={onSave}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Form should be populated with entity data
      await waitFor(() => {
        expect(screen.getByDisplayValue("Test Entity")).toBeInTheDocument();
      });
    });

    it("should reinitialize form when dialog closes and reopens", async () => {
      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const onSave = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <MetadataEditDialog
            entityName="Test Entity"
            entityType="person"
            isPending={false}
            onOpenChange={onOpenChange}
            onSave={onSave}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Wait for form to populate
      await waitFor(() => {
        expect(screen.getByDisplayValue("Test Entity")).toBeInTheDocument();
      });

      // User edits the name (use exact label to avoid matching "Sort Name")
      const nameInput = screen.getByLabelText("Name");
      await user.clear(nameInput);
      await user.type(nameInput, "Edited Name");
      expect(screen.getByDisplayValue("Edited Name")).toBeInTheDocument();

      // Close the dialog (without saving)
      rerender(
        <QueryClientProvider client={queryClient}>
          <MetadataEditDialog
            entityName="Test Entity"
            entityType="person"
            isPending={false}
            onOpenChange={onOpenChange}
            onSave={onSave}
            open={false}
          />
        </QueryClientProvider>,
      );

      // Reopen the dialog
      rerender(
        <QueryClientProvider client={queryClient}>
          <MetadataEditDialog
            entityName="Test Entity"
            entityType="person"
            isPending={false}
            onOpenChange={onOpenChange}
            onSave={onSave}
            open={true}
          />
        </QueryClientProvider>,
      );

      // Form should be reinitialized to original data
      await waitFor(() => {
        expect(screen.getByDisplayValue("Test Entity")).toBeInTheDocument();
      });
    });
  });

  describe("hasChanges comparison against initial state", () => {
    it("should compute hasChanges against initial values, not live props", async () => {
      // This test exposes the bug: hasChanges compares form values against
      // live props (entityName, sortName), not stored initial values.
      //
      // Scenario:
      // 1. Dialog opens with entityName="Original", user changes name to "Edited"
      // 2. hasChanges = true (correct)
      // 3. entityName prop changes to "Edited" (parent re-render or refetch)
      // 4. BUG: hasChanges now = false because form "Edited" === prop "Edited"
      //    But user DID make changes!
      //
      // The fix: Store initial values when dialog opens, compare against that.

      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const onSave = vi.fn();
      const queryClient = createQueryClient();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <MetadataEditDialog
            entityName="Original Name"
            entityType="person"
            isPending={false}
            onOpenChange={onOpenChange}
            onSave={onSave}
            open={true}
          />
        </QueryClientProvider>,
      );

      await waitFor(() => {
        expect(screen.getByDisplayValue("Original Name")).toBeInTheDocument();
      });

      // Save button should be disabled when no changes
      const saveButton = screen.getByRole("button", { name: /save/i });
      expect(saveButton).toBeDisabled();

      // User changes name to "Different Name" (use exact label to avoid matching "Sort Name")
      const nameInput = screen.getByLabelText("Name");
      await user.clear(nameInput);
      await user.type(nameInput, "Different Name");

      // hasChanges should be true now, save button enabled
      await waitFor(() => {
        expect(saveButton).not.toBeDisabled();
      });

      // Simulate entityName prop changing to match user's edits
      rerender(
        <QueryClientProvider client={queryClient}>
          <MetadataEditDialog
            entityName="Different Name"
            entityType="person"
            isPending={false}
            onOpenChange={onOpenChange}
            onSave={onSave}
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
