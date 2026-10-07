import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { Badge, BadgeRemoveButton } from "./badge";

describe("BadgeRemoveButton", () => {
  it("renders a labeled, non-submitting X button sized for a badge", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClick = vi.fn();
    render(
      <Badge>
        Fantasy
        <BadgeRemoveButton aria-label="Remove Fantasy" onClick={onClick} />
      </Badge>,
    );

    const button = screen.getByRole("button", { name: "Remove Fantasy" });
    expect(button).toHaveAttribute("type", "button");
    expect(button.className).toContain("shrink-0");
    expect(button.className).toContain("focus-visible:ring-1");
    expect(button.className).not.toContain("inline-flex");
    expect(button.querySelector("svg")).toHaveClass("size-3");

    await user.click(button);
    expect(onClick).toHaveBeenCalledOnce();
  });
});
