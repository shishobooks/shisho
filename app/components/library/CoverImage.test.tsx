import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import CoverImage from "./CoverImage";

let resize: () => void;
let width = 222;
let height = 334;

beforeEach(() => {
  width = 222;
  height = 334;
  vi.spyOn(
    HTMLImageElement.prototype,
    "getBoundingClientRect",
  ).mockImplementation(() => new DOMRect(0, 0, width, height));
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
  vi.stubGlobal("devicePixelRatio", 1);
});

describe("CoverImage", () => {
  it("requests only the tier needed for the rendered cover", () => {
    render(<CoverImage alt="Cover" src="/api/books/7/cover?v=12-1" />);
    expect(screen.getByAltText("Cover")).toHaveAttribute(
      "src",
      "/api/books/7/cover?v=12-1&size=512&aspect=book&r=1",
    );
  });

  it("waits for dimensions and accounts for screen density and resizing", () => {
    width = height = 0;
    render(<CoverImage alt="Cover" src="/api/books/7/cover?v=12-1" />);
    expect(screen.getByAltText("Cover")).not.toHaveAttribute("src");
    width = 96;
    height = 144;
    vi.stubGlobal("devicePixelRatio", 2);
    act(() => resize());
    expect(screen.getByAltText("Cover")).toHaveAttribute(
      "src",
      "/api/books/7/cover?v=12-1&size=512&aspect=book&r=1",
    );
    height = 600;
    act(() => resize());
    expect(screen.getByAltText("Cover")).toHaveAttribute(
      "src",
      "/api/books/7/cover?v=12-1&size=2048&r=1",
    );
  });

  it("retains the version and token when the source changes", () => {
    const { rerender } = render(
      <CoverImage alt="Cover" src="/api/share/tok/files/42/cover?v=1" />,
    );
    rerender(
      <CoverImage alt="Cover" src="/api/share/tok/files/42/cover?v=2" />,
    );
    expect(screen.getByAltText("Cover")).toHaveAttribute(
      "src",
      "/api/share/tok/files/42/cover?v=2&size=512&aspect=book&r=1",
    );
  });
});
