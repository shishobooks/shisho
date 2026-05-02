import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as React from "react";
import { describe, expect, it, vi } from "vitest";

import { IdentifySectionBanner } from "./IdentifySectionBanner";

function renderBanner(
  overrides: Partial<React.ComponentProps<typeof IdentifySectionBanner>> = {},
) {
  const handlers = {
    onToggleCollapse: vi.fn(),
    onCheckedChange: vi.fn(),
  };
  render(
    <IdentifySectionBanner
      checkboxState={true}
      collapsed={false}
      hint="applies to all files"
      label="BOOK"
      selectedCount={2}
      totalCount={5}
      {...handlers}
      {...overrides}
    />,
  );
  return handlers;
}

describe("IdentifySectionBanner", () => {
  it("renders label, hint, and selection count", () => {
    renderBanner();
    expect(screen.getByText("BOOK")).toBeInTheDocument();
    expect(screen.getByText("applies to all files")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(screen.getByText(/of 5 selected/)).toBeInTheDocument();
  });

  it("toggles collapse when banner button is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onToggleCollapse } = renderBanner();
    await user.click(
      screen.getByRole("button", { name: /toggle book section/i }),
    );
    expect(onToggleCollapse).toHaveBeenCalledTimes(1);
  });

  it("renders chevron rotated when collapsed", () => {
    const { rerender } = render(
      <IdentifySectionBanner
        checkboxState={false}
        collapsed={true}
        label="BOOK"
        onCheckedChange={() => {}}
        onToggleCollapse={() => {}}
        selectedCount={0}
        totalCount={0}
      />,
    );
    const button = screen.getByRole("button", { name: /toggle/i });
    const chevron = button.querySelector("svg");
    expect(chevron?.getAttribute("class")).toContain("-rotate-90");

    rerender(
      <IdentifySectionBanner
        checkboxState={false}
        collapsed={false}
        label="BOOK"
        onCheckedChange={() => {}}
        onToggleCollapse={() => {}}
        selectedCount={0}
        totalCount={0}
      />,
    );
    const chevron2 = button.querySelector("svg");
    expect(chevron2?.getAttribute("class")).not.toContain("-rotate-90");
  });

  it("calls onCheckedChange and stops propagation when checkbox clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onToggleCollapse, onCheckedChange } = renderBanner({
      checkboxState: false,
    });
    await user.click(screen.getByRole("checkbox"));
    expect(onCheckedChange).toHaveBeenCalledWith(true);
    expect(onToggleCollapse).not.toHaveBeenCalled();
  });

  it("renders indeterminate checkbox state", () => {
    renderBanner({ checkboxState: "indeterminate" });
    expect(screen.getByRole("checkbox")).toHaveAttribute(
      "data-state",
      "indeterminate",
    );
  });
});
