import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import ProgressMarks, { type ProgressMark } from "./ProgressMarks";

const marker = (position: number, label = `At ${position}`): ProgressMark => ({
  kind: "chapter",
  position,
  label,
  onSelect: vi.fn(),
});

const bookmark = (position: number, note?: string): ProgressMark => ({
  kind: "bookmark",
  position,
  label: `Bookmark at ${position}`,
  note,
  onSelect: vi.fn(),
});

const lefts = () =>
  screen.queryAllByTestId("chapter-marker").map((m) => m.style.left);

describe("ProgressMarks", () => {
  it("marks each chapter where it starts along the bar", () => {
    render(<ProgressMarks markers={[marker(0.25), marker(0.5)]} />);
    expect(lefts()).toEqual(["25%", "50%"]);
  });

  it("leaves out a chapter at the very start or end, where the bar's ends are", () => {
    render(<ProgressMarks markers={[marker(0), marker(0.4), marker(1)]} />);
    expect(lefts()).toEqual(["40%"]);
  });

  it("takes a chapter a hair from either end as at that end", () => {
    // foliate nudges every section's start by Number.EPSILON.
    render(
      <ProgressMarks
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
    render(<ProgressMarks markers={[marker(0.25)]} rtl />);
    const mark = screen.getByTestId("chapter-marker");
    expect(mark.style.right).toBe("25%");
    expect(mark.style.left).toBe("");
  });

  it("leaves out positions outside the bar", () => {
    render(
      <ProgressMarks
        markers={[marker(-0.1), marker(0.3), marker(1.2), marker(Number.NaN)]}
      />,
    );
    expect(lefts()).toEqual(["30%"]);
  });

  it("names the chapter when the pointer rests on its mark", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<ProgressMarks markers={[marker(0.5, "Chapter 3")]} />);

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
        <ProgressMarks markers={[chapter]} />
      </div>,
    );

    await user.click(screen.getByTestId("chapter-marker"));

    expect(chapter.onSelect).toHaveBeenCalledTimes(1);
    expect(barClick).not.toHaveBeenCalled();
  });

  it("stays out of the tab order, which the chapter menu already serves", () => {
    render(<ProgressMarks markers={[marker(0.5, "Chapter 3")]} />);
    expect(screen.getByTestId("chapter-marker")).toHaveAttribute(
      "tabindex",
      "-1",
    );
  });
});

describe("ProgressMarks bookmarks", () => {
  it("marks each bookmark apart from the chapters", () => {
    render(<ProgressMarks markers={[marker(0.25), bookmark(0.6)]} />);
    expect(
      screen.getAllByTestId("bookmark-marker").map((m) => m.style.left),
    ).toEqual(["60%"]);
    expect(lefts()).toEqual(["25%"]);
  });

  it("shows a bookmark right by either end, where a chapter mark would sit on the bar's end", () => {
    // A bookmark seconds into a long book.
    render(<ProgressMarks markers={[bookmark(0.0001), bookmark(0.9999)]} />);
    expect(screen.getAllByTestId("bookmark-marker")).toHaveLength(2);
  });

  it("shows a bookmark's note under its name on hover", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<ProgressMarks markers={[bookmark(0.5, "The twist")]} />);

    await user.hover(screen.getByTestId("bookmark-marker"));

    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent("Bookmark at 0.5");
    expect(tooltip).toHaveTextContent("The twist");
  });

  it("goes to the bookmark on a click", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const mark = bookmark(0.5);
    render(<ProgressMarks markers={[mark]} />);

    await user.click(screen.getByTestId("bookmark-marker"));

    expect(mark.onSelect).toHaveBeenCalledTimes(1);
  });
});

describe("ProgressMarks close together", () => {
  it("keeps each mark's hit area from reaching over its neighbors", () => {
    render(
      <ProgressMarks
        markers={[marker(0.5), marker(0.503), bookmark(0.506), marker(0.9)]}
      />,
    );
    // No wider than the gap to the nearest mark, so pointing at a mark's
    // center picks that mark.
    const widths = [
      ...screen.getAllByTestId("chapter-marker"),
      ...screen.getAllByTestId("bookmark-marker"),
    ].map((m) => m.style.maxWidth);
    expect(widths).toEqual(["0.3%", "0.3%", "", "0.3%"]);
  });
});

describe("ProgressMarks on a touch screen", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // A device whose only pointer can't hover, such as a phone.
  const noHover = () =>
    vi.stubGlobal(
      "matchMedia",
      (query: string) =>
        ({
          matches: query === "(hover: none)",
          addEventListener: () => {},
          removeEventListener: () => {},
        }) as unknown as MediaQueryList,
    );

  it("goes to the mark on the first tap, with no tooltip in the way", async () => {
    // iOS takes a tap that shows something on hover as a hover alone.
    noHover();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const mark = bookmark(0.5, "The twist");
    render(<ProgressMarks markers={[mark]} />);

    const target = screen.getByTestId("bookmark-marker");
    // Not a tooltip's trigger at all, which Radix marks with data-state.
    expect(target).not.toHaveAttribute("data-state");
    await user.click(target);

    expect(mark.onSelect).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  });
});
