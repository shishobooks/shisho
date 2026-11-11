import type { GrowthMonth } from "@/types/generated/serverstats";
import { formatMonthYear } from "@/utils/format";

/** One point on the Library Growth chart. */
export interface GrowthPoint {
  /** "Nov 2026", "Q4 2026", or "2026", by the chart's grouping. */
  label: string;
  /** Books added in this month, quarter, or year. */
  added: number;
  /** Books in the library by the end of it. */
  total: number;
}

/** How the chart groups its points. */
export type GrowthUnit = "month" | "quarter" | "year";

/** The chart's points, grouped by `unit`. */
export interface GrowthSeries {
  unit: GrowthUnit;
  points: GrowthPoint[];
}

/** A chart this short or shorter is padded out with empty months before it. */
const MIN_MONTHS = 12;
/** Past this many points, the chart groups into the next coarser unit. */
const MAX_POINTS = 36;

const monthKey = (year: number, monthIndex: number) => {
  const d = new Date(Date.UTC(year, monthIndex, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
};

const parseMonth = (month: string) => {
  const [year, m] = month.split("-").map(Number);
  return { year, monthIndex: m - 1 };
};

/**
 * Turns Books added per month into the running total the chart plots.
 *
 * A server younger than a year is padded with empty months before its first,
 * so a lone month reads as a new server rather than a full-height block.
 * Past three years of months the points become quarters, and past nine years
 * of quarters, years, so the line keeps a readable number of points.
 * `months` comes from the server with every month present and in order.
 */
export const growthSeries = (months: GrowthMonth[]): GrowthSeries => {
  if (months.length === 0) return { unit: "month", points: [] };

  const padded = [...months];
  const { year, monthIndex } = parseMonth(months[0].month);
  for (let i = 1; padded.length < MIN_MONTHS; i++) {
    padded.unshift({ month: monthKey(year, monthIndex - i), books: 0 });
  }

  const bucketOf = (unit: GrowthUnit, month: string) => {
    const { year: y, monthIndex: m } = parseMonth(month);
    if (unit === "year") return String(y);
    if (unit === "quarter") return `Q${Math.floor(m / 3) + 1} ${y}`;
    return formatMonthYear(month);
  };
  const countBuckets = (unit: GrowthUnit) =>
    new Set(padded.map((m) => bucketOf(unit, m.month))).size;
  const unit: GrowthUnit =
    padded.length <= MAX_POINTS
      ? "month"
      : countBuckets("quarter") <= MAX_POINTS
        ? "quarter"
        : "year";

  const points: GrowthPoint[] = [];
  let total = 0;
  for (const m of padded) {
    total += m.books;
    const label = bucketOf(unit, m.month);
    const last = points[points.length - 1];
    if (last && last.label === label) {
      last.added += m.books;
      last.total = total;
    } else {
      points.push({ label, added: m.books, total });
    }
  }
  return { unit, points };
};

/**
 * Y-axis ticks from 0 to a clean value at or above `max`, at most five, on a
 * step of 1, 2, or 5 times a power of ten.
 * @example niceTicks(18319) // [0, 5000, 10000, 15000, 20000]
 */
export const niceTicks = (max: number): number[] => {
  if (max <= 0) return [0, 1];
  let step = 1;
  for (let magnitude = 1; ; magnitude *= 10) {
    const fit = [1, 2, 5].find((f) => Math.ceil(max / (f * magnitude)) <= 4);
    if (fit) {
      step = fit * magnitude;
      break;
    }
  }
  const ticks: number[] = [];
  for (let v = 0; v < max + step; v += step) {
    ticks.push(v);
    if (v >= max) break;
  }
  return ticks;
};
