import { useEffect } from "react";

// Loads images into the browser's cache one at a time, in the order given, so
// showing one later doesn't wait on the network. One at a time keeps a burst of
// PDF pages from queuing renders on the server, which does them singly. A new
// list drops whatever hadn't started.
export const usePreloadImages = (urls: string[]) => {
  // The list's contents, since callers build a new array each render.
  const key = urls.join("\n");
  useEffect(() => {
    if (!key) return;
    const pending = key.split("\n");
    let cancelled = false;
    const loadNext = () => {
      const src = pending.shift();
      if (cancelled || !src) return;
      const image = new Image();
      image.onload = loadNext;
      image.onerror = loadNext;
      image.src = src;
    };
    loadNext();
    return () => {
      cancelled = true;
    };
  }, [key]);
};
