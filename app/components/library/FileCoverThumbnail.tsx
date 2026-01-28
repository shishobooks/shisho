import { useState } from "react";

import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { cn } from "@/libraries/utils";
import type { File } from "@/types";

interface FileCoverThumbnailProps {
  file: File;
  className?: string;
  onClick?: () => void;
  cacheBuster?: number;
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
  cacheBuster,
}: FileCoverThumbnailProps) {
  const [imageLoaded, setImageLoaded] = useState(false);
  const [imageError, setImageError] = useState(false);

  const isAudiobook = file.file_type === "m4b";
  const aspectClass = isAudiobook ? "aspect-square" : "aspect-[2/3]";
  const placeholderVariant = isAudiobook ? "audiobook" : "book";

  const hasCover = file.cover_image_filename && !imageError;
  const coverUrl = cacheBuster
    ? `/api/books/files/${file.id}/cover?t=${cacheBuster}`
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
      {/* Placeholder shown until image loads or on error */}
      {(!imageLoaded || !hasCover) && (
        <CoverPlaceholder
          className="absolute inset-0"
          variant={placeholderVariant}
        />
      )}

      {/* Image hidden until loaded */}
      {hasCover && (
        <img
          alt=""
          className={cn(
            "absolute inset-0 w-full h-full object-cover",
            !imageLoaded && "opacity-0",
          )}
          onError={() => setImageError(true)}
          onLoad={() => setImageLoaded(true)}
          src={coverUrl}
        />
      )}
    </div>
  );
}

export default FileCoverThumbnail;
