import { describe, expect, it } from "vitest";

import {
  applyTitlesAndTimestamps,
  applyTitlesOnly,
  detectIntroOffset,
} from "./audnexusChapterUtils";

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

describe("applyTitlesOnly", () => {
  it("replaces titles by position, keeps timestamps", () => {
    const existing = [
      { title: "Old 1", start_timestamp_ms: 0, children: [] },
      { title: "Old 2", start_timestamp_ms: 60_000, children: [] },
    ];
    const fromAudible = [
      { title: "New 1", start_offset_ms: 100, length_ms: 1 },
      { title: "New 2", start_offset_ms: 200, length_ms: 1 },
    ];
    const result = applyTitlesOnly(existing, fromAudible);
    expect(result).toEqual([
      { title: "New 1", start_timestamp_ms: 0, children: [] },
      { title: "New 2", start_timestamp_ms: 60_000, children: [] },
    ]);
  });

  it("returns existing unchanged when counts differ", () => {
    const existing = [{ title: "A", start_timestamp_ms: 0, children: [] }];
    const fromAudible = [
      { title: "X", start_offset_ms: 0, length_ms: 1 },
      { title: "Y", start_offset_ms: 100, length_ms: 1 },
    ];
    const result = applyTitlesOnly(existing, fromAudible);
    expect(result).toEqual(existing);
  });
});

describe("applyTitlesAndTimestamps", () => {
  it("replaces wholesale with no offset when applyIntroOffset=false", () => {
    const fromAudible = [
      { title: "C1", start_offset_ms: 0, length_ms: 1000 },
      { title: "C2", start_offset_ms: 60_000, length_ms: 1000 },
    ];
    const result = applyTitlesAndTimestamps(fromAudible, {
      applyIntroOffset: false,
      introMs: 38_000,
    });
    expect(result).toEqual([
      { title: "C1", start_timestamp_ms: 0, children: [] },
      { title: "C2", start_timestamp_ms: 60_000, children: [] },
    ]);
  });

  it("subtracts introMs from every start when applyIntroOffset=true", () => {
    const fromAudible = [
      { title: "C1", start_offset_ms: 38_000, length_ms: 1000 },
      { title: "C2", start_offset_ms: 98_000, length_ms: 1000 },
    ];
    const result = applyTitlesAndTimestamps(fromAudible, {
      applyIntroOffset: true,
      introMs: 38_000,
    });
    expect(result).toEqual([
      { title: "C1", start_timestamp_ms: 0, children: [] },
      { title: "C2", start_timestamp_ms: 60_000, children: [] },
    ]);
  });

  it("clamps negative timestamps to 0 after offset", () => {
    const fromAudible = [
      { title: "Pre", start_offset_ms: 0, length_ms: 1 },
      { title: "Main", start_offset_ms: 40_000, length_ms: 1 },
    ];
    const result = applyTitlesAndTimestamps(fromAudible, {
      applyIntroOffset: true,
      introMs: 38_000,
    });
    expect(result[0].start_timestamp_ms).toBe(0);
    expect(result[1].start_timestamp_ms).toBe(2_000);
  });
});
