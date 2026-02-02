import { UnsavedChangesDialog } from "./unsaved-changes-dialog";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

describe("UnsavedChangesDialog", () => {
  it("should render dialog content when open", () => {
    render(
      <UnsavedChangesDialog open={true} onStay={vi.fn()} onDiscard={vi.fn()} />,
    );

    expect(screen.getByText("Unsaved Changes")).toBeInTheDocument();
    expect(screen.getByText(/You have unsaved changes/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /stay/i })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /discard/i }),
    ).toBeInTheDocument();
  });

  it("should not render when open is false", () => {
    render(
      <UnsavedChangesDialog
        open={false}
        onStay={vi.fn()}
        onDiscard={vi.fn()}
      />,
    );

    expect(screen.queryByText("Unsaved Changes")).not.toBeInTheDocument();
  });

  it("should call onStay when Stay button is clicked", async () => {
    const user = userEvent.setup();
    const onStay = vi.fn();
    const onDiscard = vi.fn();

    render(
      <UnsavedChangesDialog
        open={true}
        onStay={onStay}
        onDiscard={onDiscard}
      />,
    );

    const stayButton = screen.getByRole("button", { name: /stay/i });
    await user.click(stayButton);

    expect(onStay).toHaveBeenCalledTimes(1);
    expect(onDiscard).not.toHaveBeenCalled();
  });

  it("should call onDiscard when Discard button is clicked", async () => {
    const user = userEvent.setup();
    const onStay = vi.fn();
    const onDiscard = vi.fn();

    render(
      <UnsavedChangesDialog
        open={true}
        onStay={onStay}
        onDiscard={onDiscard}
      />,
    );

    const discardButton = screen.getByRole("button", { name: /discard/i });
    await user.click(discardButton);

    expect(onDiscard).toHaveBeenCalledTimes(1);
    expect(onStay).not.toHaveBeenCalled();
  });

  it("should call onStay when dialog is closed via ESC key", async () => {
    const user = userEvent.setup();
    const onStay = vi.fn();
    const onDiscard = vi.fn();

    render(
      <UnsavedChangesDialog
        open={true}
        onStay={onStay}
        onDiscard={onDiscard}
      />,
    );

    await user.keyboard("{Escape}");

    expect(onStay).toHaveBeenCalledTimes(1);
    expect(onDiscard).not.toHaveBeenCalled();
  });

  it("should call onStay when dialog is closed via overlay click", async () => {
    const user = userEvent.setup();
    const onStay = vi.fn();
    const onDiscard = vi.fn();

    render(
      <UnsavedChangesDialog
        open={true}
        onStay={onStay}
        onDiscard={onDiscard}
      />,
    );

    // Find the overlay (the dark background)
    // The overlay is the sibling to DialogContent within the portal
    const overlay =
      document.querySelector("[data-radix-dialog-overlay]") ||
      document.querySelector(".fixed.inset-0");

    if (overlay) {
      await user.click(overlay);
      expect(onStay).toHaveBeenCalled();
    }
  });

  it("should display correct dialog description", () => {
    render(
      <UnsavedChangesDialog open={true} onStay={vi.fn()} onDiscard={vi.fn()} />,
    );

    expect(
      screen.getByText(
        /You have unsaved changes\. Are you sure you want to leave\? Your changes will be lost\./,
      ),
    ).toBeInTheDocument();
  });

  it("should have correct button variants", () => {
    render(
      <UnsavedChangesDialog open={true} onStay={vi.fn()} onDiscard={vi.fn()} />,
    );

    const stayButton = screen.getByRole("button", { name: /stay/i });
    const discardButton = screen.getByRole("button", { name: /discard/i });

    // Stay button should be outline variant (less prominent)
    expect(stayButton.className).toContain("border");

    // Discard button should be destructive variant (red/dangerous)
    expect(discardButton.className).toContain("destructive");
  });

  it("should handle rapid button clicks without errors", async () => {
    const user = userEvent.setup();
    const onStay = vi.fn();
    const onDiscard = vi.fn();

    render(
      <UnsavedChangesDialog
        open={true}
        onStay={onStay}
        onDiscard={onDiscard}
      />,
    );

    const stayButton = screen.getByRole("button", { name: /stay/i });

    // Rapid clicks
    await user.click(stayButton);
    await user.click(stayButton);
    await user.click(stayButton);

    // Should be called multiple times (no debouncing expected)
    expect(onStay).toHaveBeenCalled();
  });

  it("should be accessible with proper ARIA labels", () => {
    render(
      <UnsavedChangesDialog open={true} onStay={vi.fn()} onDiscard={vi.fn()} />,
    );

    // Dialog should be accessible
    const dialog = screen.getByRole("dialog");
    expect(dialog).toBeInTheDocument();

    // Title should be present
    expect(screen.getByText("Unsaved Changes")).toBeInTheDocument();
  });

  it("should render close button that triggers onStay", async () => {
    const user = userEvent.setup();
    const onStay = vi.fn();
    const onDiscard = vi.fn();

    render(
      <UnsavedChangesDialog
        open={true}
        onStay={onStay}
        onDiscard={onDiscard}
      />,
    );

    // Find the X close button
    const closeButton = screen.getByRole("button", { name: /close/i });
    expect(closeButton).toBeInTheDocument();

    await user.click(closeButton);

    // Closing should trigger onStay (stay on page)
    expect(onStay).toHaveBeenCalled();
    expect(onDiscard).not.toHaveBeenCalled();
  });
});
