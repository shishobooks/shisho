import { useEffect, useLayoutEffect, useState } from "react";

import CoverPlaceholder from "@/components/library/CoverPlaceholder";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/libraries/utils";
import type { File } from "@/types";
import { isCoverLoaded, markCoverLoaded } from "@/utils/coverCache";
import { fileCoverUrl } from "@/utils/coverUrl";
import { fileLabel } from "@/utils/format";

interface CoverGalleryTabsProps {
  files: File[];
  className?: string;
  /**
   * Builds the selected file's cover URL, or returns null when there is no
   * cover. Defaults to the authenticated file cover endpoint when the file has
   * a cover.
   */
  getCoverUrl?: (file: File) => string | null;
}

interface FileWithLabel extends File {
  /** Display label for the tab (e.g., "EPUB", "M4B 1", "M4B 2") */
  label: string;
}

/**
 * Generates display labels for files, adding numbers when multiple files share a type.
 * Single file of type: "EPUB", "M4B", "CBZ"
 * Multiple files of same type: "EPUB 1", "EPUB 2", etc.
 */
function getFilesWithLabels(files: File[]): FileWithLabel[] {
  // Count files by type
  const typeCounts: Record<string, number> = {};
  for (const file of files) {
    typeCounts[file.file_type] = (typeCounts[file.file_type] || 0) + 1;
  }

  // Track current index per type for numbering
  const typeIndexes: Record<string, number> = {};

  return files.map((file) => {
    const count = typeCounts[file.file_type];
    const typeUpper = file.file_type.toUpperCase();

    if (count === 1) {
      return { ...file, label: typeUpper };
    }

    // Multiple files of this type - add number
    typeIndexes[file.file_type] = (typeIndexes[file.file_type] || 0) + 1;
    return { ...file, label: `${typeUpper} ${typeIndexes[file.file_type]}` };
  });
}

/**
 * Cover gallery tabs that appear below the main cover image.
 * Allows switching between different file covers when a book has multiple files.
 * Only renders when there are 2+ files.
 */
function CoverGalleryTabs({
  files,
  className,
  getCoverUrl,
}: CoverGalleryTabsProps) {
  const [selectedFileId, setSelectedFileId] = useState<number | null>(null);
  const [coverLoaded, setCoverLoaded] = useState(false);
  const [coverError, setCoverError] = useState(false);

  const filesWithLabels = getFilesWithLabels(files);

  // Initialize selected file to first file
  useEffect(() => {
    if (files.length > 0 && selectedFileId === null) {
      setSelectedFileId(files[0].id);
    }
  }, [files, selectedFileId]);

  const selectedFile = filesWithLabels.find((f) => f.id === selectedFileId);
  const isAudiobook = selectedFile?.file_type === "m4b";
  const aspectClass = isAudiobook ? "aspect-square" : "aspect-[2/3]";
  const placeholderVariant = isAudiobook ? "audiobook" : "book";

  const coverUrl = !selectedFile
    ? null
    : getCoverUrl
      ? getCoverUrl(selectedFile)
      : selectedFile.cover_image_filename
        ? fileCoverUrl(selectedFile)
        : null;
  const hasCover = coverUrl && !coverError;

  // Check cache synchronously before paint
  useLayoutEffect(() => {
    if (coverUrl && isCoverLoaded(coverUrl)) {
      setCoverLoaded(true);
    }
  }, [coverUrl]);

  // Reset loading state on tab change so the placeholder shows while the
  // new file's cover loads.
  useEffect(() => {
    setCoverLoaded(false);
  }, [selectedFileId]);

  useEffect(() => {
    setCoverError(false);
  }, [coverUrl]);

  const handleCoverLoad = () => {
    if (coverUrl) {
      markCoverLoaded(coverUrl);
    }
    setCoverLoaded(true);
  };

  const handleTabClick = (fileId: number) => {
    if (fileId !== selectedFileId) {
      setSelectedFileId(fileId);
    }
  };

  // Don't render if only 1 file
  if (files.length <= 1) {
    return null;
  }

  return (
    <div className={cn("space-y-3", className)}>
      {/* Cover Image */}
      <div
        className={cn(
          "w-48 sm:w-64 lg:w-full mx-auto lg:mx-0 relative rounded-md border border-border overflow-hidden",
          aspectClass,
        )}
      >
        {/* Placeholder shown until image loads or on error */}
        {(!coverLoaded || !hasCover) && (
          <CoverPlaceholder
            className="absolute inset-0"
            variant={placeholderVariant}
          />
        )}

        {/* Image hidden until loaded */}
        {hasCover && (
          <img
            alt={`${selectedFile ? fileLabel(selectedFile) : "File"} Cover`}
            className={cn(
              "absolute inset-0 w-full h-full object-cover",
              !coverLoaded && "opacity-0",
            )}
            key={coverUrl}
            onError={() => setCoverError(true)}
            onLoad={handleCoverLoad}
            src={coverUrl}
          />
        )}
      </div>

      {/* Tabs */}
      <div className="flex justify-center lg:justify-start gap-1.5 flex-wrap">
        {filesWithLabels.map((file) => (
          <Tooltip key={file.id}>
            <TooltipTrigger asChild>
              <button
                className={cn(
                  "px-2.5 py-1 text-xs font-medium rounded-full border cursor-pointer",
                  "transition-all duration-150",
                  file.id === selectedFileId
                    ? "border-primary bg-primary/5 text-primary"
                    : "border-border text-muted-foreground hover:bg-accent hover:text-foreground",
                )}
                onClick={() => handleTabClick(file.id)}
                type="button"
              >
                {file.label}
              </button>
            </TooltipTrigger>
            <TooltipContent>{fileLabel(file)}</TooltipContent>
          </Tooltip>
        ))}
      </div>
    </div>
  );
}

export default CoverGalleryTabs;
