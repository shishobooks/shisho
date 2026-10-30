import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ReadingFilterSection } from "./FilterSheet";

describe("ReadingFilterSection", () => {
  it("picks statuses, No status included, from a combobox that shows what's chosen", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onToggleStatus = vi.fn();
    render(
      <ReadingFilterSection
        filters={{ statuses: ["read", "paused"] }}
        onRatingChange={vi.fn()}
        onToggleStatus={onToggleStatus}
      />,
    );

    const statuses = screen.getByRole("combobox", { name: "Reading status" });
    expect(statuses).toHaveTextContent("Paused, Read");
    await user.click(statuses);
    expect(screen.getByRole("option", { name: /^Read/ })).toHaveTextContent(
      "(chosen)",
    );

    await user.click(screen.getByRole("option", { name: "No status" }));
    expect(onToggleStatus).toHaveBeenLastCalledWith("none");
    await user.click(screen.getByRole("option", { name: /^Read/ }));
    expect(onToggleStatus).toHaveBeenLastCalledWith("read");
  });

  it("says when no status is chosen", () => {
    render(
      <ReadingFilterSection
        filters={{ statuses: [] }}
        onRatingChange={vi.fn()}
        onToggleStatus={vi.fn()}
      />,
    );
    expect(
      screen.getByRole("combobox", { name: "Reading status" }),
    ).toHaveTextContent("Any status");
  });

  it("drags each end of the rating range in quarter stars, Any at both ends", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onRatingChange = vi.fn();
    const { rerender } = render(
      <ReadingFilterSection
        filters={{ statuses: [] }}
        onRatingChange={onRatingChange}
        onToggleStatus={vi.fn()}
      />,
    );

    const lowest = screen.getByRole("slider", { name: "Lowest rating" });
    const highest = screen.getByRole("slider", { name: "Highest rating" });
    expect(lowest).toHaveAttribute("aria-valuetext", "Any");
    expect(highest).toHaveAttribute("aria-valuetext", "Any");
    expect(screen.getByText("Any rating")).toBeInTheDocument();

    await user.click(lowest);
    await user.keyboard("{ArrowRight}");
    expect(onRatingChange).toHaveBeenLastCalledWith(0.25, undefined);

    rerender(
      <ReadingFilterSection
        filters={{ statuses: [], ratingMin: 4 }}
        onRatingChange={onRatingChange}
        onToggleStatus={vi.fn()}
      />,
    );
    expect(lowest).toHaveAttribute("aria-valuetext", "4 stars");
    expect(screen.getByText("4+ stars")).toBeInTheDocument();

    await user.click(highest);
    await user.keyboard("{ArrowLeft}");
    expect(onRatingChange).toHaveBeenLastCalledWith(4, 4.75);

    // Back to the right end is Any again.
    rerender(
      <ReadingFilterSection
        filters={{ statuses: [], ratingMin: 4, ratingMax: 4.75 }}
        onRatingChange={onRatingChange}
        onToggleStatus={vi.fn()}
      />,
    );
    await user.keyboard("{End}");
    expect(onRatingChange).toHaveBeenLastCalledWith(4, undefined);
  });
});
