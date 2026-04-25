import { describe, expect, it } from "vitest";

import { pageForSizeChange, parseGallerySize } from "@/libraries/gallerySize";

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

describe("pageForSizeChange", () => {
  it("preserves the first-visible item across size changes", () => {
    expect(pageForSizeChange(96, 48)).toBe(3);
    expect(pageForSizeChange(96, 16)).toBe(7);
    expect(pageForSizeChange(96, 12)).toBe(9);
  });

  it("errs backward when boundaries don't align", () => {
    expect(pageForSizeChange(99, 12)).toBe(9);
  });

  it("returns page 1 at offset 0", () => {
    expect(pageForSizeChange(0, 48)).toBe(1);
    expect(pageForSizeChange(0, 12)).toBe(1);
  });

  it("returns page 1 when offset < new_limit", () => {
    expect(pageForSizeChange(11, 12)).toBe(1);
  });
});
