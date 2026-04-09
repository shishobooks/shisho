import { getLanguageName, LANGUAGES } from "./languages";
import { describe, expect, it } from "vitest";

describe("getLanguageName", () => {
  it("returns exact match for a curated tag", () => {
    expect(getLanguageName("en")).toBe("English");
    expect(getLanguageName("fr")).toBe("French");
  });

  it("returns curated name for regional variants in the list", () => {
    expect(getLanguageName("en-US")).toBe("English - United States");
    expect(getLanguageName("pt-BR")).toBe("Portuguese - Brazil");
  });

  it("returns curated name for script variants in the list", () => {
    expect(getLanguageName("zh-Hans")).toBe("Chinese - Simplified");
  });

  it("falls back to base language when regional variant not in list", () => {
    // en-AU isn't in the curated list; should fall back to "English"
    expect(getLanguageName("en-AU")).toBe("English");
    expect(getLanguageName("fr-LU")).toBe("French");
  });

  it("returns undefined for completely unknown tags", () => {
    expect(getLanguageName("xx")).toBeUndefined();
    expect(getLanguageName("xx-YY")).toBeUndefined();
  });
});

describe("LANGUAGES", () => {
  it("is sorted alphabetically by name", () => {
    const names = LANGUAGES.map((l) => l.name);
    const sorted = [...names].sort((a, b) => a.localeCompare(b));
    expect(names).toEqual(sorted);
  });

  it("contains common base ISO 639-1 tags", () => {
    const tags = new Set(LANGUAGES.map((l) => l.tag));
    expect(tags.has("en")).toBe(true);
    expect(tags.has("fr")).toBe(true);
    expect(tags.has("de")).toBe(true);
    expect(tags.has("ja")).toBe(true);
    expect(tags.has("zh")).toBe(true);
  });

  it("contains key regional and script variants", () => {
    const tags = new Set(LANGUAGES.map((l) => l.tag));
    expect(tags.has("zh-Hans")).toBe(true);
    expect(tags.has("zh-Hant")).toBe(true);
    expect(tags.has("pt-BR")).toBe(true);
    expect(tags.has("en-US")).toBe(true);
  });

  it("uses the correct BCP 47 tag for Valencian", () => {
    // 'va' is a malformed ISO 639-1 code; the correct BCP 47 variant
    // for Valencian is 'ca-valencia'.
    const tags = new Set(LANGUAGES.map((l) => l.tag));
    expect(tags.has("va")).toBe(false);
    expect(tags.has("ca-valencia")).toBe(true);
  });
});
