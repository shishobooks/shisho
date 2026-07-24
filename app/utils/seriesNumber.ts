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
