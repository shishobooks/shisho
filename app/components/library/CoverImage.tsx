import {
  useLayoutEffect,
  useRef,
  useState,
  type ComponentPropsWithoutRef,
} from "react";

import type { ThumbnailAspect, ThumbnailSize } from "@/types";
import { coverThumbnailUrl, thumbnailSizeFor } from "@/utils/coverUrl";

type CoverImageProps = Omit<
  ComponentPropsWithoutRef<"img">,
  "src" | "srcSet" | "sizes"
> & {
  /** The full image, served as is on a display larger than every tier. */
  src: string;
  /**
   * Builds the URL of the thumbnail at a size, for an image whose URL is
   * not a cover's: a CBZ or PDF page passes `useFilePageUrl` with a size.
   * Defaults to the cover thumbnail of `src`.
   */
  thumbnailUrl?: (size: ThumbnailSize, aspect?: ThumbnailAspect) => string;
};

/** Requests one cached thumbnail sized for the image's box and screen density. */
const CoverImage = ({
  src,
  thumbnailUrl,
  loading = "lazy",
  decoding = "async",
  onLoad,
  ...props
}: CoverImageProps) => {
  const ref = useRef<HTMLImageElement>(null);
  // The CSS pixels the image turned out to need on its longest side, once
  // loaded, when that is more than its box: an image cropped into the box
  // (a landscape page filling a portrait cover by its height) is drawn
  // larger than the box. Later size choices keep at least this much.
  const neededPixels = useRef(0);
  const [selection, setSelection] = useState<{
    size: ThumbnailSize | 0;
    aspect?: ThumbnailAspect;
  } | null>(null);

  useLayoutEffect(() => {
    const img = ref.current;
    if (!img) return;
    neededPixels.current = 0;
    let densityQuery: MediaQueryList | undefined;
    const update = () => {
      const { width, height } = img.getBoundingClientRect();
      if (width > 0 && height > 0) {
        // Very large displays use the original rather than upscaling a thumbnail.
        const size =
          thumbnailSizeFor(Math.max(width, height, neededPixels.current)) ?? 0;
        const ratio = width / height;
        const aspect: ThumbnailAspect | undefined =
          Math.abs(ratio - 1) < 0.03
            ? "square"
            : Math.abs(ratio - 2 / 3) < 0.03
              ? "book"
              : undefined;
        setSelection((previous) =>
          previous?.size === size && previous?.aspect === aspect
            ? previous
            : { size, aspect },
        );
      }
      densityQuery?.removeEventListener("change", update);
      densityQuery = window.matchMedia?.(
        `(resolution: ${window.devicePixelRatio || 1}dppx)`,
      );
      densityQuery?.addEventListener("change", update);
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(img);
    window.addEventListener("resize", update);
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", update);
      densityQuery?.removeEventListener("change", update);
    };
  }, [src]);

  // Once loaded, checks that the thumbnail has the pixels the image is
  // drawn at, and asks for a larger tier when it is short. Only a thumbnail
  // that came back at its full tier can grow: a smaller one is the whole
  // source, and a larger request would bring nothing more.
  const handleLoad: CoverImageProps["onLoad"] = (event) => {
    onLoad?.(event);
    const img = event.currentTarget;
    const requested = selection?.size;
    const { naturalWidth, naturalHeight } = img;
    if (!requested || !naturalWidth || !naturalHeight) return;
    if (Math.max(naturalWidth, naturalHeight) < requested) return;
    const { width, height } = img.getBoundingClientRect();
    // Cropped (cover) or stretched (fill), the image needs pixels for its
    // larger scale; fitted (contain), it is never drawn past its box.
    const fit = getComputedStyle(img).objectFit;
    const scale =
      fit === "contain" || fit === "scale-down" || fit === "none"
        ? Math.min(width / naturalWidth, height / naturalHeight)
        : Math.max(width / naturalWidth, height / naturalHeight);
    const drawn = Math.max(naturalWidth, naturalHeight) * scale;
    // A crop a few percent off the box's shape overshoots it a little; tiers
    // are twice apart, so only a real shortfall is worth another load.
    const pixels = drawn * (window.devicePixelRatio || 1);
    if (pixels <= requested * 1.1) return;
    const size = thumbnailSizeFor(drawn) ?? 0;
    neededPixels.current = drawn;
    setSelection((previous) => previous && { ...previous, size });
  };

  return (
    <img
      {...props}
      decoding={decoding}
      loading={loading}
      onLoad={handleLoad}
      ref={ref}
      src={
        selection === null
          ? undefined
          : selection.size === 0
            ? src
            : thumbnailUrl
              ? thumbnailUrl(selection.size, selection.aspect)
              : coverThumbnailUrl(src, selection.size, selection.aspect)
      }
    />
  );
};

export default CoverImage;
