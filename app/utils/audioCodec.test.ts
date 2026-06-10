import { describe, expect, it } from "vitest";

import {
  resolveCodecSupport,
  resolveRuntimeFailureMessage,
  shouldIgnoreMediaError,
} from "./audioCodec";

// Representative user agent strings for the browsers we care about. xHE-AAC
// plays in WebKit (Safari and every iOS browser, which all use WebKit), but a
// plain progressive stream fails in Firefox and desktop/Android Chromium.
const UA = {
  chromeMac:
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
  edgeWindows:
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36 Edg/125.0.0.0",
  firefoxMac:
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0",
  safariMac:
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15",
  safariIPhone:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
  chromeIPhone:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/125.0.0.0 Mobile/15E148 Safari/604.1",
  chromeAndroid:
    "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Mobile Safari/537.36",
};

describe("resolveCodecSupport", () => {
  it("treats AAC-LC as playable in every browser with no message", () => {
    for (const ua of Object.values(UA)) {
      const support = resolveCodecSupport("AAC-LC", ua);
      expect(support.playable).toBe(true);
      expect(support.message).toBeNull();
    }
  });

  it("treats HE-AAC as playable in every browser with no message", () => {
    for (const ua of Object.values(UA)) {
      const support = resolveCodecSupport("HE-AAC", ua);
      expect(support.playable).toBe(true);
      expect(support.message).toBeNull();
    }
  });

  it("treats a missing codec as playable (nothing to detect against)", () => {
    expect(resolveCodecSupport(undefined, UA.chromeMac).playable).toBe(true);
    expect(resolveCodecSupport(null, UA.firefoxMac).playable).toBe(true);
    expect(resolveCodecSupport("", UA.chromeMac).playable).toBe(true);
  });

  it("flags xHE-AAC as unplayable in desktop Chrome with a message recommending Safari", () => {
    const support = resolveCodecSupport("xHE-AAC", UA.chromeMac);
    expect(support.playable).toBe(false);
    expect(support.message).toMatch(/xHE-AAC/);
    expect(support.message).toMatch(/Safari/);
  });

  it("flags xHE-AAC as unplayable in Firefox", () => {
    const support = resolveCodecSupport("xHE-AAC", UA.firefoxMac);
    expect(support.playable).toBe(false);
    expect(support.message).toMatch(/Safari/);
  });

  it("flags xHE-AAC as unplayable in Edge and Android Chrome (Chromium without HLS)", () => {
    expect(resolveCodecSupport("xHE-AAC", UA.edgeWindows).playable).toBe(false);
    expect(resolveCodecSupport("xHE-AAC", UA.chromeAndroid).playable).toBe(
      false,
    );
  });

  it("treats xHE-AAC as playable in desktop Safari", () => {
    const support = resolveCodecSupport("xHE-AAC", UA.safariMac);
    expect(support.playable).toBe(true);
    expect(support.message).toBeNull();
  });

  it("treats xHE-AAC as playable in any iOS browser (all use WebKit)", () => {
    expect(resolveCodecSupport("xHE-AAC", UA.safariIPhone).playable).toBe(true);
    expect(resolveCodecSupport("xHE-AAC", UA.chromeIPhone).playable).toBe(true);
  });

  it("matches the xHE-AAC codec case-insensitively", () => {
    expect(resolveCodecSupport("xhe-aac", UA.chromeMac).playable).toBe(false);
    expect(resolveCodecSupport("XHE-AAC", UA.firefoxMac).playable).toBe(false);
  });
});

describe("resolveRuntimeFailureMessage", () => {
  it("names the file's codec when it is known and recommends Safari", () => {
    const message = resolveRuntimeFailureMessage("xHE-AAC");
    expect(message).toMatch(/xHE-AAC/);
    expect(message).toMatch(/Safari/);
  });

  it("falls back to a generic codec message when the codec is unknown", () => {
    for (const codec of [undefined, null, ""]) {
      const message = resolveRuntimeFailureMessage(codec);
      expect(message).toMatch(/codec/i);
      expect(message).toMatch(/Safari/);
    }
  });
});

describe("shouldIgnoreMediaError", () => {
  // MediaError codes are fixed by the HTML spec: 1 = MEDIA_ERR_ABORTED,
  // 2 = MEDIA_ERR_NETWORK, 3 = MEDIA_ERR_DECODE, 4 = MEDIA_ERR_SRC_NOT_SUPPORTED.
  it("ignores MEDIA_ERR_ABORTED (user-initiated, not a codec problem)", () => {
    expect(shouldIgnoreMediaError(1)).toBe(true);
  });

  it("does not ignore decode or src-not-supported errors", () => {
    expect(shouldIgnoreMediaError(3)).toBe(false);
    expect(shouldIgnoreMediaError(4)).toBe(false);
    expect(shouldIgnoreMediaError(2)).toBe(false);
  });

  it("does not ignore an error event with no MediaError attached", () => {
    expect(shouldIgnoreMediaError(undefined)).toBe(false);
    expect(shouldIgnoreMediaError(null)).toBe(false);
  });
});
