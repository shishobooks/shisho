import {
  deriveCapabilityLabels,
  resolveInstalledPluginCapabilities,
} from "./pluginCapabilities";
import { describe, expect, it } from "vitest";

import type {
  AvailablePlugin,
  PluginCapabilities,
  PluginVersion,
} from "@/hooks/queries/plugins";
import { PluginStatusActive, type Plugin } from "@/types/generated/models";

describe("deriveCapabilityLabels", () => {
  it("returns [] for null caps", () => {
    expect(deriveCapabilityLabels(null)).toEqual([]);
  });

  it("returns [] for undefined caps", () => {
    expect(deriveCapabilityLabels(undefined)).toEqual([]);
  });

  it("returns [] for empty caps object", () => {
    expect(deriveCapabilityLabels({})).toEqual([]);
  });

  it("returns all four labels when all display-worthy capabilities are set", () => {
    const caps: PluginCapabilities = {
      metadataEnricher: {},
      inputConverter: {},
      fileParser: {},
      outputGenerator: {},
    };
    expect(deriveCapabilityLabels(caps)).toEqual([
      "Metadata enricher",
      "Input converter",
      "File parser",
      "Output generator",
    ]);
  });

  it("returns a single label for a single capability", () => {
    expect(deriveCapabilityLabels({ metadataEnricher: {} })).toEqual([
      "Metadata enricher",
    ]);
    expect(deriveCapabilityLabels({ fileParser: {} })).toEqual(["File parser"]);
  });

  it("ignores capabilities that don't produce display labels (httpAccess, shellAccess, etc.)", () => {
    // Only metadataEnricher/inputConverter/fileParser/outputGenerator should
    // produce labels; access-style capabilities should not.
    const caps: PluginCapabilities = {
      httpAccess: { domains: ["example.com"] },
      shellAccess: { commands: ["ls"] },
      fileAccess: { level: "read" },
      ffmpegAccess: {},
    };
    expect(deriveCapabilityLabels(caps)).toEqual([]);
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
    const caps: PluginCapabilities = { metadataEnricher: {} };
    const available = makeAvailable([
      makeVersion("2.0.0", { fileParser: {} }),
      makeVersion("1.0.0", caps),
    ]);
    expect(
      resolveInstalledPluginCapabilities(makePlugin("1.0.0"), available),
    ).toBe(caps);
  });

  it("falls back to versions[0] capabilities when installed version is not in the list", () => {
    const latestCaps: PluginCapabilities = { outputGenerator: {} };
    const available = makeAvailable([
      makeVersion("2.0.0", latestCaps),
      makeVersion("1.0.0", { fileParser: {} }),
    ]);
    expect(
      resolveInstalledPluginCapabilities(makePlugin("9.9.9"), available),
    ).toBe(latestCaps);
  });

  it("falls back to versions[0] when the matched version has no capabilities field", () => {
    const latestCaps: PluginCapabilities = { inputConverter: {} };
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
