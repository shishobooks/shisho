import { describe, expect, it } from "vitest";

import { formatAuthorCredits, getAuthorRoleLabel } from "./authorRoles";

describe("getAuthorRoleLabel", () => {
  it("maps known canonical roles to capitalized labels", () => {
    expect(getAuthorRoleLabel("writer")).toBe("Writer");
    expect(getAuthorRoleLabel("penciller")).toBe("Penciller");
    expect(getAuthorRoleLabel("inker")).toBe("Inker");
    expect(getAuthorRoleLabel("colorist")).toBe("Colorist");
    expect(getAuthorRoleLabel("letterer")).toBe("Letterer");
    expect(getAuthorRoleLabel("cover_artist")).toBe("Cover Artist");
    expect(getAuthorRoleLabel("editor")).toBe("Editor");
    expect(getAuthorRoleLabel("translator")).toBe("Translator");
  });

  it("falls back to the raw role string for unknown values", () => {
    expect(getAuthorRoleLabel("illustrator")).toBe("illustrator");
  });

  it("returns null for undefined, null, and empty string", () => {
    expect(getAuthorRoleLabel(undefined)).toBeNull();
    expect(getAuthorRoleLabel(null)).toBeNull();
    expect(getAuthorRoleLabel("")).toBeNull();
  });
});

describe("formatAuthorCredits", () => {
  it("credits each name once with every role it holds, in first-seen order", () => {
    expect(
      formatAuthorCredits([
        { name: "Fujita", role: "writer" },
        { name: "Someone", role: "inker" },
        { name: "Fujita", role: "penciller" },
        { name: "Fujita", role: "writer" },
      ]),
    ).toBe("Fujita (Writer, Penciller), Someone (Inker)");
  });

  it("leaves off the parentheses for a name with no role", () => {
    expect(
      formatAuthorCredits([
        { name: "Ann", role: undefined },
        { name: "Ann", role: "" },
        { name: "Bo", role: "editor" },
      ]),
    ).toBe("Ann, Bo (Editor)");
  });
});
