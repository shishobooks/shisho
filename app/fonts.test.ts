import { existsSync, readFileSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

// A font stack that names a family nothing declares falls through to the
// next one without a word, which is how "Noto Sans Japanese" went unused while
// the stylesheet loaded "Noto Sans JP". Every family a stack names must be
// declared by a font package the same stylesheet imports, or be an OS emoji
// font, which stays OS-native on purpose.
const root = path.resolve(__dirname, "..");
const stylesheets = ["app/index.css", "website/src/css/custom.css"];
const osEmojiFonts = new Set([
  "Apple Color Emoji",
  "Segoe UI Emoji",
  "Segoe UI Symbol",
  "Noto Color Emoji",
]);

const read = (file: string) => readFileSync(path.join(root, file), "utf8");

const packageStylesheet = (from: string, spec: string) => {
  const file = spec.endsWith(".css") ? spec : `${spec}/index.css`;
  const candidates = [
    path.join(root, path.dirname(from), "..", "..", "node_modules", file),
    path.join(root, "node_modules", file),
  ];
  const found = candidates.find((candidate) => existsSync(candidate));
  if (!found) throw new Error(`${from} imports ${spec}, which isn't installed`);
  return readFileSync(found, "utf8");
};

const declaredFamilies = (stylesheet: string) => {
  const imports = [
    ...read(stylesheet).matchAll(/@import\s+"(@fontsource[^"]+)"/g),
  ].map((match) => match[1]);
  return new Set(
    imports.flatMap((spec) =>
      [
        ...packageStylesheet(stylesheet, spec).matchAll(
          /font-family:\s*'([^']+)'/g,
        ),
      ].map((match) => match[1]),
    ),
  );
};

const stackedFamilies = (stylesheet: string) =>
  [
    ...read(stylesheet).matchAll(
      /(?:--font-[\w-]+|--ifm-font-family-[\w-]+|font-family)\s*:([^;{}]+);/g,
    ),
  ].flatMap((match) =>
    [...match[1].matchAll(/"([^"]+)"/g)].map((name) => name[1]),
  );

describe.each(stylesheets)("%s", (stylesheet) => {
  it("names only fonts it declares", () => {
    const declared = declaredFamilies(stylesheet);
    const undeclared = stackedFamilies(stylesheet).filter(
      (family) => !declared.has(family) && !osEmojiFonts.has(family),
    );
    expect(undeclared).toEqual([]);
  });
});

describe("fonts", () => {
  it("never load from a CDN", () => {
    const pages = [
      "index.html",
      "website/docusaurus.config.ts",
      ...stylesheets,
    ];
    const fromCdn = pages.filter((file) =>
      /fonts\.(googleapis|gstatic)\.com/.test(read(file)),
    );
    expect(fromCdn).toEqual([]);
  });
});
