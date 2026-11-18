import { reloadPage } from "@/libraries/build";

const LAST_RELOAD_KEY = "shisho-stale-chunk-reload";

// A second failure this soon after a reload means the new build is missing the
// chunk too, so reloading again would loop.
const RELOAD_COOLDOWN_MS = 30_000;

const readLastReload = (): number => {
  try {
    return Number(window.sessionStorage.getItem(LAST_RELOAD_KEY)) || 0;
  } catch {
    return 0;
  }
};

const writeLastReload = (time: number) => {
  try {
    window.sessionStorage.setItem(LAST_RELOAD_KEY, String(time));
  } catch {
    // Without storage there is no loop guard, but a stale tab still recovers.
  }
};

/**
 * Reloads the page when a lazily loaded chunk fails to load. Chunk names are
 * content hashed, so a tab opened before a server upgrade asks for files the
 * new build no longer has; reloading picks up the current build. Vite fires
 * `vite:preloadError` for every dynamic import it compiles, including the ones
 * inside the vendored foliate reader.
 *
 * Returns a function that removes the listener.
 */
export const installStaleChunkReload = (
  reload: () => void = reloadPage,
): (() => void) => {
  const onPreloadError = () => {
    if (!window.navigator.onLine) {
      return;
    }
    const now = Date.now();
    if (now - readLastReload() < RELOAD_COOLDOWN_MS) {
      return;
    }
    writeLastReload(now);
    reload();
  };
  window.addEventListener("vite:preloadError", onPreloadError);
  return () => window.removeEventListener("vite:preloadError", onPreloadError);
};
