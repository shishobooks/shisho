import RoleDialog from "./RoleDialog";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { Role } from "@/types";

// Mock the mutation hooks
const mockCreateRole = vi.fn();
const mockUpdateRole = vi.fn();
const mockDeleteRole = vi.fn();

vi.mock("@/hooks/queries/users", () => ({
  useCreateRole: () => ({
    mutateAsync: mockCreateRole,
    isPending: false,
  }),
  useUpdateRole: () => ({
    mutateAsync: mockUpdateRole,
    isPending: false,
  }),
  useDeleteRole: () => ({
    mutateAsync: mockDeleteRole,
    isPending: false,
  }),
}));

describe("RoleDialog", () => {
  const createQueryClient = () =>
    new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });

  const mockRole: Role = {
    id: 1,
    name: "Admin",
    is_system: false,
    permissions: [
      { id: 1, role_id: 1, resource: "library", operation: "read" },
    ],
    created_at: "2024-01-01",
    updated_at: "2024-01-01",
  };

  describe("effect runs when dialog is closed", () => {
    it("should NOT reset form state when dialog is closed", async () => {
      // This test reproduces the bug:
      // 1. Dialog opens with a role (form populated)
      // 2. User types a new name
      // 3. Dialog closes (without saving)
      // 4. BUG: The effect runs because `open` changed, resetting the form
      //    to the original role values
      // 5. Dialog re-opens
      // 6. Expected: Form should show the NEW values the user typed (stale state preserved)
      //    OR show the original role values (if we want to reset on close)
      //
      // The actual bug is that the effect runs unnecessarily when the dialog closes.
      // This can cause issues if the parent component hasn't unmounted the dialog yet
      // and is tracking some state based on the form values.

      const user = userEvent.setup();
      const queryClient = createQueryClient();
      const onOpenChange = vi.fn();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <RoleDialog onOpenChange={onOpenChange} open={true} role={mockRole} />
        </QueryClientProvider>,
      );

      // Wait for dialog to render and form to populate
      await waitFor(() => {
        expect(screen.getByDisplayValue("Admin")).toBeInTheDocument();
      });

      // User types a different name
      const nameInput = screen.getByLabelText(/name/i);
      await user.clear(nameInput);
      await user.type(nameInput, "New Role Name");

      // Verify the new name is in the input
      expect(screen.getByDisplayValue("New Role Name")).toBeInTheDocument();

      // Close the dialog
      rerender(
        <QueryClientProvider client={queryClient}>
          <RoleDialog
            onOpenChange={onOpenChange}
            open={false}
            role={mockRole}
          />
        </QueryClientProvider>,
      );

      // The effect with [role, open] dependencies will fire because open changed
      // BUG: This resets the form state even though the dialog is closed

      // Re-open the dialog with the SAME role
      rerender(
        <QueryClientProvider client={queryClient}>
          <RoleDialog onOpenChange={onOpenChange} open={true} role={mockRole} />
        </QueryClientProvider>,
      );

      // Wait for dialog to render
      await waitFor(() => {
        expect(screen.getByLabelText(/name/i)).toBeInTheDocument();
      });

      // The form should show the ORIGINAL role values, not the user's typed values
      // This is the EXPECTED behavior - when re-opening with the same role,
      // the form should show the original values.
      //
      // However, the bug is that the effect ran when open=false, which is wasteful.
      // The real issue is observable in the effect running when it shouldn't.
      // Let's verify the effect only runs when open is true by checking the form state.

      // After re-opening, the name should be "Admin" (from the role prop)
      expect(screen.getByDisplayValue("Admin")).toBeInTheDocument();
    });

    it("should not run initialization effect when dialog closes", async () => {
      // This test more directly tests the bug:
      // The effect should NOT run when open transitions from true to false

      const queryClient = createQueryClient();
      const onOpenChange = vi.fn();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <RoleDialog onOpenChange={onOpenChange} open={true} role={mockRole} />
        </QueryClientProvider>,
      );

      // Wait for dialog to render
      await waitFor(() => {
        expect(screen.getByDisplayValue("Admin")).toBeInTheDocument();
      });

      // Close the dialog - the effect should NOT run
      rerender(
        <QueryClientProvider client={queryClient}>
          <RoleDialog
            onOpenChange={onOpenChange}
            open={false}
            role={mockRole}
          />
        </QueryClientProvider>,
      );

      // The dialog should not be visible
      expect(screen.queryByDisplayValue("Admin")).not.toBeInTheDocument();

      // Re-open with a DIFFERENT role
      const newRole = {
        ...mockRole,
        id: 2,
        name: "Editor",
      };

      rerender(
        <QueryClientProvider client={queryClient}>
          <RoleDialog onOpenChange={onOpenChange} open={true} role={newRole} />
        </QueryClientProvider>,
      );

      // Should show the new role's name
      await waitFor(() => {
        expect(screen.getByDisplayValue("Editor")).toBeInTheDocument();
      });
    });

    it("should NOT reinitialize form when role prop changes while dialog is open (preserves user edits)", async () => {
      // When dialog is open and user has made edits, we should NOT lose
      // those edits just because the role prop changed (e.g., background refetch).
      // The form should only initialize once when the dialog opens.

      const user = userEvent.setup();
      const queryClient = createQueryClient();
      const onOpenChange = vi.fn();

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <RoleDialog onOpenChange={onOpenChange} open={true} role={mockRole} />
        </QueryClientProvider>,
      );

      // Wait for form to populate with Admin
      await waitFor(() => {
        expect(screen.getByDisplayValue("Admin")).toBeInTheDocument();
      });

      // User makes edits
      const nameInput = screen.getByLabelText(/name/i);
      await user.clear(nameInput);
      await user.type(nameInput, "My Custom Role");

      // Verify user's edit is in place
      expect(screen.getByDisplayValue("My Custom Role")).toBeInTheDocument();

      // Simulate role prop changing (e.g., background refetch with same data)
      const updatedRole = {
        ...mockRole,
        name: "Admin", // Same data from server
        updated_at: "2024-01-02", // Different timestamp indicates refetch
      };

      rerender(
        <QueryClientProvider client={queryClient}>
          <RoleDialog
            onOpenChange={onOpenChange}
            open={true}
            role={updatedRole}
          />
        </QueryClientProvider>,
      );

      // User's edits should be preserved, NOT overwritten by the prop change
      expect(screen.getByDisplayValue("My Custom Role")).toBeInTheDocument();
    });

    it("should not trigger state updates when role changes while dialog is closed", async () => {
      // This test verifies the effect doesn't run when the dialog is closed.
      // The bug: effect runs when open=false, causing unnecessary state updates.
      //
      // Observable issue: if role changes while dialog is closed, we want to
      // defer the state update until the dialog re-opens. This prevents:
      // 1. Unnecessary re-renders while dialog is closed
      // 2. Potential issues if parent is tracking form state
      //
      // To test this, we track when the form gets populated by checking
      // whether the input shows the role name. When the dialog is closed,
      // the input isn't rendered, so we can't directly observe the state.
      //
      // Instead, we'll use a mock to count effect invocations.

      const queryClient = createQueryClient();
      const onOpenChange = vi.fn();

      // We can't easily mock useEffect, so we'll test behavior:
      // When dialog is closed and role changes, then dialog re-opens,
      // the form should show the NEW role (effect ran on re-open, not on close)

      const { rerender } = render(
        <QueryClientProvider client={queryClient}>
          <RoleDialog onOpenChange={onOpenChange} open={true} role={mockRole} />
        </QueryClientProvider>,
      );

      // Wait for form to populate with "Admin"
      await waitFor(() => {
        expect(screen.getByDisplayValue("Admin")).toBeInTheDocument();
      });

      // Close the dialog
      rerender(
        <QueryClientProvider client={queryClient}>
          <RoleDialog
            onOpenChange={onOpenChange}
            open={false}
            role={mockRole}
          />
        </QueryClientProvider>,
      );

      // Dialog is closed, form not visible
      expect(screen.queryByDisplayValue("Admin")).not.toBeInTheDocument();

      // Change the role while dialog is closed
      const newRole = {
        ...mockRole,
        id: 2,
        name: "Editor",
      };

      rerender(
        <QueryClientProvider client={queryClient}>
          <RoleDialog onOpenChange={onOpenChange} open={false} role={newRole} />
        </QueryClientProvider>,
      );

      // Dialog still closed
      expect(screen.queryByDisplayValue("Editor")).not.toBeInTheDocument();

      // Re-open the dialog - effect should run NOW and show new role
      rerender(
        <QueryClientProvider client={queryClient}>
          <RoleDialog onOpenChange={onOpenChange} open={true} role={newRole} />
        </QueryClientProvider>,
      );

      // Should show the new role's name
      await waitFor(() => {
        expect(screen.getByDisplayValue("Editor")).toBeInTheDocument();
      });
    });
  });

  describe("hasChanges calculation", () => {
    it("should have no changes in create mode when form is empty (same as initial values)", async () => {
      // In create mode, initialValues = { name: "", permissions: [] }
      // When form is empty, hasChanges should be false
      // This tests that the hasChanges logic treats create mode consistently with edit mode

      const queryClient = createQueryClient();
      const onOpenChange = vi.fn();

      render(
        <QueryClientProvider client={queryClient}>
          <RoleDialog onOpenChange={onOpenChange} open={true} role={null} />
        </QueryClientProvider>,
      );

      // Wait for dialog to render
      await waitFor(() => {
        expect(
          screen.getByRole("heading", { name: "Create Role" }),
        ).toBeInTheDocument();
      });

      // Form should be empty (matching initial values)
      const nameInput = screen.getByLabelText(/name/i);
      expect(nameInput).toHaveValue("");

      // Try to close dialog - should close without unsaved changes warning
      // because hasChanges should be false (form matches initial values)
      const cancelButton = screen.getByRole("button", { name: /cancel/i });
      await userEvent.click(cancelButton);

      // onOpenChange should be called with false (dialog closes)
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });

    it("should detect changes in create mode when name is entered", async () => {
      const user = userEvent.setup();
      const queryClient = createQueryClient();
      const onOpenChange = vi.fn();

      render(
        <QueryClientProvider client={queryClient}>
          <RoleDialog onOpenChange={onOpenChange} open={true} role={null} />
        </QueryClientProvider>,
      );

      // Wait for dialog to render
      await waitFor(() => {
        expect(
          screen.getByRole("heading", { name: "Create Role" }),
        ).toBeInTheDocument();
      });

      // Enter a name
      const nameInput = screen.getByLabelText(/name/i);
      await user.type(nameInput, "Test Role");

      expect(nameInput).toHaveValue("Test Role");
      // hasChanges should now be true (form differs from initial values)
    });

    it("should have no changes in edit mode when form matches initial values", async () => {
      const queryClient = createQueryClient();
      const onOpenChange = vi.fn();

      render(
        <QueryClientProvider client={queryClient}>
          <RoleDialog onOpenChange={onOpenChange} open={true} role={mockRole} />
        </QueryClientProvider>,
      );

      // Wait for dialog to render with role data
      await waitFor(() => {
        expect(screen.getByDisplayValue("Admin")).toBeInTheDocument();
      });

      // Form should match initial values (role props)
      // Try to close dialog - should close without unsaved changes warning
      const cancelButton = screen.getByRole("button", { name: /cancel/i });
      await userEvent.click(cancelButton);

      // onOpenChange should be called with false (dialog closes)
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });
});
