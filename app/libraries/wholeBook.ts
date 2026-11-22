/**
 * Whether this device can hold a whole book while the reader has it open.
 * The reader reads a book by part either way; one that can also downloads
 * it whole in the background, so jumping around a cookbook stops fetching
 * every photo in each chapter. A phone can't: iOS killed a tab holding a
 * few hundred MB of a cookbook, and a whole book is a lot of cellular data.
 */
export const canKeepWholeBook = (): boolean => {
  // Every browser on an iPhone or iPad is WebKit, which reports no memory.
  // Only iOS WebKit supports -webkit-touch-callout, even on an iPad asking
  // for desktop sites; a Mac doesn't, touch screen or not.
  if (globalThis.CSS?.supports?.("-webkit-touch-callout", "none")) {
    return false;
  }
  const nav = navigator as Navigator & {
    userAgentData?: { mobile?: boolean };
    deviceMemory?: number;
  };
  // Android phones and tablets; userAgentData is Chromium's.
  if (nav.userAgentData?.mobile || /Android/i.test(nav.userAgent)) {
    return false;
  }
  // Chromium reports memory in GB, rounded down and capped at 8.
  if (nav.deviceMemory !== undefined && nav.deviceMemory < 4) return false;
  return true;
};
