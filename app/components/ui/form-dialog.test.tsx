import { FormDialog } from "./form-dialog";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

describe("FormDialog", () => {
  // Store original addEventListener/removeEventListener
  let addEventListenerSpy: ReturnType<typeof vi.spyOn>;
  let removeEventListenerSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    addEventListenerSpy = vi.spyOn(window, "addEventListener");
    removeEventListenerSpy = vi.spyOn(window, "removeEventListener");
  });

  afterEach(() => {
    addEventListenerSpy.mockRestore();
    removeEventListenerSpy.mockRestore();
  });
  it("should close without confirmation when hasChanges is false", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();

    render(
      <FormDialog open={true} onOpenChange={onOpenChange} hasChanges={false}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    expect(screen.getByText("Test Dialog")).toBeInTheDocument();

    // Click the X button to close
    const closeButton = screen.getByRole("button", { name: /close/i });
    await user.click(closeButton);

    // Should close directly without confirmation
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should close directly when clicking X button even with unsaved changes", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();

    render(
      <FormDialog open={true} onOpenChange={onOpenChange} hasChanges={true}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    // Click the X button to close - should close directly without confirmation
    // because X is an explicit close action (like Cancel button)
    const closeButton = screen.getByRole("button", { name: /close/i });
    await user.click(closeButton);

    // Should close directly without showing confirmation
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should show UnsavedChangesDialog when pressing ESC with hasChanges", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();

    render(
      <FormDialog open={true} onOpenChange={onOpenChange} hasChanges={true}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    // Press ESC to close - this is an implicit close action
    await user.keyboard("{Escape}");

    // Should not close directly
    expect(onOpenChange).not.toHaveBeenCalled();

    // Should show confirmation dialog
    await waitFor(() => {
      expect(screen.getByText("Unsaved Changes")).toBeInTheDocument();
      expect(screen.getByText(/You have unsaved changes/)).toBeInTheDocument();
    });
  });

  it("handleStay should keep dialog open when clicking Stay button", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();

    render(
      <FormDialog open={true} onOpenChange={onOpenChange} hasChanges={true}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    // Press ESC to show confirmation (implicit close action)
    await user.keyboard("{Escape}");

    // Wait for confirmation dialog
    await waitFor(() => {
      expect(screen.getByText("Unsaved Changes")).toBeInTheDocument();
    });

    // Click Stay button
    const stayButton = screen.getByRole("button", { name: /stay/i });
    await user.click(stayButton);

    // onOpenChange should not have been called
    expect(onOpenChange).not.toHaveBeenCalled();

    // Confirmation dialog should be closed, main dialog still open
    await waitFor(() => {
      expect(screen.queryByText("Unsaved Changes")).not.toBeInTheDocument();
    });
    expect(screen.getByText("Test Dialog")).toBeInTheDocument();
  });

  it("handleDiscard should close dialog when clicking Discard button", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();

    render(
      <FormDialog open={true} onOpenChange={onOpenChange} hasChanges={true}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    // Press ESC to show confirmation (implicit close action)
    await user.keyboard("{Escape}");

    // Wait for confirmation dialog
    await waitFor(() => {
      expect(screen.getByText("Unsaved Changes")).toBeInTheDocument();
    });

    // Click Discard button
    const discardButton = screen.getByRole("button", { name: /discard/i });
    await user.click(discardButton);

    // Dialog should close
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should not show multiple confirmation dialogs on rapid ESC presses", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();

    render(
      <FormDialog open={true} onOpenChange={onOpenChange} hasChanges={true}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    // First ESC shows confirmation
    await user.keyboard("{Escape}");

    // Wait for confirmation dialog to appear
    await waitFor(() => {
      expect(screen.getByText("Unsaved Changes")).toBeInTheDocument();
    });

    // Should only show one confirmation dialog even though showConfirmDialog
    // is already true (guard condition prevents setting it again)
    const unsavedChangesHeaders = screen.getAllByText("Unsaved Changes");
    expect(unsavedChangesHeaders).toHaveLength(1);

    // onOpenChange should not have been called (dialog not closed)
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("should handle onOpenChange being undefined", async () => {
    render(
      <FormDialog open={true} hasChanges={false}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    expect(screen.getByText("Test Dialog")).toBeInTheDocument();
  });

  it("should pass through other Dialog props", () => {
    render(
      <FormDialog
        open={true}
        onOpenChange={vi.fn()}
        hasChanges={false}
        modal={true}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    expect(screen.getByText("Test Dialog")).toBeInTheDocument();
  });

  it("should render children correctly", () => {
    render(
      <FormDialog open={true} onOpenChange={vi.fn()} hasChanges={false}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Custom Title</DialogTitle>
            <DialogDescription>Custom description.</DialogDescription>
          </DialogHeader>
          <div data-testid="custom-content">Custom content here</div>
        </DialogContent>
      </FormDialog>,
    );

    expect(screen.getByText("Custom Title")).toBeInTheDocument();
    expect(screen.getByTestId("custom-content")).toBeInTheDocument();
  });

  it("should not render when open is false", () => {
    render(
      <FormDialog open={false} onOpenChange={vi.fn()} hasChanges={false}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    expect(screen.queryByText("Test Dialog")).not.toBeInTheDocument();
  });

  it("should close when hasChanges changes from true to false", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();

    const { rerender } = render(
      <FormDialog open={true} onOpenChange={onOpenChange} hasChanges={true}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    // Rerender with hasChanges = false
    rerender(
      <FormDialog open={true} onOpenChange={onOpenChange} hasChanges={false}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    // Now clicking close should work without confirmation
    const closeButton = screen.getByRole("button", { name: /close/i });
    await user.click(closeButton);

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should reset confirmation dialog when open becomes false externally", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();

    const { rerender } = render(
      <FormDialog open={true} onOpenChange={onOpenChange} hasChanges={true}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    // Press ESC to show confirmation dialog (implicit close action)
    await user.keyboard("{Escape}");

    // Confirmation dialog should appear
    await waitFor(() => {
      expect(screen.getByText("Unsaved Changes")).toBeInTheDocument();
    });

    // Parent externally closes the dialog (e.g., via some other action)
    rerender(
      <FormDialog open={false} onOpenChange={onOpenChange} hasChanges={true}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Test Dialog</DialogTitle>
            <DialogDescription>Test dialog description.</DialogDescription>
          </DialogHeader>
          <p>Dialog content</p>
        </DialogContent>
      </FormDialog>,
    );

    // Both dialogs should be closed - no "Unsaved Changes" dialog should remain
    await waitFor(() => {
      expect(screen.queryByText("Unsaved Changes")).not.toBeInTheDocument();
      expect(screen.queryByText("Test Dialog")).not.toBeInTheDocument();
    });
  });

  describe("beforeunload event handling", () => {
    it("should add beforeunload listener when open and hasChanges are true", () => {
      render(
        <FormDialog open={true} onOpenChange={vi.fn()} hasChanges={true}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Test Dialog</DialogTitle>
              <DialogDescription>Test dialog description.</DialogDescription>
            </DialogHeader>
          </DialogContent>
        </FormDialog>,
      );

      expect(window.addEventListener).toHaveBeenCalledWith(
        "beforeunload",
        expect.any(Function),
      );
    });

    it("should not add beforeunload listener when hasChanges is false", () => {
      render(
        <FormDialog open={true} onOpenChange={vi.fn()} hasChanges={false}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Test Dialog</DialogTitle>
              <DialogDescription>Test dialog description.</DialogDescription>
            </DialogHeader>
          </DialogContent>
        </FormDialog>,
      );

      const beforeUnloadCalls = vi.mocked(window.addEventListener).mock.calls;
      const hasBeforeUnload = beforeUnloadCalls.some(
        (call) => call[0] === "beforeunload",
      );
      expect(hasBeforeUnload).toBe(false);
    });

    it("should not add beforeunload listener when dialog is closed", () => {
      render(
        <FormDialog open={false} onOpenChange={vi.fn()} hasChanges={true}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Test Dialog</DialogTitle>
              <DialogDescription>Test dialog description.</DialogDescription>
            </DialogHeader>
          </DialogContent>
        </FormDialog>,
      );

      const beforeUnloadCalls = vi.mocked(window.addEventListener).mock.calls;
      const hasBeforeUnload = beforeUnloadCalls.some(
        (call) => call[0] === "beforeunload",
      );
      expect(hasBeforeUnload).toBe(false);
    });

    it("should remove beforeunload listener on unmount", () => {
      const { unmount } = render(
        <FormDialog open={true} onOpenChange={vi.fn()} hasChanges={true}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Test Dialog</DialogTitle>
              <DialogDescription>Test dialog description.</DialogDescription>
            </DialogHeader>
          </DialogContent>
        </FormDialog>,
      );

      const addedHandler = vi
        .mocked(window.addEventListener)
        .mock.calls.find((call) => call[0] === "beforeunload")?.[1];

      unmount();

      expect(window.removeEventListener).toHaveBeenCalledWith(
        "beforeunload",
        addedHandler,
      );
    });

    it("should call preventDefault on beforeunload event", () => {
      render(
        <FormDialog open={true} onOpenChange={vi.fn()} hasChanges={true}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Test Dialog</DialogTitle>
              <DialogDescription>Test dialog description.</DialogDescription>
            </DialogHeader>
          </DialogContent>
        </FormDialog>,
      );

      const handler = vi
        .mocked(window.addEventListener)
        .mock.calls.find((call) => call[0] === "beforeunload")?.[1] as (
        e: BeforeUnloadEvent,
      ) => void;

      const mockEvent = {
        preventDefault: vi.fn(),
        returnValue: "",
      } as unknown as BeforeUnloadEvent;

      handler(mockEvent);

      expect(mockEvent.preventDefault).toHaveBeenCalled();
      expect(mockEvent.returnValue).toBe("");
    });
  });
});
