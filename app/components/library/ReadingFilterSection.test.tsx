import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ReadingFilterSection } from "./FilterSheet";

describe("ReadingFilterSection", () => {
  it("toggles statuses, No status included", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    render(
      <ReadingFilterSection
        filters={{ statuses: ["read"] }}
        onChange={onChange}
      />,
    );

    const group = screen.getByRole("group", { name: "Reading status" });
    expect(group).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Read" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    await user.click(screen.getByRole("button", { name: "No status" }));
    expect(onChange).toHaveBeenLastCalledWith({ statuses: ["read", "none"] });

    await user.click(screen.getByRole("button", { name: "Read" }));
    expect(onChange).toHaveBeenLastCalledWith({ statuses: [] });
  });

  it("sets each end of the rating range, offering only ends that make a range", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    render(
      <ReadingFilterSection
        filters={{ statuses: [], ratingMax: 4 }}
        onChange={onChange}
      />,
    );

    await user.click(screen.getByRole("combobox", { name: "Lowest rating" }));
    // Quarter stars, as ratings are.
    expect(
      screen.getByRole("option", { name: "3.75 stars" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("option", { name: "0.25 stars" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "4.5 stars" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    await user.click(screen.getByRole("option", { name: "4 stars" }));
    expect(onChange).toHaveBeenLastCalledWith({
      statuses: [],
      ratingMin: 4,
      ratingMax: 4,
    });

    await user.click(screen.getByRole("combobox", { name: "Highest rating" }));
    await user.click(screen.getByRole("option", { name: "Any" }));
    expect(onChange).toHaveBeenLastCalledWith({ statuses: [] });
  });
});
