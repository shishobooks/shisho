import { AdvancedPluginsDialog } from "./AdvancedPluginsDialog";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

// Mock child sections to keep tests focused on dialog behaviour
vi.mock("./AdvancedOrderSection", () => ({
  AdvancedOrderSection: () => <div>Order Section Content</div>,
}));
vi.mock("./AdvancedRepositoriesSection", () => ({
  AdvancedRepositoriesSection: () => <div>Repositories Section Content</div>,
}));

describe("AdvancedPluginsDialog", () => {
  it("renders nothing when closed", () => {
    render(<AdvancedPluginsDialog onOpenChange={vi.fn()} open={false} />);
    expect(
      screen.queryByText("Advanced plugin settings"),
    ).not.toBeInTheDocument();
  });

  it("shows the dialog with Order tab active by default", () => {
    render(<AdvancedPluginsDialog onOpenChange={vi.fn()} open={true} />);
    expect(screen.getByText("Advanced plugin settings")).toBeInTheDocument();
    expect(screen.getByText("Order Section Content")).toBeInTheDocument();
    expect(
      screen.queryByText("Repositories Section Content"),
    ).not.toBeInTheDocument();
  });

  it("shows the Repositories tab when defaultSection='repositories'", () => {
    render(
      <AdvancedPluginsDialog
        defaultSection="repositories"
        onOpenChange={vi.fn()}
        open={true}
      />,
    );
    expect(
      screen.getByText("Repositories Section Content"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Order Section Content")).not.toBeInTheDocument();
  });

  it("switches from Order to Repositories when the tab is clicked", async () => {
    const user = userEvent.setup({
      advanceTimers: vi.advanceTimersByTime,
    });

    render(<AdvancedPluginsDialog onOpenChange={vi.fn()} open={true} />);

    // Initially on Order
    expect(screen.getByText("Order Section Content")).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "Repositories" }));

    expect(
      screen.getByText("Repositories Section Content"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Order Section Content")).not.toBeInTheDocument();
  });

  it("calls onOpenChange when the dialog requests close", async () => {
    const onOpenChange = vi.fn();
    const user = userEvent.setup({
      advanceTimers: vi.advanceTimersByTime,
    });

    render(<AdvancedPluginsDialog onOpenChange={onOpenChange} open={true} />);

    await user.keyboard("{Escape}");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
