import { describe, expect, it } from "vitest";

import type { PluginCapabilities } from "@/hooks/queries/plugins";

import { summarizeCapabilities } from "./capabilitySummary";

const http = (domains: string[]): PluginCapabilities => ({
  identifierTypes: [],
  httpAccess: { description: "", domains },
});

describe("summarizeCapabilities", () => {
  it("names up to two sites and counts the rest", () => {
    expect(summarizeCapabilities(http(["*.openlibrary.org"]))).toBe(
      "Connects to openlibrary.org",
    );
    expect(
      summarizeCapabilities(http(["*.openlibrary.org", "*.archive.org"])),
    ).toBe("Connects to openlibrary.org and archive.org");
    expect(
      summarizeCapabilities(
        http(["api.audible.com", "api.audible.de", "api.audnex.us", "x.com"]),
      ),
    ).toBe("Connects to api.audible.com, api.audible.de, and 2 more sites");
  });

  it("adds file, FFmpeg, and shell access", () => {
    expect(
      summarizeCapabilities({
        identifierTypes: [],
        fileAccess: { description: "", level: "readwrite" },
        ffmpegAccess: { description: "" },
        shellAccess: { description: "", commands: ["ls"] },
      }),
    ).toBe(
      "Reads and writes files outside its folder · Runs FFmpeg · Runs shell commands",
    );
  });

  it("says when a plugin needs nothing", () => {
    expect(summarizeCapabilities({ identifierTypes: [] })).toBe(
      "Needs no special access",
    );
    expect(summarizeCapabilities(undefined)).toBe("Needs no special access");
  });
});
