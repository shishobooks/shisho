import { describe, expect, it } from "vitest";

import {
  PluginStatusActive,
  PluginStatusDisabled,
  PluginStatusMalfunctioned,
  PluginStatusNotSupported,
  type Plugin,
} from "@/types/generated/models";

import { pluginAlertContent } from "./pluginAlertContent";

const base: Plugin = {
  auto_update: false,
  id: "p",
  installed_at: "2026-01-01T00:00:00Z",
  name: "Plugin",
  scope: "shisho",
  status: PluginStatusActive,
  version: "1.0.0",
};

describe("pluginAlertContent", () => {
  it("returns null when no plugin is installed", () => {
    expect(pluginAlertContent(undefined)).toBeNull();
  });

  it("returns null for Active plugins", () => {
    expect(pluginAlertContent(base)).toBeNull();
  });

  it("returns null for Disabled plugins with no load_error", () => {
    expect(
      pluginAlertContent({ ...base, status: PluginStatusDisabled }),
    ).toBeNull();
  });

  it("returns the failed-to-load title for Malfunctioned", () => {
    expect(
      pluginAlertContent({
        ...base,
        load_error: "boom",
        status: PluginStatusMalfunctioned,
      }),
    ).toEqual({ body: "boom", title: "Plugin failed to load" });
  });

  it("returns the incompatible title for NotSupported, preserving load_error when present", () => {
    expect(
      pluginAlertContent({
        ...base,
        status: PluginStatusNotSupported,
      }),
    ).toEqual({
      body: undefined,
      title: "Plugin is not compatible with this Shisho version",
    });

    expect(
      pluginAlertContent({
        ...base,
        load_error: "requires Shisho 99.0.0",
        status: PluginStatusNotSupported,
      }),
    ).toEqual({
      body: "requires Shisho 99.0.0",
      title: "Plugin is not compatible with this Shisho version",
    });
  });

  it("falls back to failed-to-load when load_error is set without a bad status", () => {
    expect(
      pluginAlertContent({
        ...base,
        load_error: "stale error text",
      }),
    ).toEqual({ body: "stale error text", title: "Plugin failed to load" });
  });
});
