import { describe, expect, it } from "vitest";

import type { File } from "@/types";

import { getCoverFileType, selectCoverFile } from "./coverSelection";

const file = (overrides: Partial<File> & { id: number }): File =>
  ({
    book_id: 1,
    library_id: 1,
    filepath: "",
    file_type: "epub",
    file_role: "main",
    filesize_bytes: 0,
    ...overrides,
  }) as File;

const epubMain = file({
  id: 1,
  file_type: "epub",
  cover_image_filename: "epub.cover.jpg",
});
const cbzMain = file({
  id: 2,
  file_type: "cbz",
  cover_image_filename: "cbz.cover.jpg",
});
const pdfMain = file({
  id: 3,
  file_type: "pdf",
  cover_image_filename: "pdf.cover.jpg",
});
const m4bMain = file({
  id: 4,
  file_type: "m4b",
  cover_image_filename: "m4b.cover.jpg",
});
const epubMainNoCover = file({
  id: 5,
  file_type: "epub",
  cover_image_filename: undefined,
});
const m4bMainNoCover = file({
  id: 6,
  file_type: "m4b",
  cover_image_filename: undefined,
});
const pdfSupplement = file({
  id: 7,
  file_type: "pdf",
  file_role: "supplement",
  cover_image_filename: "supp.cover.jpg",
});
const epubSupplement = file({
  id: 8,
  file_type: "epub",
  file_role: "supplement",
  cover_image_filename: "supp.cover.jpg",
});

describe("selectCoverFile", () => {
  it("prefers book file in default mode", () => {
    expect(selectCoverFile([m4bMain, epubMain], "book")?.id).toBe(epubMain.id);
  });

  it("falls back to audiobook in default mode when no book file has a cover", () => {
    expect(selectCoverFile([epubMainNoCover, m4bMain], "book")?.id).toBe(
      m4bMain.id,
    );
  });

  it("prefers audiobook in audiobook mode", () => {
    expect(selectCoverFile([epubMain, m4bMain], "audiobook")?.id).toBe(
      m4bMain.id,
    );
  });

  it("falls back to book file in audiobook mode when no audiobook has a cover", () => {
    expect(selectCoverFile([epubMain, m4bMainNoCover], "audiobook")?.id).toBe(
      epubMain.id,
    );
  });

  it("treats CBZ as a book file", () => {
    expect(selectCoverFile([cbzMain], "book")?.id).toBe(cbzMain.id);
  });

  it("treats PDF as a book file", () => {
    expect(selectCoverFile([pdfMain], "book")?.id).toBe(pdfMain.id);
  });

  it("returns null for empty files", () => {
    expect(selectCoverFile([], "book")).toBeNull();
    expect(selectCoverFile(undefined, "book")).toBeNull();
  });

  it("skips files with no cover", () => {
    expect(selectCoverFile([epubMainNoCover, m4bMain], "book")?.id).toBe(
      m4bMain.id,
    );
  });

  it("skips PDF supplement when M4B main has cover", () => {
    expect(selectCoverFile([pdfSupplement, m4bMain], "book")?.id).toBe(
      m4bMain.id,
    );
  });

  it("skips EPUB supplement when M4B main has cover", () => {
    expect(selectCoverFile([epubSupplement, m4bMain], "book")?.id).toBe(
      m4bMain.id,
    );
  });

  it("returns null when only supplements have covers", () => {
    expect(selectCoverFile([pdfSupplement, m4bMainNoCover], "book")).toBeNull();
  });

  it("audiobook_fallback_book behaves like audiobook mode", () => {
    expect(
      selectCoverFile([epubMain, m4bMain], "audiobook_fallback_book")?.id,
    ).toBe(m4bMain.id);
  });

  it("unknown aspect ratio falls through to default", () => {
    expect(selectCoverFile([m4bMain, epubMain], "weird")?.id).toBe(epubMain.id);
  });
});

describe("getCoverFileType", () => {
  it("returns book by default for empty files", () => {
    expect(getCoverFileType(undefined, "book")).toBe("book");
    expect(getCoverFileType([], "book")).toBe("book");
  });

  it("returns book when an epub main exists (default mode)", () => {
    expect(getCoverFileType([epubMainNoCover], "book")).toBe("book");
  });

  it("returns audiobook when only an m4b main exists (default mode)", () => {
    expect(getCoverFileType([m4bMainNoCover], "book")).toBe("audiobook");
  });

  it("returns book when only a pdf main exists (default mode)", () => {
    expect(getCoverFileType([pdfMain], "book")).toBe("book");
  });

  it("returns audiobook when audiobook mode and m4b main exists", () => {
    expect(
      getCoverFileType([epubMainNoCover, m4bMainNoCover], "audiobook"),
    ).toBe("audiobook");
  });

  it("ignores PDF supplement when picking variant for M4B-only main book", () => {
    expect(getCoverFileType([m4bMainNoCover, pdfSupplement], "book")).toBe(
      "audiobook",
    );
  });

  it("ignores EPUB supplement when picking variant for M4B-only main book", () => {
    expect(getCoverFileType([m4bMainNoCover, epubSupplement], "book")).toBe(
      "audiobook",
    );
  });

  it("returns book when no main file exists at all", () => {
    expect(getCoverFileType([pdfSupplement], "book")).toBe("book");
  });
});
