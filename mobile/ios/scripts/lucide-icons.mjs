// Writes the Lucide icons the Shisho app uses into its asset catalog, from
// the lucide-react the web app is built with, so both draw the same icons.
// The names are the raw values of the `Lucide` enum in
// Shisho/UI/Icons.swift; a name ending in "-fill" is that icon with its
// shapes filled, as the web fills a rated star. Each is a template image
// under Lucide/, drawn on Lucide's 24 point grid with a stroke of 2.
//
// It reads lucide-react's built modules (dist/esm/icons/<name>.mjs and their
// __iconData export), so a lucide-react upgrade that changes that layout
// fails here rather than drawing nothing.
//
// `mise ios:icons` writes them; commit what it writes. `--check` (run by
// `mise lint:icons`) writes nothing and fails when the catalog differs from
// what lucide-react draws now, as after a lucide-react bump, or when the
// app's Swift names an SF Symbol, which never draws the web's icons.

import { mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const check = process.argv.includes("--check");
const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..", "..");
const app = join(here, "..", "Shisho");
const catalog = join(app, "Resources", "Assets.xcassets", "Lucide");
const iconsDir = join(
  root,
  "node_modules",
  "lucide-react",
  "dist",
  "esm",
  "icons",
);

// The `Lucide` enum's raw values: a case's own name, or its explicit value.
const iconNames = async () => {
  const source = await readFile(join(app, "UI", "Icons.swift"), "utf8");
  // The enum's body ends at the first closing brace in the first column.
  const body = /enum Lucide[^{]*\{([\s\S]*?)\n\}/.exec(source);
  if (!body) throw new Error("no `enum Lucide` in Icons.swift");
  // Each case on a line of its own, perhaps with a comment; any other
  // case line is refused rather than skipped, so no icon goes unchecked.
  const names = [];
  for (const line of body[1].split("\n")) {
    if (!/^\s*case\b/.test(line)) continue;
    const match = /^\s*case (\w+)(?:\s*=\s*"([^"]+)")?\s*(?:\/\/.*)?$/.exec(
      line,
    );
    if (!match)
      throw new Error(
        `can't read \`${line.trim()}\` in Icons.swift; give each case a line of its own`,
      );
    names.push(match[2] ?? match[1]);
  }
  return names;
};

const escape = (value) =>
  String(value)
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;");

// One element of an icon's node, as an SVG element. The asset catalog draws
// SVG without currentColor, so it's drawn in black and the image is a
// template.
const element = ([tag, attrs]) => {
  const fields = Object.entries(attrs)
    .filter(([name]) => name !== "key")
    .map(([name, value]) => `${name}="${escape(value)}"`)
    .join(" ");
  return `<${tag} ${fields}/>`;
};

// An icon's drawing. A renamed icon's module only re-exports the new name's,
// so it's followed there.
const iconData = async (name) => {
  const file = join(iconsDir, `${name}.mjs`);
  const alias = /export \{ default \} from '\.\/(.+)\.mjs'/.exec(
    await readFile(file, "utf8"),
  );
  if (alias) return iconData(alias[1]);
  const data = (await import(file)).__iconData;
  if (!data?.node)
    throw new Error(`lucide-react's ${name}.mjs has no __iconData`);
  return data;
};

const json = (value) => JSON.stringify(value, null, 2) + "\n";

// Every file the catalog should hold, by path under it.
const files = new Map();
files.set(
  "Contents.json",
  json({
    info: { author: "xcode", version: 1 },
    properties: { "provides-namespace": true },
  }),
);
for (const asset of await iconNames()) {
  const filled = asset.endsWith("-fill");
  const data = await iconData(filled ? asset.slice(0, -"-fill".length) : asset);
  files.set(
    join(`${asset}.imageset`, `${asset}.svg`),
    `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" ` +
      `fill="${filled ? "#000" : "none"}" stroke="#000" stroke-width="2" stroke-linecap="round" ` +
      `stroke-linejoin="round">${data.node.map(element).join("")}</svg>\n`,
  );
  files.set(
    join(`${asset}.imageset`, "Contents.json"),
    json({
      images: [{ filename: `${asset}.svg`, idiom: "universal" }],
      info: { author: "xcode", version: 1 },
      properties: {
        "preserves-vector-representation": true,
        "template-rendering-intent": "template",
      },
    }),
  );
}

// Every file under dir, by path under it.
const listFiles = async (dir) => {
  const out = [];
  for (const entry of await readdir(dir, {
    withFileTypes: true,
    recursive: true,
  })) {
    if (entry.isFile())
      out.push(relative(dir, join(entry.parentPath, entry.name)));
  }
  return out;
};

if (!check) {
  await rm(catalog, { recursive: true, force: true });
  for (const [path, content] of files) {
    await mkdir(dirname(join(catalog, path)), { recursive: true });
    await writeFile(join(catalog, path), content);
  }
  console.log(`wrote ${files.size} files to ${catalog}`);
} else {
  const problems = [];
  const committed = await listFiles(catalog).catch(() => []);
  for (const path of committed) {
    if (!files.has(path))
      problems.push(`${path} isn't a Lucide icon the app names`);
  }
  for (const [path, content] of files) {
    const current = await readFile(join(catalog, path), "utf8").catch(
      () => null,
    );
    if (current !== content)
      problems.push(
        `${path} ${current === null ? "is missing" : "differs from lucide-react's"}`,
      );
  }
  for (const path of await listFiles(app)) {
    if (!path.endsWith(".swift")) continue;
    const lines = (await readFile(join(app, path), "utf8")).split("\n");
    lines.forEach((line, index) => {
      if (/\b(systemName|systemImage):/.test(line)) {
        problems.push(
          `Shisho/${path}:${index + 1} names an SF Symbol; use Icon and Lucide instead`,
        );
      }
    });
  }
  if (problems.length) {
    console.error(problems.join("\n"));
    console.error(
      "Run `mise ios:icons` and commit what it writes, or swap the SF Symbol for a Lucide icon.",
    );
    process.exit(1);
  }
  console.log(
    `the app's ${files.size - 1} Lucide files match lucide-react, and it names no SF Symbols`,
  );
}
