import { afterEach, describe, expect, it, vi } from "vitest";

import { canKeepWholeBook } from "@/libraries/wholeBook";

const MAC_CHROME =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/155.0.0.0 Safari/537.36";
const IPHONE_CHROME =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 27_2_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/158.0.8127.35 Mobile/15E148 Safari/604.1";
const ANDROID_FIREFOX =
  "Mozilla/5.0 (Android 15; Mobile; rv:140.0) Gecko/140.0 Firefox/140.0";

const device = ({
  userAgent = MAC_CHROME,
  iOS = false,
  deviceMemory,
  mobile,
  platform,
}: {
  userAgent?: string;
  iOS?: boolean;
  deviceMemory?: number;
  mobile?: boolean;
  platform?: string;
}) => {
  vi.spyOn(navigator, "userAgent", "get").mockReturnValue(userAgent);
  vi.stubGlobal("CSS", {
    supports: (property: string) => iOS && property === "-webkit-touch-callout",
  });
  Object.defineProperty(navigator, "deviceMemory", {
    value: deviceMemory,
    configurable: true,
  });
  Object.defineProperty(navigator, "userAgentData", {
    value: mobile === undefined ? undefined : { mobile, platform },
    configurable: true,
  });
};

describe("canKeepWholeBook", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    Object.defineProperty(navigator, "deviceMemory", {
      value: undefined,
      configurable: true,
    });
    Object.defineProperty(navigator, "userAgentData", {
      value: undefined,
      configurable: true,
    });
  });

  it("keeps it on a desktop", () => {
    device({ deviceMemory: 8, mobile: false });
    expect(canKeepWholeBook()).toBe(true);
  });

  it("keeps it on a desktop that doesn't report its memory", () => {
    device({});
    expect(canKeepWholeBook()).toBe(true);
  });

  // WebKit reports no memory, and iOS kills a tab holding a large download.
  it("doesn't on an iPhone or iPad, whatever the browser", () => {
    device({ userAgent: IPHONE_CHROME, iOS: true });
    expect(canKeepWholeBook()).toBe(false);
  });

  it("doesn't on an iPad asking for desktop sites", () => {
    device({
      userAgent:
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/27.0 Safari/605.1.15",
      iOS: true,
    });
    expect(canKeepWholeBook()).toBe(false);
  });

  it("doesn't on an Android phone", () => {
    device({ userAgent: ANDROID_FIREFOX });
    expect(canKeepWholeBook()).toBe(false);
  });

  it("doesn't where Chromium says the device is mobile", () => {
    device({ deviceMemory: 8, mobile: true });
    expect(canKeepWholeBook()).toBe(false);
  });

  // Chrome on an Android tablet asking for desktop sites drops "Android"
  // from its user agent and says it isn't mobile.
  it("doesn't on an Android tablet asking for desktop sites", () => {
    device({ deviceMemory: 8, mobile: false, platform: "Android" });
    expect(canKeepWholeBook()).toBe(false);
  });

  it("doesn't with little memory", () => {
    device({ deviceMemory: 2, mobile: false });
    expect(canKeepWholeBook()).toBe(false);
  });
});
