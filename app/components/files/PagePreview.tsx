import { useState } from "react";

import {
  HoverCard,
  HoverCardContent,
  HoverCardPortal,
  HoverCardTrigger,
} from "@/components/ui/hover-card";
import { useFilePageUrl } from "@/hooks/useFilePageUrl";
import { cn } from "@/libraries/utils";
import type { PageSourceFile } from "@/utils/pageUrl";

export interface PagePreviewProps {
  file: PageSourceFile;
  /** 0-indexed page number */
  page: number;
  /** Size of the thumbnail in pixels */
  thumbnailSize?: number;
  /** Size of the preview in pixels (default: 300) */
  previewSize?: number;
  /** Which side to show the preview on (default: "right") */
  side?: "top" | "right" | "bottom" | "left";
  /** Additional class name for the trigger wrapper */
  className?: string;
  /** Click handler for the thumbnail */
  onClick?: () => void;
  /** Children to render as the trigger (if not provided, renders default thumbnail) */
  children?: React.ReactNode;
}

/**
 * Page thumbnail with hover preview.
 *
 * Shows a small thumbnail that displays a larger preview on hover.
 * Can wrap custom children or render a default thumbnail.
 */
const PagePreview = ({
  file,
  page,
  thumbnailSize = 60,
  previewSize = 300,
  side = "right",
  className,
  onClick,
  children,
}: PagePreviewProps) => {
  const filePageUrl = useFilePageUrl();
  const [isLoading, setIsLoading] = useState(true);
  const [hasError, setHasError] = useState(false);
  const [previewLoading, setPreviewLoading] = useState(true);

  const imageUrl = filePageUrl(file, page);
  // Start over when the URL changes (another page, or a new cache key after
  // the file was replaced), so an earlier error does not hide the new image.
  const [shownUrl, setShownUrl] = useState(imageUrl);
  if (shownUrl !== imageUrl) {
    setShownUrl(imageUrl);
    setIsLoading(true);
    setHasError(false);
    setPreviewLoading(true);
  }

  const handleLoad = () => {
    setIsLoading(false);
  };

  const handleError = () => {
    setIsLoading(false);
    setHasError(true);
  };

  // Display is 1-indexed
  const displayPage = page + 1;

  // Default thumbnail content
  const defaultThumbnail = (
    <div
      className={cn(
        "relative rounded border border-border overflow-hidden bg-muted flex items-center justify-center",
        onClick && "cursor-pointer",
        className,
      )}
      onClick={onClick}
      style={{ height: `${thumbnailSize}px`, minWidth: `${thumbnailSize}px` }}
    >
      {/* Loading placeholder */}
      {isLoading && !hasError && (
        <div className="absolute inset-0 bg-muted animate-pulse" />
      )}

      {/* Error state: show page number */}
      {hasError ? (
        <span className="text-muted-foreground text-sm font-medium">
          {displayPage}
        </span>
      ) : (
        <img
          alt={`Page ${displayPage}`}
          className={cn(
            "h-full w-auto object-contain",
            isLoading && "opacity-0",
          )}
          onError={handleError}
          onLoad={handleLoad}
          src={imageUrl}
        />
      )}
    </div>
  );

  return (
    <HoverCard closeDelay={100} openDelay={300}>
      <HoverCardTrigger asChild>
        {children ?? defaultThumbnail}
      </HoverCardTrigger>
      <HoverCardPortal>
        <HoverCardContent
          className="w-auto p-2"
          collisionPadding={16}
          side={side}
          sideOffset={8}
        >
          <div
            className="relative rounded border border-border overflow-hidden bg-muted flex items-center justify-center"
            style={{
              height: `${previewSize}px`,
              minWidth: `${previewSize * 0.7}px`,
              maxWidth: `${previewSize * 1.5}px`,
            }}
          >
            {/* Loading placeholder for preview */}
            {previewLoading && (
              <div className="absolute inset-0 bg-muted animate-pulse" />
            )}
            <img
              alt={`Page ${displayPage} preview`}
              className={cn(
                "h-full w-auto object-contain",
                previewLoading && "opacity-0",
              )}
              onLoad={() => setPreviewLoading(false)}
              src={imageUrl}
            />
          </div>
          <p className="text-center text-sm text-muted-foreground mt-1">
            Page {displayPage}
          </p>
        </HoverCardContent>
      </HoverCardPortal>
    </HoverCard>
  );
};

export default PagePreview;
