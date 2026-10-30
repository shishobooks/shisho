import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ReadingFilterSection } from "./FilterSheet";

describe("ReadingFilterSection", () => {
  it("picks statuses, No status included, from a combobox that shows what's chosen", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    render(
      <ReadingFilterSection
        filters={{ statuses: ["read", "paused"] }}
        onChange={onChange}
      />,
    );

    const statuses = screen.getByRole("combobox", { name: "Reading status" });
    expect(statuses).toHaveTextContent("Paused, Read");
    await user.click(statuses);
    expect(screen.getByRole("option", { name: /^Read/ })).toHaveTextContent(
      "(chosen)",
    );

    await user.click(screen.getByRole("option", { name: "No status" }));
    expect(onChange).toHaveBeenLastCalledWith({
      statuses: ["read", "paused", "none"],
    });

    await user.click(screen.getByRole("option", { name: /^Read/ }));
    expect(onChange).toHaveBeenLastCalledWith({ statuses: ["paused"] });
  });

  it("says when no status is chosen", () => {
    render(
      <ReadingFilterSection filters={{ statuses: [] }} onChange={vi.fn()} />,
    );
    expect(
      screen.getByRole("combobox", { name: "Reading status" }),
    ).toHaveTextContent("Any status");
  });

  it("drags each end of the rating range in quarter stars, Any at both ends", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    const { rerender } = render(
      <ReadingFilterSection filters={{ statuses: [] }} onChange={onChange} />,
    );

    const lowest = screen.getByRole("slider", { name: "Lowest rating" });
    const highest = screen.getByRole("slider", { name: "Highest rating" });
    expect(lowest).toHaveAttribute("aria-valuetext", "Any");
    expect(highest).toHaveAttribute("aria-valuetext", "Any");
    expect(screen.getByText("Any rating")).toBeInTheDocument();

    await user.click(lowest);
    await user.keyboard("{ArrowRight}");
    expect(onChange).toHaveBeenLastCalledWith({
      statuses: [],
      ratingMin: 0.25,
    });

    rerender(
      <ReadingFilterSection
        filters={{ statuses: [], ratingMin: 4 }}
        onChange={onChange}
      />,
    );
    expect(lowest).toHaveAttribute("aria-valuetext", "4 stars");
    expect(screen.getByText("4+ stars")).toBeInTheDocument();

    await user.click(highest);
    await user.keyboard("{ArrowLeft}");
    expect(onChange).toHaveBeenLastCalledWith({
      statuses: [],
      ratingMin: 4,
      ratingMax: 4.75,
    });

    // Back to the right end is Any again.
    rerender(
      <ReadingFilterSection
        filters={{ statuses: [], ratingMin: 4, ratingMax: 4.75 }}
        onChange={onChange}
      />,
    );
    await user.keyboard("{End}");
    expect(onChange).toHaveBeenLastCalledWith({ statuses: [], ratingMin: 4 });
  });
});
