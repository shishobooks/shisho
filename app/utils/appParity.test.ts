import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

// Reading Stats is drawn twice, by the web app and by the Shisho app, and
// the two should say and look the same. These read both sides' sources and
// fail when one gains, loses, or rewords a card, tile, or note the other
// doesn't, or when the app's chart colors drift from the web's.

const read = (path: string) =>
  readFileSync(fileURLToPath(new URL(path, import.meta.url)), "utf8");

const webPage = read("../components/pages/ReadingStatsPage.tsx");
const webSections = read("../components/library/ReadingStatsSections.tsx");
const appStats = read("../../mobile/ios/Shisho/UI/ReadingStatsView.swift");
const css = read("../index.css");

const squash = (text: string) => text.replace(/\s+/g, " ").trim();

const between = (source: string, start: string, end: string) => {
  const from = source.indexOf(start);
  const to = source.indexOf(end, from);
  if (from < 0 || to < 0) {
    throw new Error(`can't find ${start} … ${end}`);
  }
  return source.slice(from, to);
};

const all = (source: string, pattern: RegExp) =>
  [...source.matchAll(pattern)].flatMap((m) =>
    m.slice(1).filter((s): s is string => s !== undefined),
  );

// What the web's Overview shows: its tiles, its cards' titles (a range's
// two wordings included), their empty messages, and the notes under them.
const webStrings = () => {
  const body = between(webPage, "const StatsBody", "const FINISHED_PER_PAGE");
  return [
    ...all(body, /label="([^"]+)"/g),
    ...all(body, /title="([^"]+)"/g),
    ...all(body, /title=\{\s*byYear\s*\?\s*"([^"]+)"\s*:\s*"([^"]+)"\s*\}/g),
    ...all(body, /emptyMessage="([^"]+)"/g),
    ...all(webSections, /<EmptyNote>([^<{]+)<\/EmptyNote>/g),
    ...all(
      between(webSections, "export const PaceList", "export const PlaceList"),
      /<p className="mt-3 text-xs text-muted-foreground">([^<]+)<\/p>/g,
    ),
  ].map(squash);
};

// The same from the app's Overview.
const appStrings = () =>
  [
    ...all(
      appStats,
      /StatTile(?:Frame)?\(label: String\(localized: "([^"]+)"\)/g,
    ),
    ...all(appStats, /StatsCard\(title: String\(localized: "([^"]+)"\)/g),
    ...all(
      appStats,
      /StatsCard\(title: year == nil \? String\(localized: "([^"]+)"\)\s*:\s*String\(localized: "([^"]+)"\)/g,
    ),
    ...all(appStats, /emptyMessage: String\(localized: "([^"]+)"\)/g),
    ...all(appStats, /EmptyNote\(text: String\(localized: "([^"]+)"\)\)/g),
    ...all(
      between(appStats, "private struct PaceList", "private struct PlaceList"),
      /Text\("([^"]+)"\)/g,
    ),
  ].map(squash);

describe("Reading Stats on the web and in the Shisho app", () => {
  it("finds what each side shows", () => {
    // A guard against the patterns silently matching nothing.
    expect(webStrings().length).toBeGreaterThan(15);
    expect(appStrings().length).toBeGreaterThan(15);
  });

  it("says everything the web says in the app", () => {
    const app = squash(appStats);
    const missing = webStrings().filter((s) => !app.includes(`"${s}"`));
    expect(missing).toEqual([]);
  });

  it("says everything the app says on the web", () => {
    const web = squash(webPage + webSections);
    // A string the app fills in, like "No \(noun.many) match.", can't be
    // looked for word for word.
    const missing = appStrings()
      .filter((s) => !s.includes("\\("))
      .filter((s) => !web.includes(s));
    expect(missing).toEqual([]);
  });
});

// OKLCH to 8-bit sRGB, as CSS Color 4 converts it, for comparing the web's
// colors with the app's asset catalog.
const oklchToSRGB = (l: number, c: number, h: number): number[] => {
  const a = c * Math.cos((h * Math.PI) / 180);
  const b = c * Math.sin((h * Math.PI) / 180);
  const lms = [
    l + 0.3963377774 * a + 0.2158037573 * b,
    l - 0.1055613458 * a - 0.0638541728 * b,
    l - 0.0894841775 * a - 1.291485548 * b,
  ].map((v) => v ** 3);
  const linear = [
    4.0767416621 * lms[0] - 3.3077115913 * lms[1] + 0.2309699292 * lms[2],
    -1.2684380046 * lms[0] + 2.6097574011 * lms[1] - 0.3413193965 * lms[2],
    -0.0041960863 * lms[0] - 0.7034186147 * lms[1] + 1.707614701 * lms[2],
  ];
  return linear.map((v) => {
    const x = Math.min(1, Math.max(0, v));
    const encoded = x <= 0.0031308 ? 12.92 * x : 1.055 * x ** (1 / 2.4) - 0.055;
    return Math.round(255 * encoded);
  });
};

const cssColor = (block: string, name: string) => {
  const match = block.match(
    new RegExp(`--${name}:\\s*oklch\\(([\\d.]+) ([\\d.]+) ([\\d.]+)\\)`),
  );
  if (!match) throw new Error(`no --${name} in that block of index.css`);
  return oklchToSRGB(Number(match[1]), Number(match[2]), Number(match[3]));
};

interface ColorSet {
  colors: {
    appearances?: { value: string }[];
    color: { components: { red: string; green: string; blue: string } };
  }[];
}

const assetColor = (name: string, dark: boolean) => {
  const set = JSON.parse(
    read(
      `../../mobile/ios/Shisho/Resources/Assets.xcassets/${name}.colorset/Contents.json`,
    ),
  ) as ColorSet;
  const entry = set.colors.find(
    (c) => (c.appearances?.some((a) => a.value === "dark") ?? false) === dark,
  );
  if (!entry)
    throw new Error(`${name} has no ${dark ? "dark" : "light"} color`);
  const { red, green, blue } = entry.color.components;
  return [red, green, blue].map((v) => Number.parseInt(v, 16));
};

describe("the Shisho app's chart colors", () => {
  const light = between(css, ":root {", "}");
  const dark = between(css, ".dark {", "}");

  it.each([1, 2, 3])("match the web's --chart-purple-%i", (step) => {
    const name = `chart-purple-${step}`;
    const asset = `ChartPurple${step}`;
    // A channel off by one is rounding, not drift.
    for (const [want, got] of [
      [cssColor(light, name), assetColor(asset, false)],
      [cssColor(dark, name), assetColor(asset, true)],
    ]) {
      want.forEach((channel, i) =>
        expect(Math.abs(channel - got[i])).toBeLessThanOrEqual(1),
      );
    }
  });
});
