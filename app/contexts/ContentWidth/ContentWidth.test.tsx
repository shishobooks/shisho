import { act, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ContentWidth, ContentWidthProvider, useContentWidth } from ".";

const Reader = () => <div>{String(useContentWidth((width) => width))}</div>;

let resize: () => void = () => {};

const renderMeasured = (width: number) => {
  const clientWidth = vi
    .spyOn(HTMLElement.prototype, "clientWidth", "get")
    .mockReturnValue(width);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: () => void) {
        resize = callback;
      }
      observe() {}
      disconnect() {}
    },
  );
  render(
    <ContentWidthProvider>
      <ContentWidth>
        <Reader />
      </ContentWidth>
    </ContentWidthProvider>,
  );
  return clientWidth;
};

afterEach(() => {
  document.body.removeAttribute("data-scroll-locked");
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("ContentWidth", () => {
  it("reports the width its content gets, and changes to it", () => {
    const clientWidth = renderMeasured(1000);
    expect(screen.getByText("1000")).toBeInTheDocument();

    clientWidth.mockReturnValue(1200);
    act(() => resize());
    expect(screen.getByText("1200")).toBeInTheDocument();
  });

  it("ignores the narrowing while a modal locks scrolling", () => {
    const clientWidth = renderMeasured(1000);

    document.body.setAttribute("data-scroll-locked", "1");
    clientWidth.mockReturnValue(985);
    act(() => resize());
    expect(screen.getByText("1000")).toBeInTheDocument();

    document.body.removeAttribute("data-scroll-locked");
    clientWidth.mockReturnValue(1000);
    act(() => resize());
    expect(screen.getByText("1000")).toBeInTheDocument();
  });
});
