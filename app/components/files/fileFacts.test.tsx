import { describe, expect, it } from "vitest";

import {
  FileRoleMain,
  FileTypeCBZ,
  FileTypeEPUB,
  FileTypePDF,
  ReadingDirectionLTR,
  ReadingDirectionRTL,
  type File,
} from "@/types";

import { fileEditionRows } from "./fileFacts";

const file = (overrides: Partial<File>): File => ({
  id: 1,
  book_id: 1,
  library_id: 1,
  filepath: "/library/book.cbz",
  display_name: "book.cbz",
  file_type: FileTypeCBZ,
  file_role: FileRoleMain,
  filesize_bytes: 1000,
  created_at: "2024-01-01",
  updated_at: "2024-01-01",
  is_preferred_cover: false,
  reachable_file_types: [],
  ...overrides,
});

const directionRow = (f: File) =>
  fileEditionRows(f).find((row) => row.label === "Reading Direction")?.value;

describe("fileEditionRows", () => {
  it("names a Page-based File that runs right to left", () => {
    expect(directionRow(file({ reading_direction: ReadingDirectionRTL }))).toBe(
      "Right to left",
    );
    expect(
      directionRow(
        file({
          file_type: FileTypePDF,
          reading_direction: ReadingDirectionRTL,
        }),
      ),
    ).toBe("Right to left");
  });

  it("leaves out left to right, the way nearly every book runs", () => {
    expect(directionRow(file({}))).toBeUndefined();
    expect(
      directionRow(file({ reading_direction: ReadingDirectionLTR })),
    ).toBeUndefined();
  });

  it("leaves it out for a File that isn't Page-based", () => {
    expect(
      directionRow(
        file({
          file_type: FileTypeEPUB,
          reading_direction: ReadingDirectionRTL,
        }),
      ),
    ).toBeUndefined();
  });
});
