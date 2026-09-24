import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { readDemoSettings, writeDemoSettings } from "./demoSettings";

const KEY = "shisho-demo-test";
const defaults = { gallery_size: "m", fit_mode: "width" };

describe("demoSettings", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("merges stored values over the server defaults", () => {
    localStorage.setItem(KEY, JSON.stringify({ gallery_size: "xl" }));

    expect(readDemoSettings(KEY, defaults)).toEqual({
      gallery_size: "xl",
      fit_mode: "width",
    });
  });

  it("falls back to the server defaults when stored JSON is corrupt", () => {
    localStorage.setItem(KEY, "{not json");

    expect(readDemoSettings(KEY, defaults)).toEqual(defaults);
  });

  it("falls back to the server defaults when storage is unavailable", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("denied", "SecurityError");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("denied", "SecurityError");
    });

    expect(readDemoSettings(KEY, defaults)).toEqual(defaults);
  });

  it("does not throw when a write exceeds the storage quota", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("full", "QuotaExceededError");
    });

    expect(() => writeDemoSettings(KEY, defaults)).not.toThrow();
  });
});
