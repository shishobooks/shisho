import { useEffect, useState } from "react";

import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { cn } from "@/libraries/utils";
import type { File } from "@/types";

interface FileCoverThumbnailProps {
  file: File;
  className?: string;
  onClick?: () => void;
  /**
   * Forces the <img> to remount (and thus re-request) when this value changes.
   * Typically a React Query `dataUpdatedAt` from a mutation-aware query.
   */
  cacheKey?: number;
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
        "relative overflow-hidden rounded border border-border shrink-0 cursor-pointer",
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
          key={`${file.id}-${cacheKey ?? 0}`}
          onError={() => setImageError(true)}
          onLoad={() => setImageLoaded(true)}
          src={coverUrl}
        />
      )}
    </div>
  );
}

export default FileCoverThumbnail;
