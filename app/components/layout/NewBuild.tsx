import { useIsMutating } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { useEffect, useRef, useSyncExternalStore } from "react";
import { useLocation, useMatches } from "react-router-dom";

import { Button } from "@/components/ui/button";
import {
  alreadyReloadedForServerBuild,
  checkServerBuild,
  isNewBuildOut,
  markReloadingForServerBuild,
  reloadPage,
  subscribeToServerBuild,
} from "@/libraries/build";

/** What a route's `handle` in router.tsx may declare. */
export interface RouteHandle {
  // A full-screen reader, which leaves out the new build banner.
  reader?: boolean;
}

const useNewBuildIsOut = () =>
  useSyncExternalStore(subscribeToServerBuild, isNewBuildOut);

/**
 * Moves a tab onto a new build at its next page change, with a full page load
 * in place of the in-app one, so someone who has left a page loses nothing.
 * A save still in flight finishes first, since a reload would cancel it, and
 * a reload that came back on the old build is not repeated. Also
 * checks the server's build when the tab comes back into view, since a phone
 * can resume a tab that made no request across an upgrade.
 */
export const ReloadOnNewBuild = () => {
  const newBuildIsOut = useNewBuildIsOut();
  const saving = useIsMutating() > 0;
  const { pathname } = useLocation();
  // The page the tab was on when it learned of the new build. Leaving it is
  // the cue (a search or hash change stays on the page), and it still counts
  // if they come back before a save finishes.
  const pathnameWhenOut = useRef<string | null>(null);
  const leftThePage = useRef(false);

  useEffect(() => {
    if (!newBuildIsOut) {
      pathnameWhenOut.current = null;
      leftThePage.current = false;
      return;
    }
    if (pathnameWhenOut.current === null) {
      pathnameWhenOut.current = pathname;
    }
    if (pathname !== pathnameWhenOut.current) {
      leftThePage.current = true;
    }
    if (leftThePage.current && !saving && !alreadyReloadedForServerBuild()) {
      markReloadingForServerBuild();
      reloadPage();
    }
  }, [newBuildIsOut, pathname, saving]);

  useEffect(() => {
    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") {
        void checkServerBuild();
      }
    };
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () =>
      document.removeEventListener("visibilitychange", onVisibilityChange);
  }, []);

  return null;
};

/**
 * Tells someone whose tab runs an older build than the server that a reload
 * moves them onto it. The readers leave it out: leaving one reloads anyway,
 * and the banner would cover the page.
 */
export const NewBuildBanner = () => {
  const newBuildIsOut = useNewBuildIsOut();
  const inReader = useMatches().some(
    (match) => (match.handle as RouteHandle | undefined)?.reader,
  );

  if (!newBuildIsOut || inReader) {
    return null;
  }

  return (
    <div
      className="border-b border-primary/20 bg-background text-sm"
      role="status"
    >
      <div className="bg-primary/10">
        <div className="flex items-center justify-between gap-4 px-4 py-1.5 md:px-6">
          <p className="flex min-w-0 flex-1 items-center gap-2 text-xs sm:text-sm">
            <span className="hidden size-6 shrink-0 items-center justify-center rounded-full bg-primary/15 text-primary sm:flex">
              <RefreshCw aria-hidden="true" className="size-3.5" />
            </span>
            <span>
              <span className="font-semibold">Shisho was updated.</span>{" "}
              <span className="text-muted-foreground">
                Reload to get the new version.
              </span>
            </span>
          </p>
          <Button
            className="h-7 shrink-0 px-2.5"
            onClick={reloadPage}
            size="sm"
          >
            Reload
          </Button>
        </div>
      </div>
    </div>
  );
};
