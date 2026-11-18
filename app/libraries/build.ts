// The server names the frontend build it embeds on every response, and the
// page names the build it loaded in a meta tag (both written by the
// shisho-build-id plugin in vite.config.ts). A tab left open across an upgrade
// sees them differ, and moves onto the new build with a full page load: its
// chunks are gone from the server, and the API it calls may have changed with
// the frontend that ships beside it.

export const BUILD_HEADER = "X-Shisho-Build";

/** The build this page loaded, or null under the Vite dev server. */
const runningBuild = (): string | null =>
  document.querySelector<HTMLMetaElement>('meta[name="shisho-build"]')
    ?.content || null;

let serverBuild: string | null = null;
const listeners = new Set<() => void>();

/** Records the build a response names, if it names one. */
export const noteServerBuild = (response: Response) => {
  const id = response.headers.get(BUILD_HEADER);
  if (!id || id === serverBuild) {
    return;
  }
  serverBuild = id;
  listeners.forEach((listener) => listener());
};

/** Whether the server has a different build than the one this page runs. */
export const isNewBuildOut = (): boolean => {
  const running = runningBuild();
  return running !== null && serverBuild !== null && serverBuild !== running;
};

export const subscribeToServerBuild = (listener: () => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

/**
 * Asks the server for its build without waiting for the app's next request,
 * for moments an upgrade is likely to have happened unseen: the tab coming
 * back into view, or the event stream reconnecting after a restart.
 */
export const checkServerBuild = async () => {
  if (runningBuild() === null) {
    return;
  }
  try {
    // Public and cheap, and under /api, which every proxy must forward
    // (/health is optional for proxies).
    noteServerBuild(await fetch("/api/auth/status", { cache: "no-store" }));
  } catch {
    // Offline or restarting; the next request checks again.
  }
};

const RELOADED_FOR_KEY = "shisho-new-build-reload";

/**
 * Records the server build an automatic reload is for. A page that still runs
 * an older build after that reload is being served a stale index.html (a
 * proxy caching it, say), and reloading again would loop.
 */
export const markReloadingForServerBuild = () => {
  try {
    window.sessionStorage.setItem(RELOADED_FOR_KEY, serverBuild ?? "");
  } catch {
    // Without storage there is no loop guard, but the reload still helps.
  }
};

/** Whether this tab already reloaded for the build the server has now. */
export const alreadyReloadedForServerBuild = (): boolean => {
  try {
    return (
      serverBuild !== null &&
      window.sessionStorage.getItem(RELOADED_FOR_KEY) === serverBuild
    );
  } catch {
    return false;
  }
};

/** A full page load, which fetches the server's current build. */
export const reloadPage = () => window.location.reload();
