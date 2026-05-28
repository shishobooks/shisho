import { useEffect, useState } from "react";

import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { cn } from "@/libraries/utils";
import type { File } from "@/types";

interface FileCoverThumbnailProps {
  file: File;
  className?: string;
  onClick?: () => void;
  cacheKey?: string;
  /**
   * Whether to apply interactive styles (cursor-pointer, hover:scale, hover:shadow).
   * Defaults to true. Pass false for non-interactive contexts like file list rows.
   */
  interactive?: boolean;
}

/**
 * Small cover thumbnail for a file.
 * Shows the file's cover image or a placeholder based on file type.
 * Aspect ratio: 2:3 for EPUB/CBZ, square for M4B.
 */
function FileCoverThumbnail({
  file,
  className,
  onClick,
  cacheKey,
  interactive = true,
}: FileCoverThumbnailProps) {
  const [imageLoaded, setImageLoaded] = useState(false);
  const [imageError, setImageError] = useState(false);

  // Reset both flags when the URL would change — either the cover filename
  // was replaced or the cacheKey bumped from a data refetch. Without resetting
  // imageError, a transient load failure latches the placeholder forever
  // because the <img> is conditionally unrendered and the `key` bump can't
  // remount it. Without resetting imageLoaded, the remounted <img> renders at
  // full opacity during the fresh fetch, skipping the fade-in.
  useEffect(() => {
    setImageError(false);
    setImageLoaded(false);
  }, [cacheKey, file.cover_image_filename]);

  const isAudiobook = file.file_type === "m4b";
  const aspectClass = isAudiobook ? "aspect-square" : "aspect-[2/3]";
  const placeholderVariant = isAudiobook ? "audiobook" : "book";

  const hasCover = file.cover_image_filename && !imageError;
  const coverUrl = cacheKey
    ? `/api/books/files/${file.id}/cover?v=${cacheKey}`
    : `/api/books/files/${file.id}/cover`;

  return (
    <div
      className={cn(
        "relative overflow-hidden rounded border border-border shrink-0",
        interactive && "cursor-pointer",
        interactive &&
          "transition-all duration-200 hover:scale-105 hover:shadow-md",
        aspectClass,
        className,
      )}
      onClick={onClick}
    >
      {(!imageLoaded || !hasCover) && (
        <CoverPlaceholder
          className="absolute inset-0"
          variant={placeholderVariant}
        />
      )}

      {hasCover && (
        <img
          alt=""
          className={cn(
            "absolute inset-0 w-full h-full object-cover",
            !imageLoaded && "opacity-0",
          )}
          key={`${file.id}-${cacheKey ?? ""}`}
          onError={() => setImageError(true)}
          onLoad={() => setImageLoaded(true)}
          src={coverUrl}
        />
      )}
    </div>
  );
}

export default FileCoverThumbnail;
