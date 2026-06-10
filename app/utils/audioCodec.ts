// Pure, framework-free codec compatibility resolution for the audiobook
// player. Nothing here touches React, the DOM, or the audio element. Given the
// File's stored audiobook codec and the browser's user agent string, it
// decides whether a plain progressive stream of that codec can play and what
// message to show the listener when it cannot. This keeps the logic
// unit-testable in complete isolation; the audio-element wiring (error and
// stalled events, the seek timeout guard) stays thin in the React shell.
//
// The one codec we detect up-front is xHE-AAC (USAC). It plays in WebKit
// (Safari, and every iOS browser since they all use WebKit), but Firefox does
// not support it at all and Chromium only supports it through HLS, so the
// player's plain progressive stream fails there. AAC-LC and HE-AAC, which
// cover the overwhelming majority of M4B files, play everywhere. Anything
// else (or a misdetected file) is caught at runtime by the audio element's
// error/stalled events and the seek timeout guard, which surface the same
// hedged recommendation.

/**
 * How long (ms) to wait for an in-flight seek to complete before giving up.
 * A seek into an undecodable stream never finishes (the element reports
 * `seeking: true` forever), so the guard pauses playback and surfaces the
 * codec message instead of letting the player hang.
 */
export const SEEK_TIMEOUT_MS = 5000;

/** The outcome of checking the File's stored codec against the browser. */
export interface CodecSupport {
  /** Whether a plain progressive stream of this codec is expected to play. */
  playable: boolean;
  /** User-facing explanation when not playable, otherwise null. */
  message: string | null;
}

/** Matches the stored codec strings that denote xHE-AAC (USAC, AOT 42). */
const XHE_AAC_PATTERN = /xhe-aac|usac/i;

/**
 * Whether this browser can play a plain progressive xHE-AAC stream. True for
 * WebKit: desktop/mobile Safari, plus any iOS browser (Chrome, Firefox, etc.
 * on iOS are WebKit underneath). False for Firefox and for Chromium, which
 * only plays xHE-AAC through HLS.
 */
export function canBrowserPlayXheAac(userAgent: string): boolean {
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
 * the current browser. Only xHE-AAC is flagged; unknown or absent codecs are
 * assumed playable and left to the runtime backstop.
 */
export function resolveCodecSupport(
  codec: string | null | undefined,
  userAgent: string,
): CodecSupport {
  if (
    codec &&
    XHE_AAC_PATTERN.test(codec) &&
    !canBrowserPlayXheAac(userAgent)
  ) {
    return {
      playable: false,
      message:
        "This audiobook is encoded with xHE-AAC, which this browser cannot play. Try Safari, which supports xHE-AAC playback.",
    };
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
