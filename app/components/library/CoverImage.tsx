import {
  useLayoutEffect,
  useRef,
  useState,
  type ComponentPropsWithoutRef,
} from "react";

import type { ThumbnailAspect, ThumbnailSize } from "@/types";
import { COVER_THUMBNAIL_SIZES, coverThumbnailUrl } from "@/utils/coverUrl";

type CoverImageProps = Omit<
  ComponentPropsWithoutRef<"img">,
  "src" | "srcSet" | "sizes"
> & {
  src: string;
};

/** Requests one cached thumbnail sized for the image's box and screen density. */
const CoverImage = ({
  src,
  loading = "lazy",
  decoding = "async",
  ...props
}: CoverImageProps) => {
  const ref = useRef<HTMLImageElement>(null);
  const [selection, setSelection] = useState<{
    size: ThumbnailSize | 0;
    aspect?: ThumbnailAspect;
  } | null>(null);

  useLayoutEffect(() => {
    const img = ref.current;
    if (!img) return;
    let densityQuery: MediaQueryList | undefined;
    const update = () => {
      const { width, height } = img.getBoundingClientRect();
      if (width > 0 && height > 0) {
        const pixels = Math.ceil(
          Math.max(width, height) * (window.devicePixelRatio || 1),
        );
        // Very large displays use the original rather than upscaling a thumbnail.
        const size =
          COVER_THUMBNAIL_SIZES.find((candidate) => candidate >= pixels) ?? 0;
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

  return (
    <img
      {...props}
      decoding={decoding}
      loading={loading}
      ref={ref}
      src={
        selection === null
          ? undefined
          : selection.size === 0
            ? src
            : coverThumbnailUrl(src, selection.size, selection.aspect)
      }
    />
  );
};

export default CoverImage;
