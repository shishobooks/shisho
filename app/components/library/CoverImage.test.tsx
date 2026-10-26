import { act, fireEvent, render, screen } from "@testing-library/react";
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

// Loads the image as if the server sent one of the given pixel size.
const loadAs = (
  img: HTMLElement,
  naturalWidth: number,
  naturalHeight: number,
) => {
  Object.defineProperty(img, "naturalWidth", {
    configurable: true,
    value: naturalWidth,
  });
  Object.defineProperty(img, "naturalHeight", {
    configurable: true,
    value: naturalHeight,
  });
  fireEvent.load(img);
};

describe("CoverImage cropping into its box", () => {
  const page = (size: number) => `/page/0?v=1&size=${size}`;
  const renderPage = () =>
    render(
      <CoverImage
        alt="Page"
        src="/page/0?v=1"
        style={{ objectFit: "cover" }}
        thumbnailUrl={(size) => page(size)}
      />,
    );

  it("asks for a larger thumbnail when a landscape page fills a portrait box by its height", () => {
    renderPage();
    const img = screen.getByAltText("Page");
    expect(img).toHaveAttribute("src", page(512));

    // 512x256 drawn 334px tall is about 668px wide: short of what it needs.
    loadAs(img, 512, 256);
    expect(img).toHaveAttribute("src", page(1024));

    // A resize that still fits keeps the larger size rather than going back.
    act(() => resize());
    expect(img).toHaveAttribute("src", page(1024));
  });

  it("keeps the size for a portrait page that fits it", () => {
    renderPage();
    const img = screen.getByAltText("Page");

    loadAs(img, 341, 512);
    expect(img).toHaveAttribute("src", page(512));
  });

  it("keeps the size when the image overshoots its box only slightly", () => {
    // A box a little off 2:3 still counts as a book cover, so a 2:3 crop is
    // drawn slightly past it: about 517px tall for a 512px thumbnail.
    width = 344;
    height = 500;
    renderPage();
    const img = screen.getByAltText("Page");
    expect(img).toHaveAttribute("src", page(512));

    loadAs(img, 341, 512);
    expect(img).toHaveAttribute("src", page(512));
  });

  it("still calls the caller's onLoad", () => {
    const onLoad = vi.fn();
    render(
      <CoverImage alt="Cover" onLoad={onLoad} src="/api/books/7/cover?v=1" />,
    );

    loadAs(screen.getByAltText("Cover"), 341, 512);
    expect(onLoad).toHaveBeenCalledOnce();
  });

  it("starts over for a new image", () => {
    const { rerender } = renderPage();
    const img = screen.getByAltText("Page");
    loadAs(img, 512, 256);
    expect(img).toHaveAttribute("src", page(1024));

    rerender(
      <CoverImage
        alt="Page"
        src="/page/1?v=1"
        style={{ objectFit: "cover" }}
        thumbnailUrl={(size) => `/page/1?v=1&size=${size}`}
      />,
    );
    expect(img).toHaveAttribute("src", "/page/1?v=1&size=512");
  });

  it("keeps the size when the source is smaller than the thumbnail", () => {
    renderPage();
    const img = screen.getByAltText("Page");

    // The server sends no more pixels than the source has.
    loadAs(img, 300, 150);
    expect(img).toHaveAttribute("src", page(512));
  });
});

describe("CoverImage", () => {
  it("requests only the tier needed for the rendered cover", () => {
    render(<CoverImage alt="Cover" src="/api/books/7/cover?v=12-1" />);
    expect(screen.getByAltText("Cover")).toHaveAttribute(
      "src",
      "/api/books/7/cover?v=12-1&size=512&aspect=book&r=2",
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
      "/api/books/7/cover?v=12-1&size=512&aspect=book&r=2",
    );
    height = 600;
    act(() => resize());
    expect(screen.getByAltText("Cover")).toHaveAttribute(
      "src",
      "/api/books/7/cover?v=12-1&size=2048&r=2",
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
      "/api/share/tok/files/42/cover?v=2&size=512&aspect=book&r=2",
    );
  });
});
