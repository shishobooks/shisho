import { describe, expect, it } from "vitest";

import type { Book } from "@/types";

import { hasAnyCBZFile } from "./hasAnyCBZFile";

function makeBook(overrides: Partial<Book> = {}): Book {
  return {
    id: 1,
    title: "Test",
    created_at: "2024-01-01T00:00:00Z",
    updated_at: "2024-01-01T00:00:00Z",
    library_id: 1,
    ...overrides,
  } as Book;
}

describe("hasAnyCBZFile", () => {
  it("returns true when at least one file is CBZ", () => {
    const book = makeBook({
      files: [
        { id: 1, file_type: "epub" } as never,
        { id: 2, file_type: "cbz" } as never,
      ],
    });
    expect(hasAnyCBZFile(book)).toBe(true);
  });

  it("returns false when no files are CBZ", () => {
    const book = makeBook({
      files: [
        { id: 1, file_type: "epub" } as never,
        { id: 2, file_type: "m4b" } as never,
      ],
    });
    expect(hasAnyCBZFile(book)).toBe(false);
  });

  it("returns false when book has no files", () => {
    const book = makeBook({ files: [] });
    expect(hasAnyCBZFile(book)).toBe(false);
  });

  it("returns false when files is undefined", () => {
    const book = makeBook({ files: undefined });
    expect(hasAnyCBZFile(book)).toBe(false);
  });

  it("returns true when all files are CBZ", () => {
    const book = makeBook({
      files: [
        { id: 1, file_type: "cbz" } as never,
        { id: 2, file_type: "cbz" } as never,
      ],
    });
    expect(hasAnyCBZFile(book)).toBe(true);
  });
});
