import { describe, expect, it } from "vitest";

import { durationTicks, niceTicks } from "./chartTicks";

describe("niceTicks", () => {
  it("rounds the top up to a clean step", () => {
    expect(niceTicks(6)).toEqual([0, 2, 4, 6]);
    expect(niceTicks(7)).toEqual([0, 2, 4, 6, 8]);
    expect(niceTicks(18319)).toEqual([0, 5000, 10000, 15000, 20000]);
    expect(niceTicks(1)).toEqual([0, 1]);
  });

  it("still has an axis with nothing to show", () => {
    expect(niceTicks(0)).toEqual([0, 1]);
  });
});

describe("durationTicks", () => {
  it("steps in whole hours from an hour up", () => {
    const axis = durationTicks(7.2 * 3600);
    expect(axis.ticks).toEqual([0, 2, 4, 6, 8].map((h) => h * 3600));
    expect(axis.ticks.map(axis.format)).toEqual(["0", "2h", "4h", "6h", "8h"]);
  });

  it("steps in minutes under an hour", () => {
    const axis = durationTicks(25 * 60);
    expect(axis.ticks).toEqual([0, 10, 20, 30].map((m) => m * 60));
    expect(axis.ticks.map(axis.format)).toEqual(["0", "10m", "20m", "30m"]);
  });

  it("still has an axis with no time", () => {
    const axis = durationTicks(0);
    expect(axis.ticks.map(axis.format)).toEqual(["0", "1m"]);
  });
});
