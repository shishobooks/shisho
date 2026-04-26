export type SeriesNumberUnit = "volume" | "chapter";

function formatNumber(n: number): string {
  return n.toString();
}

export function formatSeriesNumber(
  number: number | null | undefined,
  unit: SeriesNumberUnit | null | undefined,
  fileType: string | null | undefined,
): string {
  if (number === null || number === undefined) return "";
  if (fileType === "cbz") {
    if (unit === "chapter") return `Ch. ${formatNumber(number)}`;
    return `Vol. ${formatNumber(number)}`;
  }
  return `#${formatNumber(number)}`;
}
