import { describe, expect, it } from "vitest";

import type { Chapter } from "@/types";

import {
  PREVIOUS_CHAPTER_RESTART_THRESHOLD_SECONDS,
  resolveChapters,
  resolvePlayback,
  SKIP_SECONDS,
} from "./chapters";

// Build a minimal Chapter with only the fields the resolver reads. The rest of
// the model (timestamps, file_id, etc.) is irrelevant to the pure module.
const chapter = (
  title: string,
  startMs: number | undefined,
  sortOrder: number,
): Chapter =>
  ({
    title,
    start_timestamp_ms: startMs,
    sort_order: sortOrder,
  }) as Chapter;

describe("resolveChapters", () => {
  it("returns no chapters for an empty list", () => {
    const result = resolveChapters([], 3600);
    expect(result).toEqual([]);
  });

  it("returns no chapters for an undefined list", () => {
    const result = resolveChapters(undefined, 3600);
    expect(result).toEqual([]);
  });

  it("derives a single chapter spanning the whole file", () => {
    const result = resolveChapters([chapter("Only", 0, 0)], 3600);
    expect(result).toEqual([
      { index: 0, title: "Only", startSeconds: 0, endSeconds: 3600 },
    ]);
  });

  it("converts millisecond starts to seconds and ends each chapter where the next begins", () => {
    const result = resolveChapters(
      [
        chapter("One", 0, 0),
        chapter("Two", 60000, 1),
        chapter("Three", 120000, 2),
      ],
      300,
    );
    expect(result).toEqual([
      { index: 0, title: "One", startSeconds: 0, endSeconds: 60 },
      { index: 1, title: "Two", startSeconds: 60, endSeconds: 120 },
      { index: 2, title: "Three", startSeconds: 120, endSeconds: 300 },
    ]);
  });

  it("sorts chapters by start time when they arrive out of order", () => {
    const result = resolveChapters(
      [chapter("Two", 60000, 1), chapter("One", 0, 0)],
      120,
    );
    expect(result.map((c) => c.title)).toEqual(["One", "Two"]);
    expect(result[0].startSeconds).toBe(0);
    expect(result[1].startSeconds).toBe(60);
  });

  it("treats chapters with missing start timestamps as no chapters", () => {
    const result = resolveChapters(
      [chapter("One", undefined, 0), chapter("Two", undefined, 1)],
      120,
    );
    expect(result).toEqual([]);
  });

  it("drops only the chapters missing a start timestamp, keeping the rest", () => {
    const result = resolveChapters(
      [
        chapter("One", 0, 0),
        chapter("Bad", undefined, 1),
        chapter("Two", 60000, 2),
      ],
      120,
    );
    expect(result.map((c) => c.title)).toEqual(["One", "Two"]);
    expect(result[1].endSeconds).toBe(120);
  });
});

describe("resolvePlayback", () => {
  const chapters = resolveChapters(
    [
      chapter("One", 0, 0),
      chapter("Two", 60000, 1),
      chapter("Three", 120000, 2),
    ],
    180,
  );

  it("reports no current chapter when there are no chapters", () => {
    const result = resolvePlayback([], 50, 180);
    expect(result.currentChapterIndex).toBeNull();
    expect(result.currentChapter).toBeNull();
  });

  it("clamps the current chapter to the first one when time is before the first start", () => {
    // start is 0 so this can only happen with a defensive negative time, but
    // the last-chapter-at-or-before rule still yields the first chapter.
    const result = resolvePlayback(chapters, -5, 180);
    expect(result.currentChapterIndex).toBe(0);
    expect(result.currentChapter?.title).toBe("One");
  });

  it("selects the last chapter whose start is at or before the current time", () => {
    const result = resolvePlayback(chapters, 90, 180);
    expect(result.currentChapterIndex).toBe(1);
    expect(result.currentChapter?.title).toBe("Two");
  });

  it("treats an exact boundary as the start of the next chapter", () => {
    const result = resolvePlayback(chapters, 120, 180);
    expect(result.currentChapterIndex).toBe(2);
    expect(result.currentChapter?.title).toBe("Three");
  });

  it("keeps the last chapter selected past the end of the file", () => {
    const result = resolvePlayback(chapters, 999, 180);
    expect(result.currentChapterIndex).toBe(2);
    expect(result.currentChapter?.title).toBe("Three");
  });

  describe("next chapter target", () => {
    it("jumps to the start of the following chapter", () => {
      const result = resolvePlayback(chapters, 30, 180);
      expect(result.nextChapterTarget).toBe(60);
    });

    it("is null on the last chapter (nothing to advance to)", () => {
      const result = resolvePlayback(chapters, 150, 180);
      expect(result.nextChapterTarget).toBeNull();
    });

    it("is null when there are no chapters", () => {
      const result = resolvePlayback([], 30, 180);
      expect(result.nextChapterTarget).toBeNull();
    });
  });

  describe("previous chapter target (smart restart rule)", () => {
    it("restarts the current chapter when more than the threshold into it", () => {
      // 60 + threshold + a hair => restart current chapter (start 60).
      const time = 60 + PREVIOUS_CHAPTER_RESTART_THRESHOLD_SECONDS + 0.5;
      const result = resolvePlayback(chapters, time, 180);
      expect(result.previousChapterTarget).toBe(60);
    });

    it("goes to the prior chapter when within the threshold of the current start", () => {
      // 60 + threshold - a hair => prior chapter (start 0).
      const time = 60 + PREVIOUS_CHAPTER_RESTART_THRESHOLD_SECONDS - 0.5;
      const result = resolvePlayback(chapters, time, 180);
      expect(result.previousChapterTarget).toBe(0);
    });

    it("restarts the first chapter (target 0) rather than going negative", () => {
      // Within the first chapter, before the threshold: there is no prior
      // chapter, so the target restarts at 0.
      const result = resolvePlayback(chapters, 2, 180);
      expect(result.previousChapterTarget).toBe(0);
    });

    it("restarts the first chapter when well into it", () => {
      const result = resolvePlayback(chapters, 40, 180);
      expect(result.previousChapterTarget).toBe(0);
    });

    it("is null when there are no chapters", () => {
      const result = resolvePlayback([], 30, 180);
      expect(result.previousChapterTarget).toBeNull();
    });
  });

  describe("skip targets", () => {
    it("skips forward by the skip interval", () => {
      const result = resolvePlayback(chapters, 30, 180);
      expect(result.skipForwardTarget).toBe(30 + SKIP_SECONDS);
    });

    it("skips backward by the skip interval", () => {
      const result = resolvePlayback(chapters, 100, 180);
      expect(result.skipBackTarget).toBe(100 - SKIP_SECONDS);
    });

    it("clamps skip back to 0 at the start of the file", () => {
      const result = resolvePlayback(chapters, 10, 180);
      expect(result.skipBackTarget).toBe(0);
    });

    it("clamps skip forward to the duration at the end of the file", () => {
      const result = resolvePlayback(chapters, 170, 180);
      expect(result.skipForwardTarget).toBe(180);
    });

    it("skip targets work with no chapters", () => {
      const result = resolvePlayback([], 10, 180);
      expect(result.skipBackTarget).toBe(0);
      expect(result.skipForwardTarget).toBe(10 + SKIP_SECONDS);
    });
  });
});
