import { describe, expect, it } from "vitest";

import { safeRedirectPath } from "./redirect";

describe("safeRedirectPath", () => {
  it.each([
    "/lists",
    "/libraries/1/books?page=2",
    "/user/preferences#password",
  ])("keeps the in-app path %s", (path) => {
    expect(safeRedirectPath(path)).toBe(path);
  });

  it.each([
    null,
    "",
    "lists",
    "//evil.example",
    "/\\evil.example",
    "https://evil.example",
    "javascript:alert(1)",
  ])("rejects %s", (value) => {
    expect(safeRedirectPath(value)).toBeNull();
  });
});
