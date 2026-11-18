import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import ChapterMarkers from "./ChapterMarkers";

const lefts = () =>
  screen.queryAllByTestId("chapter-marker").map((marker) => marker.style.left);

describe("ChapterMarkers", () => {
  it("marks each chapter where it starts along the bar", () => {
    render(<ChapterMarkers positions={[0.25, 0.5]} />);
    expect(lefts()).toEqual(["25%", "50%"]);
  });

  it("leaves out a chapter at the very start or end, where the bar's ends are", () => {
    render(<ChapterMarkers positions={[0, 0.4, 1]} />);
    expect(lefts()).toEqual(["40%"]);
  });

  it("takes a chapter a hair from either end as at that end", () => {
    // foliate nudges every section's start by Number.EPSILON.
    render(
      <ChapterMarkers positions={[Number.EPSILON, 0.4, 1 - Number.EPSILON]} />,
    );
    expect(lefts()).toEqual(["40%"]);
  });

  it("leaves out positions outside the bar", () => {
    render(<ChapterMarkers positions={[-0.1, 0.3, 1.2, Number.NaN]} />);
    expect(lefts()).toEqual(["30%"]);
  });

  it("is hidden from screen readers, which hear the chapter elsewhere", () => {
    const { container } = render(<ChapterMarkers positions={[0.5]} />);
    expect(container.firstElementChild).toHaveAttribute("aria-hidden", "true");
  });
});
