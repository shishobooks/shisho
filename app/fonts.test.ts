import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

// A font stack that names a family nothing declares falls through to the
// next one without a word, which is how "Noto Sans Japanese" went unused while
// the stylesheet loaded "Noto Sans JP". Every family a stack names must be
// declared by a font package the same stylesheet imports, be a generic
// family, or be an OS emoji font, which stays OS-native on purpose.
const root = path.resolve(__dirname, "..");
const stylesheets = ["app/index.css", "website/src/css/custom.css"];
const allowed = new Set([
  "ui-sans-serif",
  "ui-monospace",
  "system-ui",
  "sans-serif",
  "serif",
  "monospace",
  "Apple Color Emoji",
  "Segoe UI Emoji",
  "Segoe UI Symbol",
  "Noto Color Emoji",
]);

const read = (file: string) => readFileSync(path.join(root, file), "utf8");
const withoutComments = (css: string) => css.replace(/\/\*[\s\S]*?\*\//g, "");

// Finds an imported package's stylesheet the way Node finds a package: in the
// nearest node_modules from the importing file up to the repository root.
const packageStylesheet = (from: string, spec: string) => {
  const file = spec.endsWith(".css") ? spec : `${spec}/index.css`;
  for (
    let dir = path.dirname(path.join(root, from));
    dir.startsWith(root);
    dir = path.dirname(dir)
  ) {
    const candidate = path.join(dir, "node_modules", file);
    if (existsSync(candidate)) return readFileSync(candidate, "utf8");
  }
  throw new Error(`${from} imports ${spec}, which isn't installed`);
};

const familyNames = (stack: string) =>
  stack
    .split(",")
    .map((name) => name.trim().replace(/^(["'])(.*)\1$/, "$2"))
    .filter((name) => name && !name.startsWith("var("));

const declarations = (css: string, property: RegExp) =>
  [...css.matchAll(new RegExp(`(?:${property.source})\\s*:([^;{}]+);`, "g"))]
    .map((match) => match[1])
    .flatMap(familyNames);

const declaredFamilies = (stylesheet: string) => {
  const css = withoutComments(read(stylesheet));
  const imports = [...css.matchAll(/@import\s+["'](@fontsource[^"']+)["']/g)];
  return new Set(
    imports.flatMap(([, spec]) =>
      declarations(packageStylesheet(stylesheet, spec), /font-family/),
    ),
  );
};

const stackedFamilies = (stylesheet: string) =>
  declarations(
    withoutComments(read(stylesheet)),
    /--font-[\w-]+|--ifm-font-family-[\w-]+|font-family/,
  );

describe.each(stylesheets)("%s", (stylesheet) => {
  it("names only fonts it declares", () => {
    const declared = declaredFamilies(stylesheet);
    const undeclared = stackedFamilies(stylesheet).filter(
      (family) => !declared.has(family) && !allowed.has(family),
    );
    expect(undeclared).toEqual([]);
  });
});

const sourceFiles = (dir: string): string[] =>
  readdirSync(path.join(root, dir), { recursive: true, encoding: "utf8" })
    .filter((file) => /\.(tsx?|css|html|mdx?)$/.test(file))
    .map((file) => path.join(dir, file))
    .filter((file) => !file.endsWith("fonts.test.ts"));

describe("fonts", () => {
  it("never load from a CDN", () => {
    const pages = [
      "index.html",
      "website/docusaurus.config.ts",
      ...sourceFiles("app"),
      ...sourceFiles("website/src"),
    ];
    const fromCdn = pages.filter((file) =>
      /fonts\.(googleapis|gstatic)\.com/.test(read(file)),
    );
    expect(fromCdn).toEqual([]);
  });
});
