import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useSectionPaging } from "./useSectionPaging";

const sectionAt = (top: number) => {
  const section = document.createElement("section");
  section.getBoundingClientRect = () => ({ top }) as DOMRect;
  section.scrollIntoView = vi.fn();
  return section;
};

const page = (top: number) => {
  const onPageChange = vi.fn();
  const { result } = renderHook(() =>
    useSectionPaging<HTMLElement>(onPageChange),
  );
  const section = sectionAt(top);
  result.current.sectionRef.current = section;
  act(() => {
    result.current.sectionPageChange(2, { replace: true });
    vi.runAllTimers();
  });
  return { onPageChange, section };
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useSectionPaging", () => {
  it("keeps the page's scroll and passes the change on", () => {
    const { onPageChange } = page(100);

    expect(onPageChange).toHaveBeenCalledWith(2, {
      replace: true,
      preventScrollReset: true,
    });
  });

  it("brings a section scrolled above the screen back into view", () => {
    const { section } = page(-400);

    expect(section.scrollIntoView).toHaveBeenCalledWith({ block: "start" });
  });

  it("brings back a section whose top is under the sticky nav", () => {
    const onPageChange = vi.fn();
    const { result } = renderHook(() =>
      useSectionPaging<HTMLElement>(onPageChange),
    );
    // The scroll margin is the room the nav and banner take at the top.
    const section = sectionAt(40);
    section.style.scrollMarginTop = "80px";
    result.current.sectionRef.current = section;
    act(() => {
      result.current.sectionPageChange(2);
      vi.runAllTimers();
    });

    expect(section.scrollIntoView).toHaveBeenCalledWith({ block: "start" });
  });

  it("leaves a section that is already in view where it is", () => {
    const { section } = page(100);

    expect(section.scrollIntoView).not.toHaveBeenCalled();
  });
});
