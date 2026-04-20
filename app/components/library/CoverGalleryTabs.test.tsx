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
  it("re-renders cover image after error when cacheBuster changes", () => {
    const files = [
      makeFile({ id: 1, file_type: "epub", filepath: "/library/a.epub" }),
      makeFile({ id: 2, file_type: "m4b", filepath: "/library/b.m4b" }),
    ];

    const { container, rerender } = render(
      <CoverGalleryTabs cacheBuster={111} files={files} />,
    );

    // Initial render: the first file's cover <img> is present with cacheBuster=111
    let img = container.querySelector("img");
    expect(img).not.toBeNull();
    expect(img?.getAttribute("src")).toContain(
      "/api/books/files/1/cover?t=111",
    );

    // Simulate the cover failing to load (e.g. 404 because the cover didn't
    // exist on disk at that time)
    fireEvent.error(img!);

    // Once errored, the <img> is unmounted and only the placeholder remains
    expect(container.querySelector("img")).toBeNull();

    // A rescan/refresh completes: cacheBuster changes, so the cover URL
    // changes. The <img> must be re-rendered to attempt the new URL.
    rerender(<CoverGalleryTabs cacheBuster={222} files={files} />);

    img = container.querySelector("img");
    expect(img).not.toBeNull();
    expect(img?.getAttribute("src")).toContain(
      "/api/books/files/1/cover?t=222",
    );
  });
});
