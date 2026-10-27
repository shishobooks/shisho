import { useEffect, useState } from "react";

import CoverImage from "@/components/library/CoverImage";
import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import { useFileImage } from "@/hooks/useFileImage";
import { cn } from "@/libraries/utils";
import { FileRoleSupplement, type File } from "@/types";
import { fileTypeLabel } from "@/utils/format";

interface FileCoverThumbnailProps {
  file: File;
  className?: string;
  onClick?: () => void;
  /**
   * Whether to apply interactive styles (hover:scale, hover:shadow, and
   * cursor-pointer when onClick is set). Defaults to true. Pass false for
   * non-interactive contexts like file list rows.
   */
  interactive?: boolean;
  /**
   * Builds the cover URL, or returns null when there is no cover. Defaults to
   * the authenticated file cover endpoint (`fileCoverUrl`) when the file has
   * a cover.
   */
  getCoverUrl?: (file: File) => string | null;
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
  interactive = true,
  getCoverUrl,
}: FileCoverThumbnailProps) {
  const [imageLoaded, setImageLoaded] = useState(false);
  const [imageError, setImageError] = useState(false);

  const image = useFileImage(file, getCoverUrl);
  const coverUrl = image?.url ?? null;

  // Reset both flags when the URL would change — either the cover filename
  // was replaced or updated_at bumped from a data refetch. Without resetting
  // imageError, a transient load failure latches the placeholder forever
  // because the <img> is conditionally unrendered and the `key` bump can't
  // remount it. Without resetting imageLoaded, the remounted <img> renders at
  // full opacity during the fresh fetch, skipping the fade-in.
  useEffect(() => {
    setImageError(false);
    setImageLoaded(false);
  }, [file.cover_image_filename, coverUrl]);

  const isAudiobook = file.file_type === "m4b";
  const aspectClass = isAudiobook ? "aspect-square" : "aspect-[2/3]";
  const placeholderVariant = isAudiobook ? "audiobook" : "book";

  const hasCover = coverUrl && !imageError;
  // A supplement with no picture reads as a document of its type, not as a
  // book with a missing cover.
  const typeTile = !hasCover && file.file_role === FileRoleSupplement;

  return (
    <div
      className={cn(
        "relative overflow-hidden rounded border border-border shrink-0",
        interactive && onClick && "cursor-pointer",
        interactive &&
          "transition-all duration-200 hover:scale-105 hover:shadow-md",
        aspectClass,
        className,
      )}
      onClick={onClick}
    >
      {typeTile ? (
        // Sized to its tile: a row thumbnail or the file page's header.
        <div className="@container absolute inset-0 flex items-end justify-center bg-muted pb-[8%]">
          <span className="text-[9px] font-semibold tracking-wide text-muted-foreground @[6rem]:text-sm">
            {fileTypeLabel(file.file_type)}
          </span>
        </div>
      ) : (
        (!imageLoaded || !hasCover) && (
          <CoverPlaceholder
            className="absolute inset-0"
            variant={placeholderVariant}
          />
        )
      )}

      {hasCover && (
        <CoverImage
          alt=""
          className={cn(
            "absolute inset-0 w-full h-full object-cover",
            // A page stands in for a cover from its top, on paper white.
            image?.isPage && "bg-white object-top",
            !imageLoaded && "opacity-0",
          )}
          key={coverUrl}
          onError={() => setImageError(true)}
          onLoad={() => setImageLoaded(true)}
          src={coverUrl}
          thumbnailUrl={image?.thumbnailUrl}
        />
      )}
    </div>
  );
}

export default FileCoverThumbnail;
