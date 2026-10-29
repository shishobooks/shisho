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
