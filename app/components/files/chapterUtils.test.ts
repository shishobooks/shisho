import {
  formatTimestampMs,
  getNextChapterTitle,
  parseTimestampMs,
} from "./chapterUtils";
import { describe, expect, it } from "vitest";

describe("getNextChapterTitle", () => {
  it('detects "Chapter N" pattern', () => {
    expect(getNextChapterTitle("Chapter 3")).toBe("Chapter 4");
  });

  it('detects "Ch. N" pattern', () => {
    expect(getNextChapterTitle("Ch. 3")).toBe("Ch. 4");
  });

  it('detects "Chapter N: Title" pattern and strips the title', () => {
    expect(getNextChapterTitle("Chapter 3: The Title")).toBe("Chapter 4");
  });

  it('detects "N: Title" pattern and strips the title', () => {
    expect(getNextChapterTitle("3: The Title")).toBe("4");
  });

  it("returns empty string for non-pattern titles", () => {
    expect(getNextChapterTitle("Introduction")).toBe("");
  });

  it("returns empty string for empty input", () => {
    expect(getNextChapterTitle("")).toBe("");
  });
});

describe("formatTimestampMs", () => {
  it("formats 0 as 00:00:00.000", () => {
    expect(formatTimestampMs(0)).toBe("00:00:00.000");
  });

  it("formats 3661001 as 01:01:01.001", () => {
    expect(formatTimestampMs(3661001)).toBe("01:01:01.001");
  });

  it("formats milliseconds correctly", () => {
    expect(formatTimestampMs(500)).toBe("00:00:00.500");
  });

  it("formats seconds correctly", () => {
    expect(formatTimestampMs(5000)).toBe("00:00:05.000");
  });

  it("formats minutes correctly", () => {
    expect(formatTimestampMs(300000)).toBe("00:05:00.000");
  });

  it("formats hours correctly", () => {
    expect(formatTimestampMs(3600000)).toBe("01:00:00.000");
  });

  it("formats large values correctly", () => {
    // 99 hours, 59 minutes, 59 seconds, 999 milliseconds
    expect(formatTimestampMs(359999999)).toBe("99:59:59.999");
  });

  it("pads single-digit values with leading zeros", () => {
    expect(formatTimestampMs(1001)).toBe("00:00:01.001");
  });
});

describe("parseTimestampMs", () => {
  it('parses "01:30:00.500" to 5400500', () => {
    expect(parseTimestampMs("01:30:00.500")).toBe(5400500);
  });

  it('returns null for "invalid"', () => {
    expect(parseTimestampMs("invalid")).toBe(null);
  });

  it('returns null for ""', () => {
    expect(parseTimestampMs("")).toBe(null);
  });

  it('parses "00:00:00.000" to 0', () => {
    expect(parseTimestampMs("00:00:00.000")).toBe(0);
  });

  it("parses full format without milliseconds", () => {
    expect(parseTimestampMs("01:30:00")).toBe(5400000);
  });

  it("parses short format MM:SS.mmm", () => {
    expect(parseTimestampMs("30:00.500")).toBe(1800500);
  });

  it("parses short format MM:SS without milliseconds", () => {
    expect(parseTimestampMs("30:00")).toBe(1800000);
  });

  it("handles single-digit values", () => {
    expect(parseTimestampMs("1:2:3.4")).toBe(3723400);
  });

  it("pads milliseconds correctly when only 1 digit provided", () => {
    expect(parseTimestampMs("00:00:00.1")).toBe(100);
  });

  it("pads milliseconds correctly when only 2 digits provided", () => {
    expect(parseTimestampMs("00:00:00.12")).toBe(120);
  });

  it("returns null for invalid minutes (>= 60)", () => {
    expect(parseTimestampMs("00:60:00.000")).toBe(null);
  });

  it("returns null for invalid seconds (>= 60)", () => {
    expect(parseTimestampMs("00:00:60.000")).toBe(null);
  });

  it("handles whitespace-only string", () => {
    expect(parseTimestampMs("   ")).toBe(null);
  });

  it("trims whitespace from input", () => {
    expect(parseTimestampMs("  01:00:00.000  ")).toBe(3600000);
  });

  it("returns null for non-string input", () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    expect(parseTimestampMs(null as any)).toBe(null);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    expect(parseTimestampMs(undefined as any)).toBe(null);
  });

  it("returns null for malformed timestamps", () => {
    expect(parseTimestampMs("1:2:3:4")).toBe(null);
    expect(parseTimestampMs("abc:de:fg")).toBe(null);
    expect(parseTimestampMs("00:00:00.")).toBe(null);
  });

  it("is inverse of formatTimestampMs", () => {
    const testValues = [0, 1000, 60000, 3600000, 5400500, 3661001];
    for (const ms of testValues) {
      const formatted = formatTimestampMs(ms);
      const parsed = parseTimestampMs(formatted);
      expect(parsed).toBe(ms);
    }
  });
});
