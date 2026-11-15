// Pure, framework-free codec compatibility resolution for the audiobook
// player. Nothing here touches React, the DOM, or the audio element. Given the
// File's stored audiobook codec and the browser's user agent string, it
// decides whether a plain progressive stream of that codec can play and what
// message to show the listener when it cannot. This keeps the logic
// unit-testable in complete isolation; the audio-element wiring (error and
// stalled events, the seek timeout guard) stays thin in the React shell.
//
// The codecs we detect up-front are the ones only WebKit plays (Safari, and
// every iOS browser since they all use WebKit):
//   - xHE-AAC (USAC): Firefox does not support it at all and Chromium only
//     supports it through HLS, so the player's plain progressive stream fails.
//   - ALAC (Apple Lossless): Chrome, Chromium, and Firefox refuse it outright
//     (no canPlayType support, media error 4, and Web Audio can't decode it).
// AAC-LC and HE-AAC, which cover the overwhelming majority of M4B files, play
// everywhere. Anything else (or a misdetected file) is caught at runtime by
// the audio element's error/stalled events and the seek timeout guard, which
// surface the same hedged recommendation.
//
// Outside WebKit the browser answers canPlayType for xHE-AAC with "probably"
// and then hangs on any seek: nothing throws and canplay never fires. So any
// code driving an M4B <audio> element never calls play() before
// readyState >= 3 (wait for canplay), puts a timeout on every wait for seeked
// or canplay, and on timeout removes its listeners, resets the playing state,
// and tells the user the codec may be unsupported. Check a file's profile with
// `ffprobe -v error -show_streams <file.m4b> | grep profile`. The user-facing
// caveat is the codec note in website/docs/supported-formats.md; keep it in
// sync with player behavior.

/**
 * How long (ms) to wait for an in-flight seek to complete before giving up.
 * A seek into an undecodable stream never finishes (the element reports
 * `seeking: true` forever), so the guard pauses playback and surfaces the
 * codec message instead of letting the player hang.
 */
export const SEEK_TIMEOUT_MS = 5000;

/**
 * The same wait for the seek to the saved place as the player opens: it may
 * land hours into a file nothing has buffered yet.
 */
export const RESTORE_SEEK_TIMEOUT_MS = 15000;

/**
 * How long the player waits to autoplay after opening. Later than this, the
 * listener has moved on, and sound starting out of nowhere would startle them.
 */
export const AUTOPLAY_TIMEOUT_MS = 20000;

/** The outcome of checking the File's stored codec against the browser. */
export interface CodecSupport {
  /** Whether a plain progressive stream of this codec is expected to play. */
  playable: boolean;
  /** User-facing explanation when not playable, otherwise null. */
  message: string | null;
}

/**
 * Codecs that play only in WebKit, each matched against the File's stored
 * codec string and paired with the message shown everywhere else.
 */
const WEBKIT_ONLY_CODECS: ReadonlyArray<{ pattern: RegExp; message: string }> =
  [
    {
      // xHE-AAC (USAC, AOT 42).
      pattern: /xhe-aac|usac/i,
      message:
        "This audiobook is encoded with xHE-AAC, which this browser cannot play. Try Safari, which supports xHE-AAC playback.",
    },
    {
      // Apple Lossless. The parser stores it as "ALAC".
      pattern: /^alac$/i,
      message:
        "This audiobook is encoded with ALAC (Apple Lossless), which this browser cannot play. Try Safari, which supports ALAC playback.",
    },
  ];

/**
 * Whether this browser is WebKit, the only engine that plays the codecs in
 * WEBKIT_ONLY_CODECS as a plain progressive stream. True for desktop/mobile
 * Safari, plus any iOS browser (Chrome, Firefox, etc. on iOS are WebKit
 * underneath). False for Firefox and Chromium.
 */
function isWebKitBrowser(userAgent: string): boolean {
  // Every iOS browser is WebKit, regardless of its brand token.
  if (/iPhone|iPad|iPod/.test(userAgent)) return true;
  // Genuine Safari: a WebKit UA without another browser's brand token.
  // (iPadOS in desktop mode reports a Macintosh Safari UA and lands here.)
  return (
    /Safari\//.test(userAgent) &&
    !/Chrome|Chromium|CriOS|Edg|OPR|FxiOS|Android/i.test(userAgent)
  );
}

/**
 * Resolves whether the File's stored audiobook codec is expected to play in
 * the current browser. Only the WebKit-only codecs are flagged; unknown or
 * absent codecs are assumed playable and left to the runtime backstop.
 */
export function resolveCodecSupport(
  codec: string | null | undefined,
  userAgent: string,
): CodecSupport {
  if (!codec || isWebKitBrowser(userAgent)) {
    return { playable: true, message: null };
  }
  const webKitOnly = WEBKIT_ONLY_CODECS.find(({ pattern }) =>
    pattern.test(codec),
  );
  if (webKitOnly) {
    return { playable: false, message: webKitOnly.message };
  }
  return { playable: true, message: null };
}

/**
 * The message shown when playback fails at runtime (an audio element error, a
 * stall before any data is decodable, or a seek that never completes). Hedged
 * because runtime failures are a backstop: the codec is the most likely
 * culprit but not a certainty.
 */
export function resolveRuntimeFailureMessage(
  codec: string | null | undefined,
): string {
  const codecLabel = codec ? `codec (${codec})` : "codec";
  return `Playback failed. This audiobook's ${codecLabel} may not be supported by this browser. Try Safari, which has the widest audiobook codec support.`;
}

/** MEDIA_ERR_ABORTED: the user agent aborted the fetch at the user's request. */
const MEDIA_ERR_ABORTED = 1;

/**
 * Whether an audio element error event should be ignored rather than surfaced
 * as a playback failure. Only aborted fetches are ignored (they are
 * user-initiated, e.g. navigating away mid-load); network, decode, and
 * src-not-supported errors all indicate the stream cannot play, and an error
 * event with no MediaError attached is treated as a failure too.
 */
export function shouldIgnoreMediaError(
  errorCode: number | null | undefined,
): boolean {
  return errorCode === MEDIA_ERR_ABORTED;
}
