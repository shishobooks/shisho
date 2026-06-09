// Pure, framework-free chapter and playback resolution for the audiobook
// player. Nothing here touches React, the DOM, or the audio element — it takes
// plain inputs (chapters, the current playback time, the file duration) and
// returns plain data (resolved chapter boundaries and seek targets). This keeps
// the logic unit-testable in complete isolation and decoupled from the player.
//
// UNIT CHOICE: the public interface of this module works in SECONDS, matching
// the audio element's `currentTime` and the file's `audiobook_duration_seconds`.
// Chapter starts arrive on the model as `start_timestamp_ms` (MILLISECONDS), so
// the conversion to seconds happens here, at the boundary, when chapters are
// resolved. Callers never deal with milliseconds.

import type { Chapter } from "@/types";

/** Seconds skipped by the skip-back / skip-forward controls (arrow keys). */
export const SKIP_SECONDS = 30;

/**
 * Threshold, in seconds, for the smart previous-chapter rule: when more than
 * this far into the current chapter, "previous" restarts the current chapter;
 * otherwise it jumps to the prior chapter.
 */
export const PREVIOUS_CHAPTER_RESTART_THRESHOLD_SECONDS = 5;

/** A chapter resolved into absolute second boundaries within the file. */
export interface ResolvedChapter {
  /** Zero-based index into the sorted, resolved chapter list. */
  index: number;
  title: string;
  /** Start of the chapter, in seconds from the beginning of the file. */
  startSeconds: number;
  /**
   * End of the chapter, in seconds. A chapter ends where the next begins; the
   * last chapter ends at the file duration.
   */
  endSeconds: number;
}

/** The result of resolving playback state against the resolved chapter list. */
export interface PlaybackState {
  /** Index of the current chapter, or null when there are no chapters. */
  currentChapterIndex: number | null;
  /** The current chapter, or null when there are no chapters. */
  currentChapter: ResolvedChapter | null;
  /** Seek target (seconds) to advance to the next chapter, or null if none. */
  nextChapterTarget: number | null;
  /**
   * Seek target (seconds) for the previous-chapter control, or null if there
   * are no chapters. Follows the smart restart rule (see threshold constant).
   */
  previousChapterTarget: number | null;
  /** Seek target (seconds) for skip-back, clamped to [0, duration]. */
  skipBackTarget: number;
  /** Seek target (seconds) for skip-forward, clamped to [0, duration]. */
  skipForwardTarget: number;
}

const clamp = (value: number, min: number, max: number): number =>
  Math.min(Math.max(value, min), max);

/**
 * Resolves a list of model chapters into absolute second boundaries.
 *
 * Defensive against real-world data: chapters may arrive unsorted or missing a
 * `start_timestamp_ms`. Chapters without a start are dropped (they have no
 * position on the timeline); the remainder are sorted by start. If nothing is
 * left, the file is treated as having no chapters and an empty list is returned.
 * Each chapter ends where the next begins, and the last ends at `durationSeconds`.
 *
 * @param chapters The file's chapters (M4B chapters are flat, top-level only).
 * @param durationSeconds Total file duration in seconds.
 */
export function resolveChapters(
  chapters: Chapter[] | undefined,
  durationSeconds: number,
): ResolvedChapter[] {
  if (!chapters || chapters.length === 0) return [];

  const withStarts = chapters
    .filter(
      (c): c is Chapter & { start_timestamp_ms: number } =>
        typeof c.start_timestamp_ms === "number",
    )
    .map((c) => ({
      title: c.title,
      startSeconds: c.start_timestamp_ms / 1000,
    }))
    .sort((a, b) => a.startSeconds - b.startSeconds);

  if (withStarts.length === 0) return [];

  return withStarts.map((c, index) => {
    const next = withStarts[index + 1];
    const endSeconds = next ? next.startSeconds : durationSeconds;
    return {
      index,
      title: c.title,
      startSeconds: c.startSeconds,
      endSeconds,
    };
  });
}

/**
 * Finds the current chapter: the last chapter whose start is at or before the
 * current time. An exact boundary belongs to the chapter that starts there
 * (i.e. the next chapter). Times before the first start clamp to the first
 * chapter; times past the end stay on the last chapter. Returns null when there
 * are no chapters.
 */
function findCurrentChapterIndex(
  chapters: ResolvedChapter[],
  currentTimeSeconds: number,
): number | null {
  if (chapters.length === 0) return null;
  let index = 0;
  for (let i = 0; i < chapters.length; i++) {
    if (chapters[i].startSeconds <= currentTimeSeconds) {
      index = i;
    } else {
      break;
    }
  }
  return index;
}

/**
 * Resolves all playback-derived state for the player from the resolved chapter
 * list, the current time, and the file duration. Pure: same inputs always
 * yield the same output.
 */
export function resolvePlayback(
  chapters: ResolvedChapter[],
  currentTimeSeconds: number,
  durationSeconds: number,
): PlaybackState {
  const skipBackTarget = clamp(
    currentTimeSeconds - SKIP_SECONDS,
    0,
    durationSeconds,
  );
  const skipForwardTarget = clamp(
    currentTimeSeconds + SKIP_SECONDS,
    0,
    durationSeconds,
  );

  const currentChapterIndex = findCurrentChapterIndex(
    chapters,
    currentTimeSeconds,
  );

  if (currentChapterIndex === null) {
    return {
      currentChapterIndex: null,
      currentChapter: null,
      nextChapterTarget: null,
      previousChapterTarget: null,
      skipBackTarget,
      skipForwardTarget,
    };
  }

  const currentChapter = chapters[currentChapterIndex];

  const nextChapter = chapters[currentChapterIndex + 1];
  const nextChapterTarget = nextChapter ? nextChapter.startSeconds : null;

  // Smart previous: if we are more than the threshold into the current chapter,
  // restart it; otherwise jump to the prior chapter's start. On the first
  // chapter there is no prior chapter, so it always restarts at 0.
  const secondsIntoChapter = currentTimeSeconds - currentChapter.startSeconds;
  const priorChapter = chapters[currentChapterIndex - 1];
  let previousChapterTarget: number;
  if (
    secondsIntoChapter > PREVIOUS_CHAPTER_RESTART_THRESHOLD_SECONDS ||
    !priorChapter
  ) {
    previousChapterTarget = currentChapter.startSeconds;
  } else {
    previousChapterTarget = priorChapter.startSeconds;
  }

  return {
    currentChapterIndex,
    currentChapter,
    nextChapterTarget,
    previousChapterTarget,
    skipBackTarget,
    skipForwardTarget,
  };
}
