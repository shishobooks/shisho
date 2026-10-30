import { useEffect, useState } from "react";

import type { PageFlow } from "@/types";

// A page at least this many times taller than it is wide is a strip from a
// comic made for vertical scrolling (a webtoon). Printed pages sit near 1.5,
// and strips split for e-readers still run past 2.5.
export const TALL_PAGE_RATIO = 2.5;

// A page that hasn't loaded by then counts as not tall, so a slow server
// can't keep the reader on a spinner.
export const PROBE_TIMEOUT_MS = 10_000;

// The pages whose shape decides the flow: the two after the cover, which is
// often a printed-size image even in a webtoon, or the only page there is.
export const pagesToProbe = (totalPages: number): number[] => {
  if (totalPages <= 0) return [];
  if (totalPages === 1) return [0];
  return [1, 2].filter((page) => page < totalPages);
};

const isTall = (url: string) =>
  new Promise<boolean>((resolve) => {
    const img = new Image();
    const timer = setTimeout(() => resolve(false), PROBE_TIMEOUT_MS);
    img.onload = () => {
      clearTimeout(timer);
      resolve(
        img.naturalWidth > 0 &&
          img.naturalHeight / img.naturalWidth >= TALL_PAGE_RATIO,
      );
    };
    img.onerror = () => {
      clearTimeout(timer);
      resolve(false);
    };
    img.src = url;
  });

/**
 * Picks a flow for a file from the shape of the pages at `urls`: scrolled if
 * any is a tall strip, paginated otherwise. Undefined while the pages load,
 * and when `urls` is null, which probes nothing.
 */
export function useDetectedPageFlow(
  urls: string[] | null,
): PageFlow | undefined {
  // The URLs as one string, so a caller building a new array each render
  // with the same URLs doesn't restart the probe.
  const key = urls?.join("\n") ?? null;
  const [detected, setDetected] = useState<{ key: string; flow: PageFlow }>();

  useEffect(() => {
    if (key === null || key === "") return;
    let cancelled = false;
    Promise.all(key.split("\n").map(isTall)).then((tall) => {
      if (cancelled) return;
      setDetected({ key, flow: tall.some(Boolean) ? "scrolled" : "paginated" });
    });
    return () => {
      cancelled = true;
    };
  }, [key]);

  if (key === "") return "paginated";
  return detected && detected.key === key ? detected.flow : undefined;
}
