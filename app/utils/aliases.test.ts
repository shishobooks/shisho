import { describe, expect, it } from "vitest";

import { resolveAliases } from "./aliases";

describe("resolveAliases", () => {
  it("returns committed aliases unchanged when pending input is empty", () => {
    expect(resolveAliases(["Sci-Fi", "SF"], "")).toEqual(["Sci-Fi", "SF"]);
  });

  it("returns committed aliases unchanged when pending input is whitespace-only", () => {
    expect(resolveAliases(["Sci-Fi"], "   ")).toEqual(["Sci-Fi"]);
  });

  it("appends trimmed pending input to committed aliases", () => {
    expect(resolveAliases(["Sci-Fi"], "  SF  ")).toEqual(["Sci-Fi", "SF"]);
  });

  it("appends pending input when committed list is empty", () => {
    expect(resolveAliases([], "New Alias")).toEqual(["New Alias"]);
  });

  it("does not add pending input that duplicates an existing alias (case-insensitive)", () => {
    expect(resolveAliases(["Sci-Fi"], "sci-fi")).toEqual(["Sci-Fi"]);
  });

  it("does not add pending input that duplicates an existing alias (different case)", () => {
    expect(resolveAliases(["SF"], "sf")).toEqual(["SF"]);
  });

  it("handles empty committed list with whitespace-only pending input", () => {
    expect(resolveAliases([], "   ")).toEqual([]);
  });
});
