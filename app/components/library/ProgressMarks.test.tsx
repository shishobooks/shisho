import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import ProgressMarks, { type ProgressMark } from "./ProgressMarks";

const chapterMark = (
  position: number,
  label = `At ${position}`,
): ProgressMark => ({
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
  screen.queryAllByTestId("chapter-mark").map((m) => m.style.left);

describe("ProgressMarks", () => {
  it("marks each chapter where it starts along the bar", () => {
    render(<ProgressMarks marks={[chapterMark(0.25), chapterMark(0.5)]} />);
    expect(lefts()).toEqual(["25%", "50%"]);
  });

  it("leaves out a chapter at the very start or end, where the bar's ends are", () => {
    render(
      <ProgressMarks
        marks={[chapterMark(0), chapterMark(0.4), chapterMark(1)]}
      />,
    );
    expect(lefts()).toEqual(["40%"]);
  });

  it("takes a chapter a hair from either end as at that end", () => {
    // foliate nudges every section's start by Number.EPSILON.
    render(
      <ProgressMarks
        marks={[
          chapterMark(Number.EPSILON),
          chapterMark(0.4),
          chapterMark(1 - Number.EPSILON),
        ]}
      />,
    );
    expect(lefts()).toEqual(["40%"]);
  });

  it("runs from the right in a bar that fills right to left", () => {
    render(<ProgressMarks marks={[chapterMark(0.25)]} rtl />);
    const mark = screen.getByTestId("chapter-mark");
    expect(mark.style.right).toBe("25%");
    expect(mark.style.left).toBe("");
  });

  it("leaves out positions outside the bar", () => {
    render(
      <ProgressMarks
        marks={[
          chapterMark(-0.1),
          chapterMark(0.3),
          chapterMark(1.2),
          chapterMark(Number.NaN),
        ]}
      />,
    );
    expect(lefts()).toEqual(["30%"]);
  });

  it("names the chapter when the pointer rests on its mark", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<ProgressMarks marks={[chapterMark(0.5, "Chapter 3")]} />);

    await user.hover(screen.getByTestId("chapter-mark"));

    expect(await screen.findByRole("tooltip")).toHaveTextContent("Chapter 3");
  });

  it("goes to the chapter's start on a click, rather than the place clicked on the bar", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const chapter = chapterMark(0.5, "Chapter 3");
    const barClick = vi.fn();
    render(
      // Stands in for a reader's progress bar, which seeks on a click.
      <div onClick={barClick}>
        <ProgressMarks marks={[chapter]} />
      </div>,
    );

    await user.click(screen.getByTestId("chapter-mark"));

    expect(chapter.onSelect).toHaveBeenCalledTimes(1);
    expect(barClick).not.toHaveBeenCalled();
  });

  it("stays out of the tab order, which the chapter menu already serves", () => {
    render(<ProgressMarks marks={[chapterMark(0.5, "Chapter 3")]} />);
    expect(screen.getByTestId("chapter-mark")).toHaveAttribute(
      "tabindex",
      "-1",
    );
  });
});

describe("ProgressMarks bookmarks", () => {
  it("marks each bookmark apart from the chapters", () => {
    render(<ProgressMarks marks={[chapterMark(0.25), bookmark(0.6)]} />);
    expect(
      screen.getAllByTestId("bookmark-mark").map((m) => m.style.left),
    ).toEqual(["60%"]);
    expect(lefts()).toEqual(["25%"]);
  });

  it("shows a bookmark right by either end, where a chapter mark would sit on the bar's end", () => {
    // A bookmark seconds into a long book.
    render(<ProgressMarks marks={[bookmark(0.0001), bookmark(0.9999)]} />);
    expect(screen.getAllByTestId("bookmark-mark")).toHaveLength(2);
  });

  it("shows a bookmark's note under its name on hover", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<ProgressMarks marks={[bookmark(0.5, "The twist")]} />);

    await user.hover(screen.getByTestId("bookmark-mark"));

    const tooltip = await screen.findByRole("tooltip");
    expect(tooltip).toHaveTextContent("Bookmark at 0.5");
    expect(tooltip).toHaveTextContent("The twist");
  });

  it("goes to the bookmark on a click", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const mark = bookmark(0.5);
    render(<ProgressMarks marks={[mark]} />);

    await user.click(screen.getByTestId("bookmark-mark"));

    expect(mark.onSelect).toHaveBeenCalledTimes(1);
  });
});

describe("ProgressMarks close together", () => {
  it("keeps each mark's hit area from reaching over its neighbors", () => {
    render(
      <ProgressMarks
        marks={[
          chapterMark(0.5),
          chapterMark(0.503),
          bookmark(0.506),
          chapterMark(0.9),
        ]}
      />,
    );
    // No wider than the gap to the nearest mark, so pointing at a mark's
    // center picks that mark.
    const widths = [
      ...screen.getAllByTestId("chapter-mark"),
      ...screen.getAllByTestId("bookmark-mark"),
    ].map((m) => m.style.maxWidth);
    expect(widths).toEqual(["0.3%", "0.3%", "", ""]);
  });

  it("keeps a chapter's hit area whole under a bookmark at the same place", () => {
    // The bookmark draws on top; the chapter's tick stays reachable around it.
    render(<ProgressMarks marks={[chapterMark(0.5), bookmark(0.5)]} />);
    expect(screen.getByTestId("chapter-mark").style.maxWidth).toBe("");
    expect(screen.getByTestId("bookmark-mark").style.maxWidth).toBe("");
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
    render(<ProgressMarks marks={[mark]} />);

    const target = screen.getByTestId("bookmark-mark");
    // Not a tooltip's trigger at all, which Radix marks with data-state.
    expect(target).not.toHaveAttribute("data-state");
    await user.click(target);

    expect(mark.onSelect).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
  });
});

describe("ProgressMarks at one place", () => {
  it("names one mark after every chapter starting there, going to the first", async () => {
    // A part and its first chapter on one page.
    const part = chapterMark(0.4, "Part One");
    const chapter = chapterMark(0.4, "Chapter 1");
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<ProgressMarks marks={[part, chapter]} />);

    const marks = screen.getAllByTestId("chapter-mark");
    expect(marks).toHaveLength(1);
    expect(marks[0]).toHaveAccessibleName("Part One / Chapter 1");
    await user.click(marks[0]);
    expect(part.onSelect).toHaveBeenCalledTimes(1);
    expect(chapter.onSelect).not.toHaveBeenCalled();
  });

  it("keeps the notes of two bookmarks at one moment on their one mark", () => {
    render(
      <ProgressMarks
        marks={[bookmark(0.4, "First note"), bookmark(0.4, "Second note")]}
      />,
    );
    const marks = screen.getAllByTestId("bookmark-mark");
    expect(marks).toHaveLength(1);
    expect(marks[0]).toHaveAccessibleName(
      "Bookmark at 0.4: First note / Second note",
    );
  });

  it("keeps a bookmark and a chapter at one place as two marks", () => {
    render(<ProgressMarks marks={[chapterMark(0.4), bookmark(0.4)]} />);
    expect(screen.getAllByTestId("chapter-mark")).toHaveLength(1);
    expect(screen.getAllByTestId("bookmark-mark")).toHaveLength(1);
  });
});

describe("ProgressMarks near the ends", () => {
  it("shows a chapter close to an end that isn't at it", () => {
    // Chapters 18 seconds from either end of a ten-hour audiobook.
    render(
      <ProgressMarks marks={[chapterMark(0.0005), chapterMark(0.9995)]} />,
    );
    expect(screen.getAllByTestId("chapter-mark")).toHaveLength(2);
  });
});

describe("ProgressMarks over a slider", () => {
  it("keeps a press on a mark from reaching the slider under it", () => {
    const sliderPress = vi.fn();
    render(
      // Stands in for the audiobook's seek bar, which starts a drag on a press.
      <div onPointerDown={sliderPress}>
        <ProgressMarks marks={[chapterMark(0.5)]} />
      </div>,
    );
    fireEvent.pointerDown(screen.getByTestId("chapter-mark"));
    expect(sliderPress).not.toHaveBeenCalled();
  });
});
