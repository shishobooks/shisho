import { describe, expect, it } from "vitest";

import { FileRoleMain, FileRoleSupplement } from "@/types";

import { isBookNeedsReview } from "./book";

describe("isBookNeedsReview", () => {
  it("returns false for a book with no files", () => {
    expect(isBookNeedsReview({ id: 1, title: "A" } as never)).toBe(false);
  });

  it("returns false for a book with no main files (only supplement)", () => {
    expect(
      isBookNeedsReview({
        id: 1,
        title: "A",
        files: [{ id: 10, file_role: FileRoleSupplement, reviewed: false }],
      } as never),
    ).toBe(false);
  });

  it("returns true when a main file has reviewed=false", () => {
    expect(
      isBookNeedsReview({
        id: 1,
        title: "A",
        files: [{ id: 10, file_role: FileRoleMain, reviewed: false }],
      } as never),
    ).toBe(true);
  });

  it("returns true when a main file has reviewed=undefined", () => {
    expect(
      isBookNeedsReview({
        id: 1,
        title: "A",
        files: [{ id: 10, file_role: FileRoleMain }],
      } as never),
    ).toBe(true);
  });

  it("returns false when all main files are reviewed", () => {
    expect(
      isBookNeedsReview({
        id: 1,
        title: "A",
        files: [
          { id: 10, file_role: FileRoleMain, reviewed: true },
          { id: 11, file_role: FileRoleMain, reviewed: true },
        ],
      } as never),
    ).toBe(false);
  });

  it("returns true when at least one main file is not reviewed (mixed)", () => {
    expect(
      isBookNeedsReview({
        id: 1,
        title: "A",
        files: [
          { id: 10, file_role: FileRoleMain, reviewed: true },
          { id: 11, file_role: FileRoleMain, reviewed: false },
        ],
      } as never),
    ).toBe(true);
  });
});
