import { useState } from "react";

import { cn } from "@/libraries/utils";

export interface CBZPageThumbnailProps {
  fileId: number;
  page: number;
  size?: number;
  onClick?: () => void;
}

/**
 * Renders a thumbnail image for a CBZ page.
 *
 * Shows a grey placeholder while loading and displays the page number
 * if the image fails to load.
 */
const CBZPageThumbnail = ({
  fileId,
  page,
  size = 60,
  onClick,
}: CBZPageThumbnailProps) => {
  const [isLoading, setIsLoading] = useState(true);
  const [hasError, setHasError] = useState(false);

  const handleLoad = () => {
    setIsLoading(false);
  };

  const handleError = () => {
    setIsLoading(false);
    setHasError(true);
  };

  return (
    <div
      className={cn(
        "relative rounded border border-border overflow-hidden bg-muted flex items-center justify-center",
        onClick && "cursor-pointer",
      )}
      onClick={onClick}
      style={{ height: `${size}px`, minWidth: `${size}px` }}
    >
      {/* Loading placeholder */}
      {isLoading && !hasError && (
        <div className="absolute inset-0 bg-muted animate-pulse" />
      )}

      {/* Error state: show page number */}
      {hasError ? (
        <span className="text-muted-foreground text-sm font-medium">
          {page}
        </span>
      ) : (
        <img
          alt={`Page ${page}`}
          className={cn(
            "h-full w-auto object-contain",
            isLoading && "opacity-0",
          )}
          onError={handleError}
          onLoad={handleLoad}
          src={`/api/books/files/${fileId}/page/${page}`}
        />
      )}
    </div>
  );
};

export default CBZPageThumbnail;
