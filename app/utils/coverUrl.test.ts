import { describe, expect, it } from "vitest";

import {
  bookCoverUrl,
  fileCacheKey,
  fileCoverUploadUrl,
  fileCoverUrl,
  seriesCoverUrl,
  shareBookCoverUrl,
  shareFileCoverUrl,
} from "./coverUrl";

describe("fileCacheKey", () => {
  it("is the file's updated_at in epoch milliseconds", () => {
    expect(fileCacheKey({ updated_at: "2024-01-01T00:00:00Z" })).toBe(
      1704067200000,
    );
  });
});

describe("bookCoverUrl", () => {
  it("keys the book cover on cover_cache_key", () => {
    expect(bookCoverUrl({ id: 7, cover_cache_key: "12-1704067200" })).toBe(
      "/api/books/7/cover?v=12-1704067200",
    );
  });

  it("always carries a key, even an empty one", () => {
    expect(bookCoverUrl({ id: 7, cover_cache_key: "" })).toBe(
      "/api/books/7/cover?v=",
    );
  });
});

describe("seriesCoverUrl", () => {
  it("keys the series cover on cover_cache_key", () => {
    expect(seriesCoverUrl({ id: 3, cover_cache_key: "12-1704067200" })).toBe(
      "/api/series/3/cover?v=12-1704067200",
    );
  });
});

describe("fileCoverUrl", () => {
  it("keys the file cover on updated_at in epoch milliseconds", () => {
    expect(fileCoverUrl({ id: 42, updated_at: "2024-01-01T00:00:00Z" })).toBe(
      "/api/books/files/42/cover?v=1704067200000",
    );
  });

  it("changes when the file's updated_at changes", () => {
    expect(
      fileCoverUrl({ id: 42, updated_at: "2024-06-01T00:00:00Z" }),
    ).not.toBe(fileCoverUrl({ id: 42, updated_at: "2024-01-01T00:00:00Z" }));
  });
});

describe("fileCoverUploadUrl", () => {
  it("is the POST endpoint, without a cache key", () => {
    expect(fileCoverUploadUrl(42)).toBe("/api/books/files/42/cover");
  });
});

describe("Share Link covers", () => {
  it("scopes the book cover to the encoded token", () => {
    expect(
      shareBookCoverUrl("a/b", { id: 7, cover_cache_key: "12-1704067200" }),
    ).toBe("/api/share/a%2Fb/cover?v=12-1704067200");
  });

  it("scopes the file cover to the token with an epoch-ms key", () => {
    expect(
      shareFileCoverUrl("tok", { id: 42, updated_at: "2024-01-01T00:00:00Z" }),
    ).toBe("/api/share/tok/files/42/cover?v=1704067200000");
  });
});
