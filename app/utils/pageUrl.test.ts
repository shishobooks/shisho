import { describe, expect, it } from "vitest";

import { filePageUrl } from "./pageUrl";

describe("filePageUrl", () => {
  it("builds the page URL with the file's updated_at as the cache key", () => {
    expect(filePageUrl({ id: 42, updated_at: "2024-01-01T00:00:00Z" }, 3)).toBe(
      "/api/books/files/42/page/3?v=1704067200000",
    );
  });

  it("changes the URL when the file's updated_at changes", () => {
    const before = filePageUrl(
      { id: 42, updated_at: "2024-01-01T00:00:00Z" },
      0,
    );
    const after = filePageUrl(
      { id: 42, updated_at: "2024-06-01T00:00:00Z" },
      0,
    );
    expect(after).not.toBe(before);
  });
});
