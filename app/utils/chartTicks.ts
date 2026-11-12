// Y-axis ticks for the app's charts, shared so every chart's gridlines land
// on the same kind of round values.

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

/** A duration axis: its ticks in seconds, and how to label each. */
export interface DurationAxis {
  ticks: number[];
  format: (seconds: number) => string;
}

/**
 * Y-axis ticks for time, in seconds, on round hours from an hour up and
 * round minutes below it.
 * @example durationTicks(7.2 * 3600).ticks // [0, 7200, 14400, 21600, 28800]
 */
export const durationTicks = (maxSeconds: number): DurationAxis => {
  const unit = maxSeconds >= 3600 ? 3600 : 60;
  const suffix = unit === 3600 ? "h" : "m";
  return {
    ticks: niceTicks(maxSeconds / unit).map((t) => t * unit),
    format: (seconds) => (seconds === 0 ? "0" : `${seconds / unit}${suffix}`),
  };
};

/** Where a value sits in a ChartFrame's plot, as a percent of its height. */
export const yPercent = (ticks: number[], value: number) =>
  (value / (ticks[ticks.length - 1] || 1)) * 100;
