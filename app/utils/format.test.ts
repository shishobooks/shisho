import { describe, expect, it } from "vitest";

import {
  fileLabel,
  fileTitle,
  fileTypeLabel,
  formatBitrate,
  formatCount,
  formatDate,
  formatDateTime,
  formatDayTime,
  formatDuration,
  formatElapsed,
  formatFileSize,
  formatLongDay,
  formatMonthName,
  formatPageCount,
  formatPersonCredits,
  formatPlayerTime,
  formatShortDay,
  formatShowingRange,
  formatYear,
} from "./format";

describe("formatBitrate", () => {
  it("rounds to whole kilobits per second", () => {
    expect(formatBitrate(64000)).toBe("64 kbps");
    expect(formatBitrate(127_600)).toBe("128 kbps");
  });
});

describe("formatYear", () => {
  it("reads the year in UTC, so January 1 stays in its year west of UTC", () => {
    // Tests run in America/Chicago, where this instant is still 2014.
    expect(formatYear("2015-01-01T00:00:00Z")).toBe("2015");
  });
});

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
  it("groups thousands", () => {
    expect(formatPageCount(1200)).toBe("1,200 pages");
  });

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
    expect(fileLabel({ display_name: "", file_type: "kepub" })).toBe("KePub");
  });
});

describe("fileTitle", () => {
  it("uses the file's name when set", () => {
    expect(
      fileTitle(
        { name: "Harry Potter and the Sorcerer's Stone" },
        "Harry Potter and the Philosopher's Stone",
      ),
    ).toBe("Harry Potter and the Sorcerer's Stone");
  });

  it.each([
    ["missing", undefined],
    ["empty", ""],
    ["whitespace-only", "  "],
  ])("falls back to the book's title when the name is %s", (_, name) => {
    expect(fileTitle({ name }, "Book Title")).toBe("Book Title");
  });

  it("is empty when there is neither a name nor a book title", () => {
    expect(fileTitle({}, undefined)).toBe("");
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

describe("formatShowingRange", () => {
  it("groups thousands in the range and the total", () => {
    expect(formatShowingRange(18300, 15, 18307, "book", "books")).toBe(
      "Showing 18,301-18,307 of 18,307 books",
    );
  });

  it("says nothing for a page past the end", () => {
    expect(formatShowingRange(400, 50, 120, "genre", "genres")).toBeNull();
  });

  it("caps the end of the range at the total", () => {
    expect(formatShowingRange(0, 20, 2, "genre", "genres")).toBe(
      "Showing 1-2 of 2 genres",
    );
  });

  it("names a single item in the singular", () => {
    expect(formatShowingRange(0, 50, 1, "book", "books")).toBe(
      "Showing 1-1 of 1 book",
    );
    expect(formatShowingRange(0, 50, 1, "person", "people")).toBe(
      "Showing 1-1 of 1 person",
    );
  });
});

describe("formatCount", () => {
  it("groups thousands and pluralizes with an s", () => {
    expect(formatCount(18307, "book")).toBe("18,307 books");
  });

  it("keeps the singular for one", () => {
    expect(formatCount(1, "book")).toBe("1 book");
  });

  it("takes an irregular plural", () => {
    expect(formatCount(2, "person", "people")).toBe("2 people");
  });
});

describe("formatPersonCredits", () => {
  it("names both credits", () => {
    expect(formatPersonCredits(3, 1200)).toBe(
      "3 books authored and 1,200 files narrated",
    );
  });

  it("names only the narrated files for a narrator", () => {
    expect(formatPersonCredits(0, 40)).toBe("40 files narrated");
  });

  it("names only the authored books for an author", () => {
    expect(formatPersonCredits(1, 0)).toBe("1 book authored");
  });

  it("says there is nothing for a person with no credits", () => {
    expect(formatPersonCredits(0, 0)).toBe("0 books authored");
  });
});

describe("fileTypeLabel", () => {
  it("writes KePub in Kobo's casing", () => {
    expect(fileTypeLabel("kepub")).toBe("KePub");
  });

  it("writes every other File Type in capitals", () => {
    expect(fileTypeLabel("epub")).toBe("EPUB");
    expect(fileTypeLabel("cbz")).toBe("CBZ");
  });
});

describe("formatDuration", () => {
  it("shows hours and minutes, and marks under a minute rather than none", () => {
    expect(formatDuration(3661)).toBe("1h 1m");
    expect(formatDuration(600)).toBe("10m");
    expect(formatDuration(30)).toBe("<1m");
    expect(formatDuration(0)).toBe("0m");
  });
});

describe("formatMonthName", () => {
  it("names a month short, or in full with its year", () => {
    expect(formatMonthName(2026, 3)).toBe("Mar");
    expect(formatMonthName(2026, 12, { long: true })).toBe("December");
    expect(formatMonthName(2026, 3, { long: true, withYear: true })).toBe(
      "March 2026",
    );
  });
});

describe("calendar days", () => {
  it("names a day in full or briefly, whatever the browser's zone", () => {
    expect(formatLongDay("2026-04-07")).toBe("Tuesday, April 7, 2026");
    expect(formatShortDay("2026-04-07")).toBe("Apr 7");
  });

  it("gives a time of day in a zone, with the date when it's another day", () => {
    const instant = "2026-06-01T04:30:00Z"; // 11:30 PM, May 31, in Chicago
    expect(formatDayTime(instant, "2026-05-31", "America/Chicago")).toBe(
      "11:30 PM",
    );
    expect(formatDayTime(instant, "2026-06-01", "America/Chicago")).toBe(
      "May 31, 11:30 PM",
    );
    expect(formatDayTime(instant, "2026-06-01", "UTC")).toBe("4:30 AM");
  });
});
