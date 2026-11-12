import { describe, expect, it } from "vitest";

import { growthSeries } from "./growthSeries";

const months = (from: string, counts: number[]) => {
  const [year, month] = from.split("-").map(Number);
  return counts.map((books, i) => {
    const d = new Date(Date.UTC(year, month - 1 + i, 1));
    return {
      month: `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`,
      books,
    };
  });
};

describe("growthSeries", () => {
  it("is empty with no months", () => {
    expect(growthSeries([])).toEqual({ unit: "month", points: [] });
  });

  it("pads a new server out to a year with empty months before it", () => {
    const { unit, points: series } = growthSeries(months("2026-11", [6]));

    expect(unit).toBe("month");
    expect(series).toHaveLength(12);
    expect(series[0]).toEqual({
      label: "Dec 2025",
      added: 0,
      total: 0,
    });
    expect(series[11]).toEqual({ label: "Nov 2026", added: 6, total: 6 });
  });

  it("keeps a running total by month", () => {
    const { points: series } = growthSeries(
      months("2025-12", [10, 0, 5, ...Array(9).fill(1)]),
    );

    expect(series).toHaveLength(12);
    expect(series.map((p) => p.total)).toEqual([
      10, 10, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24,
    ]);
    expect(series[2]).toEqual({ label: "Feb 2026", added: 5, total: 15 });
  });

  it("groups by quarter past three years of months", () => {
    // 2023-01 to 2026-12 is 48 months.
    const { unit, points: series } = growthSeries(
      months("2023-01", Array(48).fill(1)),
    );

    expect(unit).toBe("quarter");
    expect(series).toHaveLength(16);
    expect(series[0]).toEqual({ label: "Q1 2023", added: 3, total: 3 });
    expect(series[15]).toEqual({ label: "Q4 2026", added: 3, total: 48 });
  });

  it("starts a quarter partway through when the data does", () => {
    // Nov 2022 to Dec 2025: 38 months.
    const { points: series } = growthSeries(
      months("2022-11", Array(38).fill(2)),
    );

    expect(series[0]).toEqual({ label: "Q4 2022", added: 4, total: 4 });
    expect(series[series.length - 1]).toEqual({
      label: "Q4 2025",
      added: 6,
      total: 76,
    });
  });

  it("groups by year past nine years of quarters", () => {
    // 2010-01 to 2026-12: 17 years.
    const { unit, points: series } = growthSeries(
      months("2010-01", Array(204).fill(1)),
    );

    expect(unit).toBe("year");
    expect(series).toHaveLength(17);
    expect(series[0]).toEqual({ label: "2010", added: 12, total: 12 });
    expect(series[series.length - 1]).toEqual({
      label: "2026",
      added: 12,
      total: 204,
    });
  });
});
