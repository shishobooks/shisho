import { describe, expect, it } from "vitest";

import type { Book, File } from "@/types";

import {
  computeIdentifyEmptyState,
  pickInitialFile,
  resolveIdentifiers,
} from "./identify-utils";

describe("computeIdentifyEmptyState", () => {
  const baseInput = {
    hasEnricherPlugins: true,
    totalPlugins: 0,
    skippedPlugins: [],
    fileType: "epub",
  };

  it("tells the user to install a plugin when none are installed", () => {
    const result = computeIdentifyEmptyState({
      ...baseInput,
      hasEnricherPlugins: false,
    });
    expect(result.primary).toContain(
      "No metadata enricher plugins are installed",
    );
    expect(result.secondary).toBeUndefined();
  });

  it("reports when plugins are installed but none are enabled for this library", () => {
    const result = computeIdentifyEmptyState({
      ...baseInput,
      hasEnricherPlugins: true,
      totalPlugins: 0,
    });
    expect(result.primary).toBe(
      "No metadata enricher plugins are enabled for this library.",
    );
    expect(result.secondary).toBeUndefined();
  });

  it("reports all-skipped with singular wording for one plugin", () => {
    const result = computeIdentifyEmptyState({
      ...baseInput,
      totalPlugins: 1,
      skippedPlugins: [
        { plugin_id: "audible", plugin_name: "Audible Enricher" },
      ],
    });
    expect(result.primary).toBe(
      "No installed enricher supports EPUB files (Audible Enricher).",
    );
    expect(result.secondary).toBeUndefined();
  });

  it("reports all-skipped with plural wording for multiple plugins", () => {
    const result = computeIdentifyEmptyState({
      ...baseInput,
      totalPlugins: 2,
      skippedPlugins: [
        { plugin_id: "audible", plugin_name: "Audible Enricher" },
        { plugin_id: "librivox", plugin_name: "LibriVox Enricher" },
      ],
    });
    expect(result.primary).toBe(
      "No installed enrichers support EPUB files (Audible Enricher, LibriVox Enricher).",
    );
  });

  it("shows 'try a different query' with a secondary skip note on partial skip", () => {
    const result = computeIdentifyEmptyState({
      ...baseInput,
      totalPlugins: 2,
      skippedPlugins: [
        { plugin_id: "audible", plugin_name: "Audible Enricher" },
      ],
    });
    expect(result.primary).toBe("Try a different search query.");
    expect(result.secondary).toBe(
      "Audible Enricher was skipped because it doesn't support EPUB files.",
    );
  });

  it("pluralizes the secondary skip note when multiple plugins were skipped", () => {
    const result = computeIdentifyEmptyState({
      ...baseInput,
      totalPlugins: 3,
      skippedPlugins: [
        { plugin_id: "audible", plugin_name: "Audible Enricher" },
        { plugin_id: "librivox", plugin_name: "LibriVox Enricher" },
      ],
    });
    expect(result.primary).toBe("Try a different search query.");
    expect(result.secondary).toBe(
      "Audible Enricher, LibriVox Enricher were skipped because they don't support EPUB files.",
    );
  });

  it("shows 'try a different query' with no secondary note when nothing was skipped", () => {
    const result = computeIdentifyEmptyState({
      ...baseInput,
      totalPlugins: 2,
      skippedPlugins: [],
    });
    expect(result.primary).toBe("Try a different search query.");
    expect(result.secondary).toBeUndefined();
  });

  it("falls back to 'this file type' when fileType is undefined", () => {
    const result = computeIdentifyEmptyState({
      ...baseInput,
      totalPlugins: 1,
      fileType: undefined,
      skippedPlugins: [
        { plugin_id: "audible", plugin_name: "Audible Enricher" },
      ],
    });
    expect(result.primary).toBe(
      "No installed enricher supports this file type files (Audible Enricher).",
    );
  });

  it("uses plugin_id as a fallback when plugin_name is missing", () => {
    const result = computeIdentifyEmptyState({
      ...baseInput,
      totalPlugins: 1,
      skippedPlugins: [{ plugin_id: "audible" }],
    });
    expect(result.primary).toBe(
      "No installed enricher supports EPUB files (audible).",
    );
  });
});

describe("resolveIdentifiers", () => {
  it("returns unchanged when current and incoming are identical by type and value", () => {
    const current = [
      { type: "isbn_13", value: "9780316769488" },
      { type: "asin", value: "B01ABC1234" },
    ];
    const incoming = [
      { type: "asin", value: "B01ABC1234" },
      { type: "isbn_13", value: "9780316769488" },
    ];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("unchanged");
  });

  it("returns unchanged with empty value when both current and incoming are empty", () => {
    const result = resolveIdentifiers([], []);
    expect(result.status).toBe("unchanged");
    expect(result.value).toEqual([]);
  });

  it("returns new with deduped value when current is empty", () => {
    const incoming = [
      { type: "asin", value: "B01ABC1234" },
      { type: "asin", value: "B02DEF5678" }, // intra-incoming duplicate type
      { type: "isbn_13", value: "9780316769488" },
    ];
    const result = resolveIdentifiers([], incoming);
    expect(result.status).toBe("new");
    expect(result.value).toEqual([
      { type: "asin", value: "B02DEF5678" }, // last-wins
      { type: "isbn_13", value: "9780316769488" },
    ]);
  });

  it("returns unchanged when incoming is empty", () => {
    const current = [{ type: "asin", value: "B01ABC1234" }];
    const result = resolveIdentifiers(current, []);
    expect(result.status).toBe("unchanged");
    expect(result.value).toEqual(current);
  });

  it("returns changed and replaces existing value when incoming has same type with different value", () => {
    const current = [{ type: "asin", value: "B01ABC1234" }];
    const incoming = [{ type: "asin", value: "B02DEF5678" }];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("changed");
    expect(result.value).toEqual([{ type: "asin", value: "B02DEF5678" }]);
  });

  it("returns changed and overwrites with incoming when current has different types", () => {
    const current = [{ type: "isbn_13", value: "9780316769488" }];
    const incoming = [{ type: "asin", value: "B01ABC1234" }];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("changed");
    expect(result.value).toEqual([{ type: "asin", value: "B01ABC1234" }]);
  });

  it("overwrites current with incoming identifiers", () => {
    const current = [
      { type: "isbn_13", value: "9780316769488" },
      { type: "asin", value: "B01ABC1234" },
    ];
    const incoming = [
      { type: "asin", value: "B02DEF5678" },
      { type: "goodreads", value: "12345" },
    ];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("changed");
    expect(result.value).toEqual([
      { type: "asin", value: "B02DEF5678" },
      { type: "goodreads", value: "12345" },
    ]);
  });

  it("dedupes intra-incoming duplicates with last-wins before merging", () => {
    const current: { type: string; value: string }[] = [];
    const incoming = [
      { type: "asin", value: "OLD" },
      { type: "asin", value: "NEW" },
    ];
    const result = resolveIdentifiers(current, incoming);
    expect(result.value).toEqual([{ type: "asin", value: "NEW" }]);
  });

  it("returns changed when incoming is a subset of current", () => {
    const current = [
      { type: "isbn_13", value: "9780316769488" },
      { type: "asin", value: "B01ABC1234" },
    ];
    const incoming = [{ type: "isbn_13", value: "9780316769488" }];
    const result = resolveIdentifiers(current, incoming);
    expect(result.status).toBe("changed");
    expect(result.value).toEqual([{ type: "isbn_13", value: "9780316769488" }]);
  });
});

describe("pickInitialFile", () => {
  function file(id: number, opts: Partial<File> = {}): File {
    return {
      id,
      file_role: "main",
      ...opts,
    } as File;
  }

  it("returns undefined when there are no main files", () => {
    const result = pickInitialFile({
      files: [file(1, { file_role: "supplement" })],
      primary_file_id: undefined,
    } as unknown as Book);
    expect(result).toBeUndefined();
  });

  it("prefers a non-reviewed file when some are reviewed", () => {
    const files = [
      file(1, { reviewed: true }),
      file(2, { reviewed: false }),
      file(3, { reviewed: true }),
    ];
    expect(
      pickInitialFile({ files, primary_file_id: 1 } as unknown as Book)?.id,
    ).toBe(2);
  });

  it("treats reviewed=undefined as non-reviewed", () => {
    const files = [file(1, { reviewed: true }), file(2)];
    expect(
      pickInitialFile({ files, primary_file_id: 1 } as unknown as Book)?.id,
    ).toBe(2);
  });

  it("prefers primary when all reviewed equal", () => {
    const files = [file(1), file(2), file(3)];
    expect(
      pickInitialFile({ files, primary_file_id: 2 } as unknown as Book)?.id,
    ).toBe(2);
  });

  it("falls back to first when primary not set", () => {
    const files = [file(10, { reviewed: true }), file(20, { reviewed: true })];
    expect(
      pickInitialFile({ files, primary_file_id: undefined } as unknown as Book)
        ?.id,
    ).toBe(10);
  });

  it("falls back to first when primary id does not match any main file", () => {
    const files = [file(1), file(2)];
    expect(
      pickInitialFile({ files, primary_file_id: 99 } as unknown as Book)?.id,
    ).toBe(1);
  });
});
