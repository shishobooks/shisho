import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { File } from "@/types";

import FileCoverThumbnail from "./FileCoverThumbnail";

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

describe("FileCoverThumbnail", () => {
  it("renders cover image when file has a cover", () => {
    const { container } = render(<FileCoverThumbnail file={makeFile()} />);
    const img = container.querySelector("img");
    expect(img).not.toBeNull();
    expect(img?.getAttribute("src")).toBe("/api/books/files/1/cover");
  });

  it("hides the img and shows placeholder after load error", () => {
    const { container } = render(<FileCoverThumbnail file={makeFile()} />);
    const img = container.querySelector("img");
    expect(img).not.toBeNull();

    fireEvent.error(img!);
    expect(container.querySelector("img")).toBeNull();
  });

  it("re-mounts the img when cacheKey bumps after an error", () => {
    const file = makeFile();
    const { container, rerender } = render(
      <FileCoverThumbnail cacheKey={111} file={file} />,
    );
    const firstImg = container.querySelector("img");
    expect(firstImg).not.toBeNull();
    expect(firstImg?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=111",
    );

    fireEvent.error(firstImg!);
    expect(container.querySelector("img")).toBeNull();

    rerender(<FileCoverThumbnail cacheKey={222} file={file} />);
    const secondImg = container.querySelector("img");
    expect(secondImg).not.toBeNull();
    expect(secondImg?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=222",
    );
  });

  it("re-mounts the img when cover_image_filename changes after an error", () => {
    const { container, rerender } = render(
      <FileCoverThumbnail
        file={makeFile({ cover_image_filename: "old.jpg" })}
      />,
    );
    const firstImg = container.querySelector("img");
    expect(firstImg).not.toBeNull();

    fireEvent.error(firstImg!);
    expect(container.querySelector("img")).toBeNull();

    rerender(
      <FileCoverThumbnail
        file={makeFile({ cover_image_filename: "new.jpg" })}
      />,
    );
    expect(container.querySelector("img")).not.toBeNull();
  });
});
