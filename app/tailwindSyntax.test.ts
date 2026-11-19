import { readFileSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

// Tailwind v4 reads a CSS variable as an arbitrary value only in parentheses,
// `origin-(--radix-popover-content-transform-origin)` or `w-(length:--x)`.
// The v3 bracket form, `origin-[--radix-…]`, still compiles but emits the
// bare name as the value, which the browser drops, so the class silently
// does nothing. shadcn components copied from v3-era docs carry the old
// form. `[--name:value]`, an arbitrary property, is fine.
//
// This is a test rather than an ESLint rule because ESLint ignores
// app/components/ui, where the shadcn components live.
const sources = import.meta.glob<string>("./**/*.{ts,tsx,css}", {
  query: "?raw",
  import: "default",
  eager: true,
});

const V3_VARIABLE = /[\w-]+-\[(?:[a-z-]+:)?--[\w-]+\]/g;

describe("Tailwind classes", () => {
  it("write CSS variables as v4 arbitrary values", () => {
    const offenders = Object.entries(sources).flatMap(([file, source]) =>
      file.endsWith("tailwindSyntax.test.ts")
        ? []
        : (source.match(V3_VARIABLE) ?? []).map((cls) => `${file}: ${cls}`),
    );
    expect(
      offenders,
      "Write a CSS variable value as name-(--var), not name-[--var]",
    ).toEqual([]);
  });
});

// A color utility naming a theme color that index.css never defines, such as
// `text-destructive-foreground` before --destructive-foreground existed,
// compiles to nothing, so the element falls back to the inherited color.
const FOREGROUND_COLOR =
  /\b(?:bg|text|border|ring|fill|stroke)-([a-z-]+-foreground)\b/g;

describe("Theme colors", () => {
  it("are defined for every foreground color a class names", () => {
    // Vitest loads CSS as an empty string, so the stylesheet is read from
    // disk.
    const css = readFileSync(path.resolve(__dirname, "index.css"), "utf8");
    const missing = new Set<string>();
    for (const [file, source] of Object.entries(sources)) {
      if (!file.endsWith(".tsx") && !file.endsWith(".ts")) continue;
      if (file.endsWith("tailwindSyntax.test.ts")) continue;
      for (const [, name] of source.matchAll(FOREGROUND_COLOR)) {
        if (!css.includes(`--color-${name}:`)) missing.add(`${file}: ${name}`);
      }
    }
    expect(
      [...missing],
      "Define the color as --<name> in :root and .dark, and --color-<name> in @theme inline",
    ).toEqual([]);
  });
});

// RadioGroup lays its items out as a grid with a gap, and space-y-* adds a
// margin on top of it, so the items end up twice as far apart as intended.
const RADIO_GROUP_SPACE_Y = /<RadioGroup\b[^>]*\bspace-y-/g;

describe("RadioGroup", () => {
  it("is spaced with gap, not space-y", () => {
    const offenders = Object.entries(sources).flatMap(([file, source]) =>
      file.endsWith(".tsx") && source.match(RADIO_GROUP_SPACE_Y) ? [file] : [],
    );
    expect(
      offenders,
      "Space RadioGroup items with gap-*: the grid's gap and space-y's margins add up",
    ).toEqual([]);
  });
});
