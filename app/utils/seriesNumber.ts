import type { SeriesNumberUnit } from "@/types/generated/models";

export function formatSeriesNumber(
  number: number | null | undefined,
  unit: SeriesNumberUnit | null | undefined,
  fileType: string | null | undefined,
): string {
  if (number === null || number === undefined) return "";
  if (fileType === "cbz") {
    if (unit === "chapter") return `Ch. ${number}`;
    return `Vol. ${number}`;
  }
  return `#${number}`;
}
