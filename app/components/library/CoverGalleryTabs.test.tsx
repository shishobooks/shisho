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
    is_preferred_cover: false,
    ...overrides,
  };
}

describe("CoverGalleryTabs", () => {
  it("renders cover image with cache key from file updated_at", () => {
    const files = [
      makeFile({ id: 1, file_type: "epub", filepath: "/library/a.epub" }),
      makeFile({ id: 2, file_type: "m4b", filepath: "/library/b.m4b" }),
    ];

    const { container } = render(<CoverGalleryTabs files={files} />);

    const img = container.querySelector("img");
    expect(img).not.toBeNull();
    expect(img?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=2024-01-01T00:00:00Z",
    );
  });

  it("re-mounts cover image when file updated_at changes", () => {
    const files = [
      makeFile({
        id: 1,
        file_type: "epub",
        filepath: "/library/a.epub",
        updated_at: "2024-01-01T00:00:00Z",
      }),
      makeFile({ id: 2, file_type: "m4b", filepath: "/library/b.m4b" }),
    ];

    const { container, rerender } = render(<CoverGalleryTabs files={files} />);
    const firstImg = container.querySelector("img");
    expect(firstImg).not.toBeNull();
    expect(firstImg?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=2024-01-01T00:00:00Z",
    );

    fireEvent.error(firstImg!);
    expect(container.querySelector("img")).toBeNull();

    const updatedFiles = [
      makeFile({
        id: 1,
        file_type: "epub",
        filepath: "/library/a.epub",
        updated_at: "2024-06-01T00:00:00Z",
      }),
      makeFile({ id: 2, file_type: "m4b", filepath: "/library/b.m4b" }),
    ];
    rerender(<CoverGalleryTabs files={updatedFiles} />);
    const secondImg = container.querySelector("img");
    expect(secondImg).not.toBeNull();
    expect(secondImg?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=2024-06-01T00:00:00Z",
    );
  });

  it("mounts a fresh <img> element when file updated_at changes (even without error)", () => {
    const files = [
      makeFile({
        id: 1,
        file_type: "epub",
        filepath: "/library/a.epub",
        updated_at: "2024-01-01T00:00:00Z",
      }),
      makeFile({ id: 2, file_type: "m4b", filepath: "/library/b.m4b" }),
    ];

    const { container, rerender } = render(<CoverGalleryTabs files={files} />);
    const firstImg = container.querySelector("img");
    expect(firstImg).not.toBeNull();
    expect(firstImg?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=2024-01-01T00:00:00Z",
    );

    const updatedFiles = [
      makeFile({
        id: 1,
        file_type: "epub",
        filepath: "/library/a.epub",
        updated_at: "2024-06-01T00:00:00Z",
      }),
      makeFile({ id: 2, file_type: "m4b", filepath: "/library/b.m4b" }),
    ];
    rerender(<CoverGalleryTabs files={updatedFiles} />);
    const secondImg = container.querySelector("img");

    expect(secondImg).not.toBeNull();
    expect(secondImg).not.toBe(firstImg);
    expect(secondImg?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=2024-06-01T00:00:00Z",
    );
  });
});
