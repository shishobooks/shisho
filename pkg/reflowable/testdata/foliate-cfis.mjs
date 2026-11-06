// Writes the CFIs the web reader's foliate makes for places in the fixture
// book, so the Go tests expect foliate's own output rather than hand-written
// values. It runs foliate's epubcfi.js on the fixture parsed as XHTML, as the
// browser parses it. From the repository root, after TestWriteFixture:
//
//	node pkg/reflowable/testdata/foliate-cfis.mjs tmp/fixture > pkg/reflowable/testdata/handoff-cfis.json
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import process from "node:process";

import { JSDOM } from "jsdom";

const dom = new JSDOM("");
globalThis.NodeFilter = dom.window.NodeFilter;
const CFI = await import("../../../app/libraries/foliate/epubcfi.js");

const [fixtureDir] = process.argv.slice(2);
const parse = (text) =>
  new dom.window.DOMParser().parseFromString(text, "application/xhtml+xml");
const entry = (archive, name) =>
  execFileSync("unzip", ["-p", archive, name], { encoding: "utf8" });

const out = {};
for (const [variant, file] of [
  ["epub", "handoff.epub"],
  ["kepub", "handoff.kepub.epub"],
]) {
  const archive = join(fixtureDir, file);
  const opfPath = "OEBPS/content.opf";
  const opf = parse(entry(archive, opfPath));
  const itemrefs = [...opf.getElementsByTagName("itemref")];
  const spineCFIs = CFI.fromElements(itemrefs);
  const manifest = new Map(
    [...opf.getElementsByTagName("item")].map((item) => [
      item.getAttribute("id"),
      item.getAttribute("href"),
    ]),
  );
  const places = [];
  itemrefs.forEach((itemref, spine) => {
    const href = manifest.get(itemref.getAttribute("idref"));
    const doc = parse(entry(archive, "OEBPS/" + href));
    const body = doc.getElementsByTagName("body")[0];
    // Every text node in the body in order, with where it starts in the
    // body's text, counted in code points as the Go side counts.
    const walker = doc.createTreeWalker(
      body,
      dom.window.NodeFilter.SHOW_TEXT | 8,
    );
    let offset = 0;
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      if (node.nodeType !== 3 && node.nodeType !== 4) continue;
      const points = Array.from(node.nodeValue);
      for (let i = 0; i < points.length; i++) {
        // Every few characters, and fewer in the long last chapter.
        if (i % (spine === itemrefs.length - 1 ? 40 : 5) !== 0) continue;
        const range = doc.createRange();
        const units = points.slice(0, i).join("").length;
        range.setStart(node, units);
        range.collapse(true);
        const cfi = CFI.joinIndir(spineCFIs[spine], CFI.fromRange(range));
        places.push({ spine, offset: offset + i, cfi });
      }
      offset += points.length;
    }
  });
  out[variant] = places;
}
const lines = Object.entries(out).map(
  ([variant, places]) =>
    `  ${JSON.stringify(variant)}: [\n` +
    places.map((place) => "    " + JSON.stringify(place)).join(",\n") +
    "\n  ]",
);
process.stdout.write("{\n" + lines.join(",\n") + "\n}\n");
