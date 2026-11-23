// Writes the Lucide icons the Shisho app uses into its asset catalog, from
// the lucide-react the web app is built with, so both draw the same icons.
// Each is a template image under Lucide/, such as Image("Lucide/moon"),
// drawn on Lucide's 24 point grid with a stroke of 2 that scales with it,
// as on the web.
//
// A name ending in ":fill" also fills the icon's shapes, as the web fills a
// rated star, and is written as "<name>-fill".
//
// Run `mise ios:icons` after adding a name to `icons` or bumping
// lucide-react, and commit what it writes.

import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const icons = [
  "arrow-down",
  "arrow-down-up",
  "arrow-up",
  "arrow-up-right",
  "book",
  "book-open",
  "book-open-check",
  "bookmark",
  "bookmark-plus",
  "check",
  "chevron-down",
  "chevron-right",
  "chevron-up",
  "chevrons-up-down",
  "circle-alert",
  "clock",
  "clock-arrow-up",
  "ellipsis",
  "file",
  "file-search",
  "file-x",
  "headphones",
  "languages",
  "layers",
  "library",
  "list",
  "list-filter",
  "list-music",
  "list-x",
  "lock",
  "log-out",
  "message-circle-warning",
  "minus",
  "moon",
  "moon:fill",
  "pause",
  "pause:fill",
  "pencil",
  "play",
  "play:fill",
  "plus",
  "refresh-cw",
  "rotate-ccw",
  "rotate-cw",
  "search",
  "server",
  "share-2",
  "skip-back",
  "skip-forward",
  "square",
  "square-check-big",
  "star",
  "star:fill",
  "star-half:fill",
  "tags",
  "trash-2",
  "triangle-alert",
  "user",
  "user-x",
  "users",
  "wifi-off",
  "x",
];

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..", "..");
const catalog = join(
  here,
  "..",
  "Shisho",
  "Resources",
  "Assets.xcassets",
  "Lucide",
);
const iconsDir = join(
  root,
  "node_modules",
  "lucide-react",
  "dist",
  "esm",
  "icons",
);

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
  return (await import(file)).__iconData;
};

const json = (value) => JSON.stringify(value, null, 2) + "\n";

await rm(catalog, { recursive: true, force: true });
await mkdir(catalog, { recursive: true });
await writeFile(
  join(catalog, "Contents.json"),
  json({
    info: { author: "xcode", version: 1 },
    properties: { "provides-namespace": true },
  }),
);
for (const entry of icons) {
  const [name, variant] = entry.split(":");
  const filled = variant === "fill";
  const asset = filled ? `${name}-fill` : name;
  const data = await iconData(name);
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" ` +
    `fill="${filled ? "#000" : "none"}" stroke="#000" stroke-width="2" stroke-linecap="round" ` +
    `stroke-linejoin="round">${data.node.map(element).join("")}</svg>\n`;
  const set = join(catalog, `${asset}.imageset`);
  await mkdir(set);
  await writeFile(join(set, `${asset}.svg`), svg);
  await writeFile(
    join(set, "Contents.json"),
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
console.log(`wrote ${icons.length} icons to ${catalog}`);
