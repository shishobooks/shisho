import fs from "fs";
import path from "path";

import { describe, expect, it } from "vitest";

import { legacyDocs } from "./legacyDocs";
import sidebars from "./sidebars";

// A page missing from sidebars.ts still builds and is reachable only by URL,
// so nothing else notices it. Every current doc must be placed in the sidebar
// or named in legacyDocs, and legacyDocs must not go stale.

const docsDir = path.join(__dirname, "docs");

const docIds = (dir: string): string[] =>
  fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return docIds(full);
    if (!/\.mdx?$/.test(entry.name)) return [];
    return [
      path
        .relative(docsDir, full)
        .replace(/\.mdx?$/, "")
        .split(path.sep)
        .join("/"),
    ];
  });

// Collects every doc id a sidebar item points at: bare ids, doc and ref
// items, category links, and nested category items.
const sidebarIds = (item: unknown): string[] => {
  if (typeof item === "string") return [item];
  if (Array.isArray(item)) return item.flatMap(sidebarIds);
  if (item && typeof item === "object") {
    const record = item as Record<string, unknown>;
    return [
      ...(typeof record.id === "string" ? [record.id] : []),
      ...sidebarIds(record.link),
      ...sidebarIds(record.items),
      ...(record.type === undefined && !("id" in record)
        ? Object.values(record).flatMap(sidebarIds)
        : []),
    ];
  }
  return [];
};

describe("sidebars.ts", () => {
  const docs = docIds(docsDir);
  const placed = new Set(sidebarIds(Object.values(sidebars)));
  const legacy = Object.keys(legacyDocs);

  it("places every current doc or lists it in legacyDocs", () => {
    const orphans = docs.filter(
      (id) => !placed.has(id) && !legacy.includes(id),
    );
    expect(
      orphans,
      "Add these pages to sidebars.ts where their reader would look for them",
    ).toEqual([]);
  });

  it("lists only legacy docs that exist and are not in the sidebar", () => {
    const stale = legacy.filter((id) => !docs.includes(id) || placed.has(id));
    expect(
      stale,
      "Remove these from legacyDocs: they were deleted or are now placed",
    ).toEqual([]);
  });
});
