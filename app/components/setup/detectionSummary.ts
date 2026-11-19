import type { DetectFileTypesResponse } from "@/types";
import { formatCount } from "@/utils/format";

const englishList = (items: string[]) => {
  if (items.length <= 1) return items.join("");
  if (items.length === 2) return `${items[0]} and ${items[1]}`;
  return `${items.slice(0, -1).join(", ")}, and ${items[items.length - 1]}`;
};

/** What Setup says about the folders it looked through. */
export const detectionSummary = (
  detected: DetectFileTypesResponse | undefined,
): string => {
  if (!detected) {
    return "We couldn't look through those folders, so here are the main plugins.";
  }
  const found = [
    detected.ebooks > 0 && formatCount(detected.ebooks, "ebook"),
    detected.audiobooks > 0 && formatCount(detected.audiobooks, "audiobook"),
    detected.comics > 0 && formatCount(detected.comics, "comic"),
  ].filter((s): s is string => Boolean(s));
  if (found.length === 0) {
    return detected.stopped_early
      ? "We looked through part of those folders and didn't find any books yet, so here are the main plugins."
      : "We didn't find any books in those folders yet, so here are the main plugins.";
  }
  // The walk stops on a time or a file limit, so it says it looked through
  // part of the folders rather than naming how many files that was.
  if (detected.stopped_early) {
    return `Your folders are large, so we looked through part of them and found about ${englishList(found)}.`;
  }
  return `We found about ${englishList(found)}.`;
};
