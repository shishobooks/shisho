import type { Chapter, ChapterInput } from "@/types";
import { formatTimestamp } from "@/utils/format";

/**
 * Formats milliseconds as HH:MM:SS.mmm timestamp.
 * Re-exports formatTimestamp from utils/format.ts for consistency.
 * @example formatTimestampMs(3661500) // "01:01:01.500"
 */
export const formatTimestampMs = formatTimestamp;

/**
 * Parses a timestamp string in HH:MM:SS.mmm format to milliseconds.
 * Returns null if the string is invalid or cannot be parsed.
 *
 * Supported formats:
 * - "HH:MM:SS.mmm" (full format)
 * - "HH:MM:SS" (no milliseconds, defaults to 0)
 * - "MM:SS.mmm" (no hours, interprets as 0:MM:SS.mmm)
 * - "MM:SS" (no hours or milliseconds)
 *
 * @example parseTimestampMs("01:30:00.500") // 5400500
 * @example parseTimestampMs("invalid") // null
 * @example parseTimestampMs("") // null
 */
export const parseTimestampMs = (str: string): number | null => {
  if (!str || typeof str !== "string") {
    return null;
  }

  const trimmed = str.trim();
  if (trimmed === "") {
    return null;
  }

  // Pattern for HH:MM:SS.mmm or HH:MM:SS
  const fullPattern = /^(\d{1,2}):(\d{1,2}):(\d{1,2})(?:\.(\d{1,3}))?$/;
  // Pattern for MM:SS.mmm or MM:SS (no hours)
  const shortPattern = /^(\d{1,2}):(\d{1,2})(?:\.(\d{1,3}))?$/;

  let hours = 0;
  let minutes = 0;
  let seconds = 0;
  let millis = 0;

  const fullMatch = trimmed.match(fullPattern);
  if (fullMatch) {
    hours = parseInt(fullMatch[1], 10);
    minutes = parseInt(fullMatch[2], 10);
    seconds = parseInt(fullMatch[3], 10);
    millis = fullMatch[4] ? parseInt(fullMatch[4].padEnd(3, "0"), 10) : 0;
  } else {
    const shortMatch = trimmed.match(shortPattern);
    if (shortMatch) {
      hours = 0;
      minutes = parseInt(shortMatch[1], 10);
      seconds = parseInt(shortMatch[2], 10);
      millis = shortMatch[3] ? parseInt(shortMatch[3].padEnd(3, "0"), 10) : 0;
    } else {
      return null;
    }
  }

  // Validate ranges
  if (minutes >= 60 || seconds >= 60 || millis >= 1000) {
    return null;
  }

  const totalMs = hours * 3600000 + minutes * 60000 + seconds * 1000 + millis;

  // Handle negative input (shouldn't happen with our pattern, but just in case)
  if (totalMs < 0) {
    return null;
  }

  return totalMs;
};

/**
 * Detects numbered chapter patterns and generates the next chapter title.
 * Used when adding new chapters to suggest the next logical title.
 *
 * Pattern detection rules:
 * - "Chapter 3" -> "Chapter 4"
 * - "Ch. 3" -> "Ch. 4"
 * - "Chapter 3: The Title" -> "Chapter 4"
 * - "3: The Title" -> "4"
 * - No pattern -> empty string
 */
export const getNextChapterTitle = (previousTitle: string): string => {
  // Pattern: "Chapter N" or "Chapter N: ..."
  const chapterPattern = /^(Chapter\s+)(\d+)(.*)$/i;
  const chapterMatch = previousTitle.match(chapterPattern);
  if (chapterMatch) {
    const prefix = chapterMatch[1];
    const num = parseInt(chapterMatch[2], 10);
    return `${prefix}${num + 1}`;
  }

  // Pattern: "Ch. N" or "Ch. N: ..."
  const chPattern = /^(Ch\.\s*)(\d+)(.*)$/i;
  const chMatch = previousTitle.match(chPattern);
  if (chMatch) {
    const prefix = chMatch[1];
    const num = parseInt(chMatch[2], 10);
    return `${prefix}${num + 1}`;
  }

  // Pattern: "N" or "N: ..."
  const numberPattern = /^(\d+)(.*)$/;
  const numberMatch = previousTitle.match(numberPattern);
  if (numberMatch) {
    const num = parseInt(numberMatch[1], 10);
    return `${num + 1}`;
  }

  // No pattern detected
  return "";
};

/**
 * Converts Chapter[] to ChapterInput[] for editing.
 * Strips out server-generated fields (id, file_id, parent_id, sort_order, created_at, updated_at)
 * and keeps only editable fields: title, start_page, start_timestamp_ms, href, children.
 *
 * Note: sort_order is NOT included in ChapterInput - the API derives it from array order.
 */
export const chaptersToInputArray = (chapters: Chapter[]): ChapterInput[] => {
  return chapters.map((chapter) => ({
    title: chapter.title,
    href: chapter.href,
    start_page: chapter.start_page,
    start_timestamp_ms: chapter.start_timestamp_ms,
    children: chapter.children
      ? chaptersToInputArray(chapter.children.filter(Boolean) as Chapter[])
      : [],
  }));
};

/**
 * Counts the total number of descendants (children, grandchildren, etc.) for a chapter.
 * Used for delete confirmation dialogs to show how many chapters will be deleted.
 */
export const countDescendants = (chapter: Chapter | ChapterInput): number => {
  const children = (chapter.children?.filter(Boolean) ?? []) as (
    | Chapter
    | ChapterInput
  )[];
  if (children.length === 0) {
    return 0;
  }
  return children.reduce(
    (total, child) => total + 1 + countDescendants(child),
    0,
  );
};
