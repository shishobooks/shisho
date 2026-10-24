import { afterEach, describe, expect, it, vi } from "vitest";

import {
  CBZPageKey,
  FileTypeCBZ,
  FileTypePDF,
  ThumbnailRenderKey,
} from "@/types";

import { filePageUrl, pageThumbnailSize, type PageSourceFile } from "./pageUrl";

const cbz: PageSourceFile = {
  id: 42,
  updated_at: "2024-01-01T00:00:00Z",
  file_type: FileTypeCBZ,
};
const pdf: PageSourceFile = {
  id: 7,
  updated_at: "2024-01-01T00:00:00Z",
  file_type: FileTypePDF,
};

describe("filePageUrl", () => {
  it("builds the page URL with the file's updated_at as the cache key", () => {
    expect(filePageUrl(cbz, 3, "200-85")).toBe(
      `/api/books/files/42/page/3?v=1704067200000&r=${CBZPageKey}`,
    );
  });

  it("keys CBZ pages on the server's page key, not the PDF render key", () => {
    expect(filePageUrl(cbz, 3, "300-85")).toBe(filePageUrl(cbz, 3, "200-85"));
    expect(filePageUrl(cbz, 3, "200-85")).toContain(`&r=${CBZPageKey}`);
  });

  it("changes the URL when the file's updated_at changes", () => {
    const before = filePageUrl(cbz, 0, "200-85");
    const after = filePageUrl(
      { ...cbz, updated_at: "2024-06-01T00:00:00Z" },
      0,
      "200-85",
    );
    expect(after).not.toBe(before);
  });

  it("keys PDF pages on the server's render settings", () => {
    expect(filePageUrl(pdf, 3, "200-85")).toBe(
      "/api/books/files/7/page/3?v=1704067200000&r=200-85",
    );
    expect(filePageUrl(pdf, 3, "300-85")).not.toBe(
      filePageUrl(pdf, 3, "200-85"),
    );
  });
});

describe("page thumbnails", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("asks for a size and keys it on the page and the thumbnail renderer", () => {
    expect(filePageUrl(pdf, 3, "200-85", 256)).toBe(
      `/api/books/files/7/page/3?v=1704067200000&size=256&r=${encodeURIComponent(`200-85.${ThumbnailRenderKey}`)}`,
    );
  });

  it("picks the smallest tier that fills the box on the screen", () => {
    vi.stubGlobal("devicePixelRatio", 2);
    expect(pageThumbnailSize(96)).toBe(256);
    expect(pageThumbnailSize(300)).toBe(1024);
  });

  it("leaves a box past 1024 pixels to the whole page", () => {
    vi.stubGlobal("devicePixelRatio", 1);
    expect(pageThumbnailSize(1500)).toBeUndefined();
    expect(filePageUrl(pdf, 3, "200-85", 2048)).toBe(
      filePageUrl(pdf, 3, "200-85"),
    );
  });
});
