import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SizeSelector } from "@/components/library/SizeSelector";

describe("SizeSelector", () => {
  it("renders one button per size", () => {
    render(<SizeSelector value="m" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "S" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "M" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "L" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "XL" })).toBeInTheDocument();
  });

  it("marks the active size with aria-pressed", () => {
    render(<SizeSelector value="l" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "L" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "M" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
  });

  it("calls onChange with the clicked size", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    render(<SizeSelector value="m" onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: "L" }));
    expect(onChange).toHaveBeenCalledWith("l");
  });
});
