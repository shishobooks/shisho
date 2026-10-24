import type { SeriesNumberUnit } from "@/types/generated/models";

export function formatSeriesNumber(
  number: number | null | undefined,
  end: number | null | undefined,
  unit: SeriesNumberUnit | null | undefined,
  fileType: string | null | undefined,
): string {
  if (number === null || number === undefined) return "";

  const range =
    end === null || end === undefined ? `${number}` : `${number}-${end}`;
  if (fileType === "cbz") {
    if (unit === "chapter") return `Ch. ${range}`;
    return `Vol. ${range}`;
  }
  return range;
}

/**
 * A series number as a short label: "#5" for a bare number, or a comic's
 * "Vol. 5" or "Ch. 42" as it is. Empty when there is no number.
 */
export function seriesNumberLabel(
  number: number | null | undefined,
  end: number | null | undefined,
  unit: SeriesNumberUnit | null | undefined,
  fileType: string | null | undefined,
): string {
  const formatted = formatSeriesNumber(number, end, unit, fileType);
  if (!formatted) return "";
  return /^\d/.test(formatted) ? `#${formatted}` : formatted;
}
