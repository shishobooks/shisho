import { describe, expect, it } from "vitest";

import {
  fileLabel,
  formatDate,
  formatDateTime,
  formatElapsed,
  formatFileSize,
  formatPageCount,
  formatPlayerTime,
} from "./format";

describe("formatDate", () => {
  it("preserves the UTC date regardless of local timezone", () => {
    // A date stored as midnight UTC - should display as Jan 6, not Jan 5
    // even when the local timezone is west of UTC
    const result = formatDate("2021-01-06T00:00:00Z");
    expect(result).toContain("Jan");
    expect(result).toContain("6");
    expect(result).toContain("2021");
  });

  it("formats a date with time component correctly", () => {
    const result = formatDate("2024-03-15T00:00:00Z");
    expect(result).toContain("Mar");
    expect(result).toContain("15");
  });
});

describe("formatDateTime", () => {
  it("includes a timezone indicator", () => {
    const result = formatDateTime("2024-01-15T18:30:00Z");
    // Should contain some timezone abbreviation (e.g., CST, EST, PST, UTC)
    expect(result).toMatch(/[A-Z]{2,5}/);
  });

  it("includes date and time components", () => {
    const result = formatDateTime("2024-01-15T18:30:00Z");
    expect(result).toContain("Jan");
    expect(result).toContain("15");
    expect(result).toContain("2024");
    // Should contain a time with :30 minutes regardless of timezone
    expect(result).toMatch(/:30/);
  });
});

describe("formatPlayerTime", () => {
  it("formats sub-minute durations as M:SS with zero minutes", () => {
    expect(formatPlayerTime(5)).toBe("0:05");
    expect(formatPlayerTime(45)).toBe("0:45");
  });

  it("formats minute-and-second durations as M:SS", () => {
    expect(formatPlayerTime(65)).toBe("1:05");
    expect(formatPlayerTime(185)).toBe("3:05");
  });

  it("formats hour-long durations as H:MM:SS", () => {
    expect(formatPlayerTime(3661)).toBe("1:01:01");
    expect(formatPlayerTime(3600)).toBe("1:00:00");
  });

  it("truncates fractional seconds rather than rounding up", () => {
    expect(formatPlayerTime(59.9)).toBe("0:59");
    expect(formatPlayerTime(0.4)).toBe("0:00");
  });

  it("clamps NaN, negative, and non-finite inputs to 0:00", () => {
    expect(formatPlayerTime(NaN)).toBe("0:00");
    expect(formatPlayerTime(-10)).toBe("0:00");
    expect(formatPlayerTime(Infinity)).toBe("0:00");
  });
});

describe("formatPageCount", () => {
  it("uses the singular for one page", () => {
    expect(formatPageCount(1)).toBe("1 page");
  });

  it("uses the plural otherwise", () => {
    expect(formatPageCount(0)).toBe("0 pages");
    expect(formatPageCount(2)).toBe("2 pages");
    expect(formatPageCount(350)).toBe("350 pages");
  });
});

describe("fileLabel", () => {
  it("uses the server's display name", () => {
    expect(fileLabel({ display_name: "notes.pdf", file_type: "pdf" })).toBe(
      "notes.pdf",
    );
  });

  it("falls back to the file type when there is no display name", () => {
    expect(fileLabel({ display_name: "", file_type: "epub" })).toBe("EPUB");
  });
});

describe("formatFileSize", () => {
  it("formats zero", () => {
    expect(formatFileSize(0)).toBe("0 B");
  });

  it("formats bytes through gigabytes", () => {
    expect(formatFileSize(512)).toBe("512 B");
    expect(formatFileSize(1536)).toBe("1.5 KB");
    expect(formatFileSize(5 * 1024 ** 2)).toBe("5 MB");
    expect(formatFileSize(3 * 1024 ** 3)).toBe("3 GB");
  });

  it("formats a terabyte-sized value instead of printing undefined", () => {
    expect(formatFileSize(2 * 1024 ** 4)).toBe("2 TB");
  });

  it("stays in terabytes past the largest unit", () => {
    expect(formatFileSize(2048 * 1024 ** 4)).toBe("2048 TB");
  });

  it("formats fractions of a byte as bytes", () => {
    expect(formatFileSize(0.5)).toBe("0.5 B");
  });
});

describe("formatElapsed", () => {
  it("formats sub-second spans in milliseconds", () => {
    expect(
      formatElapsed("2024-01-01T00:00:00.000Z", "2024-01-01T00:00:00.250Z"),
    ).toBe("250ms");
  });

  it("formats sub-minute spans in seconds", () => {
    expect(formatElapsed("2024-01-01T00:00:00Z", "2024-01-01T00:00:42Z")).toBe(
      "42s",
    );
  });

  it("formats longer spans in minutes and seconds", () => {
    expect(formatElapsed("2024-01-01T00:00:00Z", "2024-01-01T00:03:05Z")).toBe(
      "3m 5s",
    );
  });
});
