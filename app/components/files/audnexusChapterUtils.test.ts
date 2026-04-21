import { describe, expect, it } from "vitest";

import { detectIntroOffset } from "./audnexusChapterUtils";

describe("detectIntroOffset", () => {
  it("defaults to false when file matches full runtime", () => {
    const result = detectIntroOffset({
      runtimeMs: 163_938_000,
      introMs: 38_000,
      outroMs: 62_000,
      fileDurationMs: 163_938_000,
    });
    expect(result).toEqual({ applyOffset: false, withinTolerance: true });
  });

  it("returns true when file duration matches runtime minus intro", () => {
    const result = detectIntroOffset({
      runtimeMs: 163_938_000,
      introMs: 38_000,
      outroMs: 62_000,
      fileDurationMs: 163_900_000, // runtime - intro
    });
    expect(result).toEqual({ applyOffset: true, withinTolerance: true });
  });

  it("returns true when file duration matches runtime minus intro minus outro (Libation)", () => {
    const result = detectIntroOffset({
      runtimeMs: 163_938_000,
      introMs: 38_000,
      outroMs: 62_000,
      fileDurationMs: 163_838_000, // runtime - intro - outro
    });
    expect(result).toEqual({ applyOffset: true, withinTolerance: true });
  });

  it("returns false when file duration matches runtime minus outro only (no intro offset needed)", () => {
    const result = detectIntroOffset({
      runtimeMs: 163_938_000,
      introMs: 38_000,
      outroMs: 62_000,
      fileDurationMs: 163_876_000, // runtime - outro
    });
    expect(result).toEqual({ applyOffset: false, withinTolerance: true });
  });

  it("accepts ±2000ms tolerance on matches", () => {
    const result = detectIntroOffset({
      runtimeMs: 163_938_000,
      introMs: 38_000,
      outroMs: 62_000,
      fileDurationMs: 163_939_500, // +1.5s off full runtime
    });
    expect(result).toEqual({ applyOffset: false, withinTolerance: true });
  });

  it("falls back to false and withinTolerance=false when nothing matches", () => {
    const result = detectIntroOffset({
      runtimeMs: 163_938_000,
      introMs: 38_000,
      outroMs: 62_000,
      fileDurationMs: 160_000_000, // way off
    });
    expect(result).toEqual({ applyOffset: false, withinTolerance: false });
  });
});
