import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { File } from "@/types";

import CoverGalleryTabs from "./CoverGalleryTabs";

function makeFile(overrides: Partial<File> = {}): File {
  return {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    library_id: 1,
    book_id: 1,
    filepath: "/library/book.epub",
    file_type: "epub",
    file_role: "main",
    filesize_bytes: 1000,
    cover_image_filename: "cover.jpg",
    ...overrides,
  };
}

describe("CoverGalleryTabs", () => {
  it("renders cover image with stable URL when no cacheKey is provided", () => {
    const files = [
      makeFile({ id: 1, file_type: "epub", filepath: "/library/a.epub" }),
      makeFile({ id: 2, file_type: "m4b", filepath: "/library/b.m4b" }),
    ];

    const { container } = render(<CoverGalleryTabs files={files} />);

    const img = container.querySelector("img");
    expect(img).not.toBeNull();
    expect(img?.getAttribute("src")).toBe("/api/books/files/1/cover");
  });

  it("re-mounts cover image when cacheKey changes", () => {
    const files = [
      makeFile({ id: 1, file_type: "epub", filepath: "/library/a.epub" }),
      makeFile({ id: 2, file_type: "m4b", filepath: "/library/b.m4b" }),
    ];

    const { container, rerender } = render(
      <CoverGalleryTabs cacheKey={111} files={files} />,
    );
    const firstImg = container.querySelector("img");
    expect(firstImg).not.toBeNull();
    expect(firstImg?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=111",
    );

    // Simulate an error first so the img is unmounted, then a cacheKey bump
    // should cause it to re-mount (simulating a "retry" after the cover was
    // fixed on disk — equivalent to the old cacheBuster-driven retry flow).
    fireEvent.error(firstImg!);
    expect(container.querySelector("img")).toBeNull();

    rerender(<CoverGalleryTabs cacheKey={222} files={files} />);
    const secondImg = container.querySelector("img");
    expect(secondImg).not.toBeNull();
    expect(secondImg?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=222",
    );
  });

  it("mounts a fresh <img> element when cacheKey changes (even without error)", () => {
    const files = [
      makeFile({ id: 1, file_type: "epub", filepath: "/library/a.epub" }),
      makeFile({ id: 2, file_type: "m4b", filepath: "/library/b.m4b" }),
    ];

    const { container, rerender } = render(
      <CoverGalleryTabs cacheKey={111} files={files} />,
    );
    const firstImg = container.querySelector("img");
    expect(firstImg).not.toBeNull();
    expect(firstImg?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=111",
    );

    rerender(<CoverGalleryTabs cacheKey={222} files={files} />);
    const secondImg = container.querySelector("img");

    expect(secondImg).not.toBeNull();
    expect(secondImg).not.toBe(firstImg); // Different DOM node — React remounted
    expect(secondImg?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=222",
    );
  });
});
