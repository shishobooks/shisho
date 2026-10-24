import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { mockCoverDimensions } from "@/testing/coverDimensions";
import type { File } from "@/types";

import FileCoverThumbnail from "./FileCoverThumbnail";

// Page images carry the server's PDF render key from useAuth.
vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

mockCoverDimensions();

function makeFile(overrides: Partial<File> = {}): File {
  return {
    id: 1,
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    library_id: 1,
    book_id: 1,
    filepath: "/library/book.epub",
    display_name: "book.epub",
    file_type: "epub",
    file_role: "main",
    filesize_bytes: 1000,
    cover_image_filename: "cover.jpg",
    is_preferred_cover: false,
    reachable_file_types: [],
    ...overrides,
  };
}

describe("FileCoverThumbnail", () => {
  it("shows a PDF's first page when it has no cover", () => {
    const { container } = render(
      <FileCoverThumbnail
        file={makeFile({
          file_type: "pdf",
          cover_image_filename: undefined,
          page_count: 12,
        })}
      />,
    );
    expect(container.querySelector("img")?.getAttribute("src")).toMatch(
      /^\/api\/books\/files\/1\/page\/0\b/,
    );
  });

  it("shows a supplement with no picture as a tile of its type", () => {
    const { container, getByText } = render(
      <FileCoverThumbnail
        file={makeFile({
          file_role: "supplement",
          cover_image_filename: undefined,
        })}
      />,
    );
    expect(container.querySelector("img")).toBeNull();
    expect(getByText("EPUB")).toBeInTheDocument();
  });

  it("asks a supplied builder, with no page fallback, on a Share Link page", () => {
    const { container } = render(
      <FileCoverThumbnail
        file={makeFile({
          file_type: "pdf",
          file_role: "supplement",
          cover_image_filename: undefined,
          page_count: 12,
        })}
        getCoverUrl={() => null}
      />,
    );
    expect(container.querySelector("img")).toBeNull();
  });

  it("shows the pointer cursor only when clicking does something", () => {
    const { container, rerender } = render(
      <FileCoverThumbnail file={makeFile()} />,
    );
    expect(container.firstElementChild).not.toHaveClass("cursor-pointer");

    rerender(<FileCoverThumbnail file={makeFile()} onClick={() => {}} />);
    expect(container.firstElementChild).toHaveClass("cursor-pointer");
  });

  it("renders the cover keyed on updated_at in epoch milliseconds", () => {
    const { container } = render(<FileCoverThumbnail file={makeFile()} />);
    const img = container.querySelector("img");
    expect(img).not.toBeNull();
    expect(img?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=1704067200000&size=512&aspect=book&r=2",
    );
  });

  it("hides the img and shows placeholder after load error", () => {
    const { container } = render(<FileCoverThumbnail file={makeFile()} />);
    const img = container.querySelector("img");
    expect(img).not.toBeNull();

    fireEvent.error(img!);
    expect(container.querySelector("img")).toBeNull();
  });

  it("re-mounts the img when updated_at bumps after an error", () => {
    const { container, rerender } = render(
      <FileCoverThumbnail file={makeFile()} />,
    );
    const firstImg = container.querySelector("img");
    expect(firstImg).not.toBeNull();

    fireEvent.error(firstImg!);
    expect(container.querySelector("img")).toBeNull();

    rerender(
      <FileCoverThumbnail
        file={makeFile({ updated_at: "2024-06-01T00:00:00Z" })}
      />,
    );
    const secondImg = container.querySelector("img");
    expect(secondImg).not.toBeNull();
    expect(secondImg?.getAttribute("src")).toBe(
      "/api/books/files/1/cover?v=1717200000000&size=512&aspect=book&r=2",
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

  it("applies interactive styles by default", () => {
    const { container } = render(
      <FileCoverThumbnail file={makeFile()} onClick={() => {}} />,
    );
    const wrapper = container.firstElementChild as HTMLElement;
    expect(wrapper.className).toContain("cursor-pointer");
    expect(wrapper.className).toContain("hover:scale-105");
    expect(wrapper.className).toContain("hover:shadow-md");
  });

  it("applies interactive styles when interactive is explicitly true", () => {
    const { container } = render(
      <FileCoverThumbnail
        file={makeFile()}
        interactive={true}
        onClick={() => {}}
      />,
    );
    const wrapper = container.firstElementChild as HTMLElement;
    expect(wrapper.className).toContain("cursor-pointer");
    expect(wrapper.className).toContain("hover:scale-105");
    expect(wrapper.className).toContain("hover:shadow-md");
  });

  it("removes interactive styles when interactive is false", () => {
    const { container } = render(
      <FileCoverThumbnail file={makeFile()} interactive={false} />,
    );
    const wrapper = container.firstElementChild as HTMLElement;
    expect(wrapper.className).not.toContain("cursor-pointer");
    expect(wrapper.className).not.toContain("hover:scale-105");
    expect(wrapper.className).not.toContain("hover:shadow-md");
  });

  it("uses a supplied cover URL builder", () => {
    const { container } = render(
      <FileCoverThumbnail
        file={makeFile({ cover_image_filename: "" })}
        getCoverUrl={(file) => `/api/share/tok/files/${file.id}/cover`}
      />,
    );
    expect(container.querySelector("img")?.getAttribute("src")).toBe(
      "/api/share/tok/files/1/cover?size=512&aspect=book&r=2",
    );
  });

  it("shows the placeholder when the builder returns null", () => {
    const { container } = render(
      <FileCoverThumbnail file={makeFile()} getCoverUrl={() => null} />,
    );
    expect(container.querySelector("img")).toBeNull();
  });
});
