import { describe, expect, it } from "vitest";

import {
  computeIdentifyEmptyState,
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
  it("re-exports and works (smoke test)", () => {
    const result = resolveIdentifiers(
      [{ type: "goodreads", value: "1" }],
      [{ type: "goodreads", value: "1" }],
    );
    expect(result.status).toBe("unchanged");
  });
});
