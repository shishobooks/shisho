import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import PermissionMatrix from "./PermissionMatrix";

describe("PermissionMatrix", () => {
  it("lets a role be granted Shares Read and Write", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    render(<PermissionMatrix onChange={onChange} permissions={[]} />);

    const row = screen.getByText("Shares").closest("tr");
    expect(row).not.toBeNull();
    // Row toggle, then Read, then Write.
    const [, read, write] = within(row!).getAllByRole("checkbox");

    await user.click(read);
    expect(onChange).toHaveBeenLastCalledWith([
      { resource: "shares", operation: "read" },
    ]);

    await user.click(write);
    expect(onChange).toHaveBeenLastCalledWith([
      { resource: "shares", operation: "write" },
    ]);
  });

  it("names every checkbox by its resource and operation", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    render(<PermissionMatrix onChange={onChange} permissions={[]} />);

    await user.click(screen.getByRole("checkbox", { name: "Write Shares" }));
    expect(onChange).toHaveBeenLastCalledWith([
      { resource: "shares", operation: "write" },
    ]);
    expect(
      screen.getByRole("checkbox", { name: "All Shares permissions" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: "Read all resources" }),
    ).toBeInTheDocument();
  });
});
