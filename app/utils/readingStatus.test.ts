import { afterEach, describe, expect, it, vi } from "vitest";

import type { ReadThrough } from "@/types";

import {
  readingStatusLabel,
  readingSummary,
  readingTransitions,
  todayDay,
} from "./readingStatus";

const rt = (overrides: Partial<ReadThrough>): ReadThrough => ({
  id: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  book_id: 1,
  begin_day: undefined,
  end_day: undefined,
  outcome: "finished",
  paused: false,
  ...overrides,
});

describe("todayDay", () => {
  afterEach(() => {
    vi.useFakeTimers();
  });

  it("is the browser's local day, not the UTC one", () => {
    // 11pm on Oct 28 in a zone behind UTC is already Oct 29 in UTC.
    vi.setSystemTime(new Date(2026, 9, 28, 23, 30));
    expect(todayDay()).toBe("2026-10-28");
  });

  it("pads the month and day", () => {
    vi.setSystemTime(new Date(2026, 0, 5, 12));
    expect(todayDay()).toBe("2026-01-05");
  });
});

describe("readingSummary", () => {
  it("is empty with no Read-throughs", () => {
    expect(readingSummary([])).toBeNull();
  });

  it("is empty with only one underway", () => {
    expect(
      readingSummary([rt({ outcome: "underway", begin_day: "2026-03-01" })]),
    ).toBeNull();
  });

  it("counts finishes and names the latest finish's month", () => {
    expect(
      readingSummary([
        rt({ id: 1, end_day: "2021-06-10" }),
        rt({ id: 2, end_day: "2024-03-02" }),
      ]),
    ).toBe("Read twice, last finished Mar 2024");
  });

  it("says once and counts beyond two", () => {
    expect(readingSummary([rt({ end_day: "2024-03-02" })])).toBe(
      "Read once, last finished Mar 2024",
    );
    expect(
      readingSummary([
        rt({ id: 1, end_day: "2020-01-01" }),
        rt({ id: 2, end_day: "2021-01-01" }),
        rt({ id: 3, end_day: undefined }),
      ]),
    ).toBe("Read 3 times, last finished Jan 2021");
  });

  it("leaves out the month when no finish is dated", () => {
    expect(readingSummary([rt({ end_day: undefined })])).toBe("Read once");
  });

  it("counts abandoned Read-throughs separately", () => {
    expect(
      readingSummary([
        rt({ id: 1, end_day: "2024-03-02" }),
        rt({ id: 2, outcome: "abandoned", end_day: "2025-01-01" }),
      ]),
    ).toBe("Read once, last finished Mar 2024 · Didn't finish once");
    expect(
      readingSummary([rt({ outcome: "abandoned", end_day: "2025-01-01" })]),
    ).toBe("Didn't finish once");
  });
});

describe("readingTransitions", () => {
  it("offers only the transitions valid from each status", () => {
    expect(readingTransitions(null)).toEqual(["start", "finish"]);
    expect(readingTransitions("to_read")).toEqual(["start", "finish"]);
    expect(readingTransitions("currently_reading")).toEqual([
      "pause",
      "finish",
      "abandon",
    ]);
    expect(readingTransitions("paused")).toEqual([
      "resume",
      "finish",
      "abandon",
    ]);
    expect(readingTransitions("read")).toEqual(["start", "finish"]);
    expect(readingTransitions("did_not_finish")).toEqual(["start", "finish"]);
  });
});

describe("readingStatusLabel", () => {
  it("names every status", () => {
    expect(readingStatusLabel("currently_reading")).toBe("Currently Reading");
    expect(readingStatusLabel("paused")).toBe("Paused");
    expect(readingStatusLabel("to_read")).toBe("To Read");
    expect(readingStatusLabel("read")).toBe("Read");
    expect(readingStatusLabel("did_not_finish")).toBe("Did Not Finish");
  });
});
