import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { AdvancedPluginsDialog } from "./AdvancedPluginsDialog";

// Mock child sections to keep tests focused on dialog behaviour
vi.mock("./AdvancedOrderSection", () => ({
  AdvancedOrderSection: () => <div>Order Section Content</div>,
}));
vi.mock("./AdvancedRepositoriesSection", () => ({
  AdvancedRepositoriesSection: () => <div>Repositories Section Content</div>,
}));

describe("AdvancedPluginsDialog", () => {
  it("renders nothing when closed", () => {
    render(
      <AdvancedPluginsDialog
        onOpenChange={vi.fn()}
        onSectionChange={vi.fn()}
        open={false}
        section="order"
      />,
    );
    expect(
      screen.queryByText("Advanced plugin settings"),
    ).not.toBeInTheDocument();
  });

  it("shows the Order tab when section='order'", () => {
    render(
      <AdvancedPluginsDialog
        onOpenChange={vi.fn()}
        onSectionChange={vi.fn()}
        open={true}
        section="order"
      />,
    );
    expect(screen.getByText("Advanced plugin settings")).toBeInTheDocument();
    expect(screen.getByText("Order Section Content")).toBeInTheDocument();
    expect(
      screen.queryByText("Repositories Section Content"),
    ).not.toBeInTheDocument();
  });

  it("shows the Repositories tab when section='repositories'", () => {
    render(
      <AdvancedPluginsDialog
        onOpenChange={vi.fn()}
        onSectionChange={vi.fn()}
        open={true}
        section="repositories"
      />,
    );
    expect(
      screen.getByText("Repositories Section Content"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Order Section Content")).not.toBeInTheDocument();
  });

  it("reports the clicked tab through onSectionChange", async () => {
    const onSectionChange = vi.fn();
    const user = userEvent.setup({
      advanceTimers: vi.advanceTimersByTime,
    });

    render(
      <AdvancedPluginsDialog
        onOpenChange={vi.fn()}
        onSectionChange={onSectionChange}
        open={true}
        section="order"
      />,
    );

    await user.click(screen.getByRole("tab", { name: "Repositories" }));

    expect(onSectionChange).toHaveBeenCalledWith("repositories");
    // Controlled: the visible section follows the prop, not the click.
    expect(screen.getByText("Order Section Content")).toBeInTheDocument();
  });

  it("calls onOpenChange when the dialog requests close", async () => {
    const onOpenChange = vi.fn();
    const user = userEvent.setup({
      advanceTimers: vi.advanceTimersByTime,
    });

    render(
      <AdvancedPluginsDialog
        onOpenChange={onOpenChange}
        onSectionChange={vi.fn()}
        open={true}
        section="order"
      />,
    );

    await user.keyboard("{Escape}");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
