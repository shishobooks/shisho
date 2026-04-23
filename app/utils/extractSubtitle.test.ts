import { describe, expect, it } from "vitest";

import { extractSubtitleFromTitle } from "./extractSubtitle";

describe("extractSubtitleFromTitle", () => {
  it("returns null when title has no colon", () => {
    expect(extractSubtitleFromTitle("Why We Sleep")).toBeNull();
  });

  it("returns null for empty string", () => {
    expect(extractSubtitleFromTitle("")).toBeNull();
  });

  it("returns null when colon is leading", () => {
    expect(extractSubtitleFromTitle(": Subtitle")).toBeNull();
  });

  it("returns null when colon is trailing", () => {
    expect(extractSubtitleFromTitle("Title:")).toBeNull();
  });

  it("returns null for a bare colon", () => {
    expect(extractSubtitleFromTitle(":")).toBeNull();
  });

  it("returns null when only whitespace on one side", () => {
    expect(extractSubtitleFromTitle("   : Subtitle")).toBeNull();
    expect(extractSubtitleFromTitle("Title :   ")).toBeNull();
  });

  it("splits on the first colon and trims both sides", () => {
    expect(
      extractSubtitleFromTitle(
        "Why We Sleep: Unlocking the Power of Sleep and Dreams",
      ),
    ).toEqual({
      title: "Why We Sleep",
      subtitle: "Unlocking the Power of Sleep and Dreams",
    });
  });

  it("trims surrounding whitespace", () => {
    expect(extractSubtitleFromTitle("  Foo  :  Bar  ")).toEqual({
      title: "Foo",
      subtitle: "Bar",
    });
  });

  it("preserves additional colons in the subtitle (splits on first only)", () => {
    expect(extractSubtitleFromTitle("Star Wars: Thrawn: Alliances")).toEqual({
      title: "Star Wars",
      subtitle: "Thrawn: Alliances",
    });
  });
});
