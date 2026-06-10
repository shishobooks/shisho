import { describe, expect, it } from "vitest";

import type {
  AvailablePlugin,
  PluginCapabilities,
  PluginVersion,
} from "@/hooks/queries/plugins";
import { PluginStatusActive, type Plugin } from "@/types/generated/models";

import {
  deriveCapabilityLabels,
  resolveInstalledPluginCapabilities,
} from "./pluginCapabilities";

// The generated Capabilities/cap types mirror Go's parsed-manifest structs,
// so every field is present on the wire. These helpers fill the boilerplate
// so tests can express just the capability mix they care about.
const caps = (
  partial: Partial<PluginCapabilities> = {},
): PluginCapabilities => ({
  identifierTypes: [],
  ...partial,
});
const enricherCap = { description: "", fields: [], fileTypes: [] };
const converterCap = {
  description: "",
  mimeTypes: [],
  sourceTypes: [],
  targetType: "",
};
const parserCap = { description: "", mimeTypes: [], types: [] };
const generatorCap = { description: "", id: "", name: "", sourceTypes: [] };

describe("deriveCapabilityLabels", () => {
  it("returns [] for null caps", () => {
    expect(deriveCapabilityLabels(null)).toEqual([]);
  });

  it("returns [] for undefined caps", () => {
    expect(deriveCapabilityLabels(undefined)).toEqual([]);
  });

  it("returns [] for empty caps object", () => {
    expect(deriveCapabilityLabels(caps())).toEqual([]);
  });

  it("returns all four labels when all display-worthy capabilities are set", () => {
    const all = caps({
      metadataEnricher: enricherCap,
      inputConverter: converterCap,
      fileParser: parserCap,
      outputGenerator: generatorCap,
    });
    expect(deriveCapabilityLabels(all)).toEqual([
      "Metadata enricher",
      "Input converter",
      "File parser",
      "Output generator",
    ]);
  });

  it("returns a single label for a single capability", () => {
    expect(
      deriveCapabilityLabels(caps({ metadataEnricher: enricherCap })),
    ).toEqual(["Metadata enricher"]);
    expect(deriveCapabilityLabels(caps({ fileParser: parserCap }))).toEqual([
      "File parser",
    ]);
  });

  it("ignores capabilities that don't produce display labels (httpAccess, shellAccess, etc.)", () => {
    // Only metadataEnricher/inputConverter/fileParser/outputGenerator should
    // produce labels; access-style capabilities should not.
    const accessOnly = caps({
      httpAccess: { description: "", domains: ["example.com"] },
      shellAccess: { commands: ["ls"], description: "" },
      fileAccess: { description: "", level: "read" },
      ffmpegAccess: { description: "" },
    });
    expect(deriveCapabilityLabels(accessOnly)).toEqual([]);
  });
});

describe("resolveInstalledPluginCapabilities", () => {
  const makePlugin = (version: string): Plugin => ({
    auto_update: false,
    id: "my-plugin",
    installed_at: "2024-01-01T00:00:00Z",
    name: "My Plugin",
    scope: "shisho",
    status: PluginStatusActive,
    version,
  });

  const makeVersion = (
    version: string,
    capabilities?: PluginCapabilities,
  ): PluginVersion => ({
    version,
    minShishoVersion: "0.0.0",
    compatible: true,
    changelog: "",
    downloadUrl: "",
    sha256: "",
    manifestVersion: 1,
    releaseDate: "2024-01-01T00:00:00Z",
    capabilities,
  });

  const makeAvailable = (versions: PluginVersion[]): AvailablePlugin => ({
    scope: "shisho",
    id: "my-plugin",
    name: "My Plugin",
    overview: "",
    description: "",
    homepage: "",
    imageUrl: "",
    is_official: false,
    versions,
    compatible: true,
  });

  it("returns null when available is undefined", () => {
    expect(
      resolveInstalledPluginCapabilities(makePlugin("1.0.0"), undefined),
    ).toBeNull();
  });

  it("returns the capabilities for the version matching the installed plugin", () => {
    const matched = caps({ metadataEnricher: enricherCap });
    const available = makeAvailable([
      makeVersion("2.0.0", caps({ fileParser: parserCap })),
      makeVersion("1.0.0", matched),
    ]);
    expect(
      resolveInstalledPluginCapabilities(makePlugin("1.0.0"), available),
    ).toBe(matched);
  });

  it("falls back to versions[0] capabilities when installed version is not in the list", () => {
    const latestCaps = caps({ outputGenerator: generatorCap });
    const available = makeAvailable([
      makeVersion("2.0.0", latestCaps),
      makeVersion("1.0.0", caps({ fileParser: parserCap })),
    ]);
    expect(
      resolveInstalledPluginCapabilities(makePlugin("9.9.9"), available),
    ).toBe(latestCaps);
  });

  it("falls back to versions[0] when the matched version has no capabilities field", () => {
    const latestCaps = caps({ inputConverter: converterCap });
    const available = makeAvailable([
      makeVersion("2.0.0", latestCaps),
      makeVersion("1.0.0"), // no capabilities
    ]);
    expect(
      resolveInstalledPluginCapabilities(makePlugin("1.0.0"), available),
    ).toBe(latestCaps);
  });

  it("returns null when available.versions is empty", () => {
    expect(
      resolveInstalledPluginCapabilities(
        makePlugin("1.0.0"),
        makeAvailable([]),
      ),
    ).toBeNull();
  });

  it("returns null when no version has capabilities (matched or fallback)", () => {
    const available = makeAvailable([makeVersion("1.0.0")]);
    expect(
      resolveInstalledPluginCapabilities(makePlugin("1.0.0"), available),
    ).toBeNull();
  });
});
