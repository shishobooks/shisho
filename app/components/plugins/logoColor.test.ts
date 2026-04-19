import { derivePluginInitials, getPluginFallbackColor } from "./logoColor";
import { describe, expect, it } from "vitest";

describe("derivePluginInitials", () => {
  it("uses first letter + letter after first hyphen when id has a hyphen", () => {
    expect(derivePluginInitials("google-books")).toBe("GB");
    expect(derivePluginInitials("shisho-local-tagger")).toBe("SL");
  });

  it("uses first two letters for single-word ids ≥ 2 chars", () => {
    expect(derivePluginInitials("calibre")).toBe("CA");
    expect(derivePluginInitials("audible")).toBe("AU");
  });

  it("uses the single letter uppercased for 1-char ids", () => {
    expect(derivePluginInitials("c")).toBe("C");
  });

  it("returns uppercase", () => {
    expect(derivePluginInitials("abc-def")).toBe("AD");
  });
});

describe("getPluginFallbackColor", () => {
  it("returns a deterministic palette entry for a given (scope, id)", () => {
    const first = getPluginFallbackColor("shisho", "google-books");
    const second = getPluginFallbackColor("shisho", "google-books");
    expect(first).toBe(second);
  });

  it("returns different colors for different (scope, id) pairs (most of the time)", () => {
    const a = getPluginFallbackColor("shisho", "a");
    const b = getPluginFallbackColor("shisho", "b");
    const c = getPluginFallbackColor("shisho", "c");
    const d = getPluginFallbackColor("shisho", "d");
    const unique = new Set([a, b, c, d]);
    expect(unique.size).toBeGreaterThan(1);
  });

  it("returns a value from the declared palette", () => {
    const color = getPluginFallbackColor("shisho", "google-books");
    expect(color).toMatch(/^#([0-9a-f]{6})$/i);
  });

  it("never returns undefined (INT32_MIN regression guard)", () => {
    // Stress test: random-ish ids shouldn't produce an invalid palette lookup.
    for (let i = 0; i < 1000; i++) {
      const color = getPluginFallbackColor("shisho", `stress-${i}-${i * 7}`);
      expect(color).toBeDefined();
      expect(color).toMatch(/^#([0-9a-f]{6})$/i);
    }
  });
});
