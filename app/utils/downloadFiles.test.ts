import { describe, expect, it } from "vitest";

import type { Book, File } from "@/types";

import { collectDownloadFiles, getAvailableFileTypes } from "./downloadFiles";

const makeFile = (overrides: Partial<File>): File =>
  ({
    id: 1,
    file_type: "epub",
    file_role: "main",
    filesize_bytes: 1000,
    ...overrides,
  }) as File;

const makeBook = (id: number, files: Partial<File>[]): Book =>
  ({
    id,
    files: files.map((f, i) => makeFile({ id: id * 100 + i, ...f })),
  }) as Book;

describe("getAvailableFileTypes", () => {
  it("returns distinct file types from main files across selected books", () => {
    const books = [
      makeBook(1, [{ file_type: "epub" }, { file_type: "m4b" }]),
      makeBook(2, [{ file_type: "cbz" }]),
    ];
    const result = getAvailableFileTypes(books, [1, 2]);
    expect(result.sort()).toEqual(["cbz", "epub", "m4b"]);
  });

  it("excludes supplement files", () => {
    const books = [
      makeBook(1, [
        { file_type: "epub", file_role: "main" },
        { file_type: "pdf", file_role: "supplement" },
      ]),
    ];
    const result = getAvailableFileTypes(books, [1]);
    expect(result).toEqual(["epub"]);
  });

  it("only considers selected book IDs", () => {
    const books = [
      makeBook(1, [{ file_type: "epub" }]),
      makeBook(2, [{ file_type: "cbz" }]),
    ];
    const result = getAvailableFileTypes(books, [1]);
    expect(result).toEqual(["epub"]);
  });

  it("returns empty array when no books match", () => {
    const result = getAvailableFileTypes([], [1]);
    expect(result).toEqual([]);
  });
});

describe("collectDownloadFiles", () => {
  it("returns all main files matching selected types", () => {
    const books = [
      makeBook(1, [
        { file_type: "epub", filesize_bytes: 500 },
        { file_type: "m4b", filesize_bytes: 2000 },
      ]),
      makeBook(2, [{ file_type: "epub", filesize_bytes: 300 }]),
    ];
    const result = collectDownloadFiles(books, [1, 2], ["epub"]);
    expect(result.fileIds).toEqual([100, 200]);
    expect(result.totalSize).toBe(800);
  });

  it("includes multiple files of the same type from one book", () => {
    const books = [
      makeBook(1, [
        { file_type: "epub", filesize_bytes: 500 },
        { file_type: "epub", filesize_bytes: 700 },
      ]),
    ];
    const result = collectDownloadFiles(books, [1], ["epub"]);
    expect(result.fileIds).toEqual([100, 101]);
    expect(result.totalSize).toBe(1200);
  });

  it("excludes supplement files even if type matches", () => {
    const books = [
      makeBook(1, [
        { file_type: "pdf", file_role: "main", filesize_bytes: 100 },
        { file_type: "pdf", file_role: "supplement", filesize_bytes: 200 },
      ]),
    ];
    const result = collectDownloadFiles(books, [1], ["pdf"]);
    expect(result.fileIds).toEqual([100]);
    expect(result.totalSize).toBe(100);
  });

  it("returns empty results when no types are selected", () => {
    const books = [makeBook(1, [{ file_type: "epub", filesize_bytes: 500 }])];
    const result = collectDownloadFiles(books, [1], []);
    expect(result.fileIds).toEqual([]);
    expect(result.totalSize).toBe(0);
  });

  it("handles multiple selected types", () => {
    const books = [
      makeBook(1, [
        { file_type: "epub", filesize_bytes: 500 },
        { file_type: "m4b", filesize_bytes: 2000 },
        { file_type: "cbz", filesize_bytes: 300 },
      ]),
    ];
    const result = collectDownloadFiles(books, [1], ["epub", "cbz"]);
    expect(result.fileIds).toEqual([100, 102]);
    expect(result.totalSize).toBe(800);
  });

  it("only considers selected book IDs", () => {
    const books = [
      makeBook(1, [{ file_type: "epub", filesize_bytes: 500 }]),
      makeBook(2, [{ file_type: "epub", filesize_bytes: 300 }]),
    ];
    const result = collectDownloadFiles(books, [1], ["epub"]);
    expect(result.fileIds).toEqual([100]);
    expect(result.totalSize).toBe(500);
  });

  it("handles null filesize_bytes gracefully", () => {
    const books = [
      makeBook(1, [
        { file_type: "epub", filesize_bytes: undefined as unknown as number },
      ]),
    ];
    const result = collectDownloadFiles(books, [1], ["epub"]);
    expect(result.fileIds).toEqual([100]);
    expect(result.totalSize).toBe(0);
  });
});
