import { describe, expect, it } from "vitest";

import {
  ebookCoverRank,
  isEbookFileType,
  isMainEligibleFileType,
  isReflowableFileType,
} from "./fileTypes";

describe("isEbookFileType", () => {
  it("includes every ebook format", () => {
    for (const ft of ["epub", "kepub", "azw3", "mobi", "cbz", "pdf"]) {
      expect(isEbookFileType(ft)).toBe(true);
    }
  });

  it("excludes audiobooks and other files", () => {
    for (const ft of ["m4b", "txt", "azw", ""]) {
      expect(isEbookFileType(ft)).toBe(false);
    }
  });
});

describe("isReflowableFileType", () => {
  it("includes EPUB, KePub, AZW3, and MOBI and nothing else", () => {
    for (const ft of ["epub", "kepub", "azw3", "mobi"]) {
      expect(isReflowableFileType(ft)).toBe(true);
    }
    for (const ft of ["cbz", "pdf", "m4b", "azw", ""]) {
      expect(isReflowableFileType(ft)).toBe(false);
    }
  });
});

describe("isMainEligibleFileType", () => {
  it("includes every built-in type", () => {
    for (const ft of ["epub", "kepub", "azw3", "mobi", "cbz", "pdf", "m4b"]) {
      expect(isMainEligibleFileType(ft)).toBe(true);
    }
  });

  it("excludes other types", () => {
    for (const ft of ["txt", "azw", "prc", ""]) {
      expect(isMainEligibleFileType(ft)).toBe(false);
    }
  });
});

describe("ebookCoverRank", () => {
  it("orders EPUB, KePub, AZW3, MOBI, then the rest together", () => {
    expect(ebookCoverRank("epub")).toBeLessThan(ebookCoverRank("kepub"));
    expect(ebookCoverRank("kepub")).toBeLessThan(ebookCoverRank("azw3"));
    expect(ebookCoverRank("azw3")).toBeLessThan(ebookCoverRank("mobi"));
    expect(ebookCoverRank("mobi")).toBeLessThan(ebookCoverRank("cbz"));
    expect(ebookCoverRank("cbz")).toBe(ebookCoverRank("pdf"));
  });
});
