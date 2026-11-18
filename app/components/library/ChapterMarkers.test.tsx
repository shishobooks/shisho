import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import ChapterMarkers, { type ChapterMarker } from "./ChapterMarkers";

const marker = (position: number, label = `At ${position}`): ChapterMarker => ({
  position,
  label,
  onSelect: vi.fn(),
});

const lefts = () =>
  screen.queryAllByTestId("chapter-marker").map((m) => m.style.left);

describe("ChapterMarkers", () => {
  it("marks each chapter where it starts along the bar", () => {
    render(<ChapterMarkers markers={[marker(0.25), marker(0.5)]} />);
    expect(lefts()).toEqual(["25%", "50%"]);
  });

  it("leaves out a chapter at the very start or end, where the bar's ends are", () => {
    render(<ChapterMarkers markers={[marker(0), marker(0.4), marker(1)]} />);
    expect(lefts()).toEqual(["40%"]);
  });

  it("takes a chapter a hair from either end as at that end", () => {
    // foliate nudges every section's start by Number.EPSILON.
    render(
      <ChapterMarkers
        markers={[
          marker(Number.EPSILON),
          marker(0.4),
          marker(1 - Number.EPSILON),
        ]}
      />,
    );
    expect(lefts()).toEqual(["40%"]);
  });

  it("runs from the right in a bar that fills right to left", () => {
    render(<ChapterMarkers markers={[marker(0.25)]} rtl />);
    const mark = screen.getByTestId("chapter-marker");
    expect(mark.style.right).toBe("25%");
    expect(mark.style.left).toBe("");
  });

  it("leaves out positions outside the bar", () => {
    render(
      <ChapterMarkers
        markers={[marker(-0.1), marker(0.3), marker(1.2), marker(Number.NaN)]}
      />,
    );
    expect(lefts()).toEqual(["30%"]);
  });

  it("names the chapter when the pointer rests on its mark", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<ChapterMarkers markers={[marker(0.5, "Chapter 3")]} />);

    await user.hover(screen.getByTestId("chapter-marker"));

    expect(await screen.findByRole("tooltip")).toHaveTextContent("Chapter 3");
  });

  it("goes to the chapter's start on a click, rather than the place clicked on the bar", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const chapter = marker(0.5, "Chapter 3");
    const barClick = vi.fn();
    render(
      // Stands in for a reader's progress bar, which seeks on a click.
      <div onClick={barClick}>
        <ChapterMarkers markers={[chapter]} />
      </div>,
    );

    await user.click(screen.getByTestId("chapter-marker"));

    expect(chapter.onSelect).toHaveBeenCalledTimes(1);
    expect(barClick).not.toHaveBeenCalled();
  });

  it("stays out of the tab order, which the chapter menu already serves", () => {
    render(<ChapterMarkers markers={[marker(0.5, "Chapter 3")]} />);
    expect(screen.getByTestId("chapter-marker")).toHaveAttribute(
      "tabindex",
      "-1",
    );
  });
});
