import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { mockCoverDimensions } from "@/testing/coverDimensions";
import type { CoverAspectRatio, ThumbnailAspect } from "@/types";

import CoverFan from "./CoverFan";

const book = (
  id: number,
  cover_aspect: ThumbnailAspect = "book",
  cover_cache_key = `${id}-1`,
) => ({ id, title: `Book ${id}`, cover_cache_key, cover_aspect });

const images = (container: HTMLElement) => [
  ...container.querySelectorAll("img"),
];

const fan = (
  books: ReturnType<typeof book>[],
  libraryCoverAspectRatio: CoverAspectRatio = "book",
) =>
  render(
    <CoverFan
      books={books}
      libraryCoverAspectRatio={libraryCoverAspectRatio}
    />,
  ).container;

mockCoverDimensions(38, 56);

describe("CoverFan", () => {
  it("shows each book's cover, keyed for the cover cache", () => {
    const srcs = images(fan([book(1), book(2)])).map((img) =>
      img.getAttribute("src"),
    );

    expect(srcs).toHaveLength(2);
    expect(srcs[0]).toContain("/api/books/1/cover");
    expect(srcs[0]).toContain("v=1-1");
    expect(srcs[1]).toContain("/api/books/2/cover");
  });

  it("draws each cover in the shape the gallery gives it", () => {
    const [portrait, square] = images(
      fan([book(1, "book"), book(2, "square")], "audiobook_fallback_book"),
    );

    expect(portrait.className).toContain("w-[38px]");
    expect(square.className).toContain("w-14");
  });

  it("keeps one slot width per library, sized for its widest covers", () => {
    const width = (el: HTMLElement) =>
      (el.firstChild as HTMLElement).style.width;

    expect(width(fan([book(1)], "book"))).toBe("62px");
    expect(width(fan([book(1), book(2), book(3)], "book"))).toBe("62px");
    // A library that can show square covers leaves room for three of them.
    expect(width(fan([book(1)], "book_fallback_audiobook"))).toBe("80px");
    expect(width(fan([book(1, "square")], "audiobook"))).toBe("80px");
  });

  it("shows a placeholder in the library's cover shape when there are no books", () => {
    const portrait = fan([], "book");
    const square = fan([], "audiobook");
    const placeholder = (el: HTMLElement) =>
      el.querySelector('[data-testid="cover-fan-placeholder"]')!;

    expect(images(portrait)).toHaveLength(0);
    expect(placeholder(portrait).className).toContain("w-[38px]");
    expect(placeholder(square).className).toContain("w-14");
  });

  it("shows a placeholder tile for a book without a cover", () => {
    const container = fan([book(1, "book", ""), book(2)]);

    expect(images(container)).toHaveLength(1);
    expect(
      container.querySelectorAll('[data-testid="cover-fan-placeholder"]'),
    ).toHaveLength(1);
  });
});
