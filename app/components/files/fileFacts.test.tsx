import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import FactsLine from "@/components/common/FactsLine";
import {
  FileRoleMain,
  FileTypeCBZ,
  FileTypeEPUB,
  FileTypePDF,
  ReadingDirectionLTR,
  ReadingDirectionRTL,
  type File,
} from "@/types";

import { fileDiskRows, fileEditionRows, fileHeaderFacts } from "./fileFacts";

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

describe("word count", () => {
  const epub = file({
    file_type: FileTypeEPUB,
    filepath: "/library/book.epub",
    word_count: 95312,
  });

  it("is a rounded fact under the name, exact for a screen reader", () => {
    render(<FactsLine facts={fileHeaderFacts(epub)} />);
    expect(screen.getByText("95K words")).toBeInTheDocument();
    expect(screen.getByText("95,312 words")).toHaveClass("sr-only");
  });

  it("has a row with the format's facts", () => {
    const row = fileDiskRows(epub).find((r) => r.label === "Words");
    render(<>{row?.value}</>);
    expect(screen.getByText("95K words")).toBeInTheDocument();
    expect(screen.getByText("95,312 words")).toHaveClass("sr-only");
  });

  it("is left out until the words are counted", () => {
    const uncounted = file({ file_type: FileTypeEPUB });
    render(<FactsLine facts={fileHeaderFacts(uncounted)} />);
    expect(screen.queryByText(/words?$/)).toBeNull();
    expect(
      fileDiskRows(uncounted).find((row) => row.label === "Words"),
    ).toBeUndefined();
  });
});

describe("FactsLine", () => {
  it("separates the facts with a dot", () => {
    const { container } = render(<FactsLine facts={["350 pages", "1 MB"]} />);
    expect(container).toHaveTextContent("350 pages · 1 MB");
  });
});
