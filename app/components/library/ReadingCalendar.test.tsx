import { render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ReadingHeatmap } from "./ReadingCalendar";

const heatmap = (props: {
  from?: string;
  to?: string;
  selectedDay?: string | null;
}) => (
  <ReadingHeatmap
    days={[]}
    from={props.from ?? "2025-11-16"}
    onSelectDay={() => {}}
    period="the last 12 months"
    selectedDay={props.selectedDay ?? null}
    to={props.to ?? "2026-11-14"}
    today={props.to ?? "2026-11-14"}
  />
);

// jsdom has no layout. Lay the year out in a scroller 300px wide, with the
// weekday labels taking its first 30px, where the squares in `inView` sit
// inside it and every other square sits far past its right edge.
const layOut = (inView: string[]) => {
  const scrollLeft = new WeakMap<Element, number>();
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(1200);
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(300);
  vi.spyOn(Element.prototype, "scrollLeft", "get").mockImplementation(function (
    this: Element,
  ) {
    return scrollLeft.get(this) ?? 0;
  });
  vi.spyOn(Element.prototype, "scrollLeft", "set").mockImplementation(function (
    this: Element,
    value: number,
  ) {
    scrollLeft.set(this, value);
  });
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(
    function (this: Element) {
      const day = this.getAttribute("data-day");
      if (day) {
        return new DOMRect(inView.includes(day) ? 100 : 800, 0, 20, 20);
      }
      if (this.textContent === "MonWedFri") return new DOMRect(0, 0, 30, 160);
      return new DOMRect(0, 0, 300, 200);
    },
  );
  return () =>
    document.querySelector("[data-day]")!.parentElement!.parentElement!
      .scrollLeft;
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("ReadingHeatmap", () => {
  it("leaves the year where it is for a day already in view", () => {
    const scrollLeft = layOut(["2026-11-14", "2026-10-07"]);
    const { rerender } = render(heatmap({}));
    expect(scrollLeft()).toBe(0);

    rerender(heatmap({ selectedDay: "2026-10-07" }));
    expect(scrollLeft()).toBe(0);
  });

  it("scrolls a day out of view to the middle", () => {
    const scrollLeft = layOut(["2026-11-14"]);
    const { rerender } = render(heatmap({}));
    rerender(heatmap({ selectedDay: "2026-03-02" }));
    // From 800px, less half the 300px scroller.
    expect(scrollLeft()).toBe(650);
  });

  it("leaves out a month whose label would start in the last week", () => {
    // Dec 1, 2026 is a Tuesday, so it falls in the last week shown.
    render(heatmap({ from: "2025-12-01", to: "2026-12-02" }));
    const months = [...document.querySelectorAll("[aria-hidden] > span")]
      .map((s) => s.textContent)
      .filter((t) => t && /^[A-Z][a-z]{2}$/.test(t));
    expect(months[0]).toBe("Dec");
    expect(months.filter((m) => m === "Dec")).toHaveLength(1);
  });
});
