import { describe, expect, it } from "vitest";

import {
  galleryColumns,
  galleryPageSize,
  parseGallerySize,
} from "@/libraries/gallerySize";

describe("parseGallerySize", () => {
  it("accepts the four valid sizes", () => {
    expect(parseGallerySize("s")).toBe("s");
    expect(parseGallerySize("m")).toBe("m");
    expect(parseGallerySize("l")).toBe("l");
    expect(parseGallerySize("xl")).toBe("xl");
  });

  it("rejects nullish input", () => {
    expect(parseGallerySize(null)).toBeNull();
    expect(parseGallerySize("")).toBeNull();
  });

  it("is case-sensitive (matches sort behavior)", () => {
    expect(parseGallerySize("S")).toBeNull();
    expect(parseGallerySize("Medium")).toBeNull();
  });

  it("rejects unknown values", () => {
    expect(parseGallerySize("xxl")).toBeNull();
    expect(parseGallerySize("huge")).toBeNull();
  });
});

describe("galleryColumns", () => {
  it("fits as many covers of the size as the width holds", () => {
    // The content width of the old 1280px column, which held 11/8/6/5.
    expect(galleryColumns("s", 1232)).toBe(11);
    expect(galleryColumns("m", 1232)).toBe(8);
    expect(galleryColumns("l", 1232)).toBe(6);
    expect(galleryColumns("xl", 1232)).toBe(5);
    expect(galleryColumns("m", 1660)).toBe(11);
  });

  it("shows at least one column", () => {
    expect(galleryColumns("xl", 100)).toBe(1);
  });

  it("shows two columns on a phone, whatever the size", () => {
    expect(galleryColumns("s", 390, { phone: true })).toBe(2);
    expect(galleryColumns("xl", 390, { phone: true })).toBe(2);
  });
});

describe("galleryPageSize", () => {
  it("fills three rows", () => {
    expect(galleryPageSize(8)).toBe(24);
    expect(galleryPageSize(11)).toBe(33);
  });

  it("drops rows rather than exceed the API limit, keeping them full", () => {
    expect(galleryPageSize(40)).toBe(80);
    expect(galleryPageSize(150)).toBe(100);
  });

  it("shows twelve rows of two on a phone", () => {
    expect(galleryPageSize(2, { phone: true })).toBe(24);
  });
});
