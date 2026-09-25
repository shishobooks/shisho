import { describe, expect, it } from "vitest";

import { FileTypeCBZ, FileTypeEPUB, FileTypeM4B, FileTypePDF } from "@/types";

import { supportsKepub } from "./supportsKepub";

describe("supportsKepub", () => {
  it("is true for EPUB and CBZ files", () => {
    expect(supportsKepub(FileTypeEPUB)).toBe(true);
    expect(supportsKepub(FileTypeCBZ)).toBe(true);
  });

  it("is false for other file types", () => {
    expect(supportsKepub(FileTypeM4B)).toBe(false);
    expect(supportsKepub(FileTypePDF)).toBe(false);
  });
});
