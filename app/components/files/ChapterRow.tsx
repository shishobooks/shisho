import CBZPagePicker from "./CBZPagePicker";
import CBZPagePreview from "./CBZPagePreview";
import {
  countDescendants,
  formatTimestampMs,
  parseTimestampMs,
} from "./chapterUtils";
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Minus,
  Play,
  Plus,
  Square,
  Trash2,
} from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/libraries/utils";
import {
  FileTypeCBZ,
  FileTypeEPUB,
  FileTypeM4B,
  type Chapter,
  type FileType,
} from "@/types";

/** Pixels of indentation per depth level for EPUB hierarchy */
const INDENT_PX_PER_LEVEL = 24;

/** Duration of the audio preview in milliseconds */
const PREVIEW_DURATION_MS = 10000;

interface PlayButtonProps {
  isPlaying: boolean;
  onPlay: () => void;
  onStop: () => void;
}

/**
 * Play/Stop button with circular progress indicator.
 * Shows a 10-second progress ring around the button when playing.
 */
const PlayButton = ({ isPlaying, onPlay, onStop }: PlayButtonProps) => {
  // SVG circle properties - using viewBox for easy centering
  const size = 32;
  const strokeWidth = 2;
  const radius = (size - strokeWidth) / 2;
  const circumference = 2 * Math.PI * radius;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          className="relative p-1.5 rounded hover:bg-muted text-muted-foreground"
          onClick={isPlaying ? onStop : onPlay}
          type="button"
        >
          {/* Circular progress ring */}
          {isPlaying && (
            <svg
              className="absolute inset-0 w-full h-full"
              style={{ transform: "rotate(-90deg)" }}
              viewBox={`0 0 ${size} ${size}`}
            >
              {/* Background circle */}
              <circle
                className="text-muted-foreground/20"
                cx={size / 2}
                cy={size / 2}
                fill="none"
                r={radius}
                stroke="currentColor"
                strokeWidth={strokeWidth}
              />
              {/* Progress circle */}
              <circle
                className="text-primary"
                cx={size / 2}
                cy={size / 2}
                fill="none"
                r={radius}
                stroke="currentColor"
                strokeDasharray={circumference}
                strokeDashoffset={circumference}
                strokeLinecap="round"
                strokeWidth={strokeWidth}
                style={{
                  animation: `progress-ring ${PREVIEW_DURATION_MS}ms linear forwards`,
                }}
              />
              <style>
                {`
                  @keyframes progress-ring {
                    from {
                      stroke-dashoffset: ${circumference};
                    }
                    to {
                      stroke-dashoffset: 0;
                    }
                  }
                `}
              </style>
            </svg>
          )}
          {isPlaying ? (
            <Square className="h-4 w-4" />
          ) : (
            <Play className="h-4 w-4" />
          )}
        </button>
      </TooltipTrigger>
      <TooltipContent>{isPlaying ? "Stop" : "Play 10s preview"}</TooltipContent>
    </Tooltip>
  );
};

export interface ChapterRowProps {
  chapter: Chapter;
  fileType: FileType;
  isEditing: boolean;
  depth: number;
  // Edit mode callbacks (only needed when isEditing=true)
  onTitleChange?: (title: string) => void;
  onStartPageChange?: (page: number) => void;
  onStartTimestampChange?: (ms: number) => void;
  onDelete?: () => void;
  onValidationChange?: (chapterId: number, hasError: boolean) => void;
  // CBZ edit mode: called when page input loses focus (for reordering)
  onBlur?: () => void;
  // EPUB edit mode: callbacks for child chapter editing (curried by index)
  onChildTitleChange?: (childIndex: number) => (title: string) => void;
  onChildDelete?: (childIndex: number) => () => void;
  // M4B playback - uses chapterIndex in edit mode (since chapter.id may not exist)
  chapterIndex?: number;
  playingChapterIndex?: number | null;
  onPlay?: (chapterIndex: number, timestampMs: number) => void;
  onStop?: () => void;
  // CBZ view mode: navigation to reader
  libraryId?: number;
  bookId?: number;
  // CBZ
  fileId?: number;
  pageCount?: number;
  // M4B
  maxDurationMs?: number;
}

/**
 * Renders a single chapter row with type-specific display.
 *
 * View mode: Displays chapter title, position (based on file type), and thumbnail/play button.
 * Edit mode: Supports inline editing for EPUB (titles only), CBZ (titles + pages), and M4B (titles + timestamps).
 *
 * Handles recursive rendering of children chapters (for EPUB hierarchy).
 */
const ChapterRow = (props: ChapterRowProps) => {
  const {
    chapter,
    fileType,
    isEditing,
    depth,
    chapterIndex,
    playingChapterIndex,
    onPlay,
    onStop,
    libraryId,
    bookId,
    fileId,
  } = props;

  const children = (chapter.children?.filter(Boolean) as Chapter[]) ?? [];
  const hasChildren = children.length > 0;
  const isEpub = fileType === FileTypeEPUB;
  const isCbz = fileType === FileTypeCBZ;
  const isM4b = fileType === FileTypeM4B;
  const isPlaying =
    chapterIndex != null && playingChapterIndex === chapterIndex;

  // Calculate indentation for EPUB hierarchy
  const indentPx = isEpub ? depth * INDENT_PX_PER_LEVEL : 0;

  // EPUB hierarchy: track expanded state (defaults to expanded)
  const [expanded, setExpanded] = useState(true);

  // State for delete confirmation dialog (EPUB only)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  // CBZ edit mode state: local page value and validation
  // Internal data is 0-indexed, but UI displays 1-indexed
  const currentPage = chapter.start_page ?? 0;
  const [localPageValue, setLocalPageValue] = useState(String(currentPage + 1));
  const [hasPageError, setHasPageError] = useState(false);
  const [pagePickerOpen, setPagePickerOpen] = useState(false);

  // M4B edit mode state: local timestamp value and validation
  const currentTimestampMs = chapter.start_timestamp_ms ?? 0;
  const [localTimestampValue, setLocalTimestampValue] = useState(
    formatTimestampMs(currentTimestampMs),
  );
  const [hasTimestampError, setHasTimestampError] = useState(false);

  // Sync local page value when chapter.start_page changes (e.g., from parent state)
  // Display is 1-indexed
  useEffect(() => {
    setLocalPageValue(String((chapter.start_page ?? 0) + 1));
    setHasPageError(false);
  }, [chapter.start_page]);

  // Sync local timestamp value when chapter.start_timestamp_ms changes (e.g., from parent state)
  useEffect(() => {
    setLocalTimestampValue(formatTimestampMs(chapter.start_timestamp_ms ?? 0));
    setHasTimestampError(false);
  }, [chapter.start_timestamp_ms]);

  // CBZ helper: Validate page number (1-indexed display value)
  const pageCount = props.pageCount ?? 0;
  const validatePage = (displayValue: number): boolean => {
    return displayValue >= 1 && displayValue <= pageCount;
  };

  // CBZ handler: Page input change (input is 1-indexed, convert to 0-indexed for storage)
  const handlePageInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value;
    setLocalPageValue(value);

    const displayValue = parseInt(value, 10);
    if (isNaN(displayValue)) {
      setHasPageError(true);
    } else if (!validatePage(displayValue)) {
      setHasPageError(true);
    } else {
      setHasPageError(false);
      // Convert from 1-indexed display to 0-indexed storage
      props.onStartPageChange?.(displayValue - 1);
    }
  };

  // CBZ handler: Decrement page
  const handleDecrementPage = () => {
    const newPage = Math.max(0, currentPage - 1);
    setLocalPageValue(String(newPage + 1)); // Display is 1-indexed
    setHasPageError(false);
    props.onStartPageChange?.(newPage);
    props.onBlur?.();
  };

  // CBZ handler: Increment page
  const handleIncrementPage = () => {
    const newPage = Math.min(pageCount - 1, currentPage + 1);
    setLocalPageValue(String(newPage + 1)); // Display is 1-indexed
    setHasPageError(false);
    props.onStartPageChange?.(newPage);
    props.onBlur?.();
  };

  // CBZ handler: Page selection from picker (receives 0-indexed page)
  const handlePageSelect = (page: number) => {
    setLocalPageValue(String(page + 1)); // Display is 1-indexed
    setHasPageError(false);
    props.onStartPageChange?.(page);
    props.onBlur?.();
  };

  // M4B helpers
  const maxDurationMs = props.maxDurationMs ?? Infinity;

  // M4B helper: Validate timestamp
  const validateTimestamp = (ms: number): boolean => {
    return ms >= 0 && ms <= maxDurationMs;
  };

  // M4B handler: Timestamp input change
  const handleTimestampInputChange = (
    e: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const value = e.target.value;
    setLocalTimestampValue(value);
  };

  // M4B handler: Timestamp input blur - validate and update
  const handleTimestampBlur = () => {
    const parsedMs = parseTimestampMs(localTimestampValue);
    if (parsedMs === null) {
      setHasTimestampError(true);
      props.onValidationChange?.(chapter.id, true);
    } else if (!validateTimestamp(parsedMs)) {
      setHasTimestampError(true);
      props.onValidationChange?.(chapter.id, true);
    } else {
      setHasTimestampError(false);
      props.onValidationChange?.(chapter.id, false);
      props.onStartTimestampChange?.(parsedMs);
    }
    props.onBlur?.();
  };

  // M4B handler: Decrement timestamp by 1 second
  const handleDecrementTimestamp = () => {
    const newMs = Math.max(0, currentTimestampMs - 1000);
    setLocalTimestampValue(formatTimestampMs(newMs));
    setHasTimestampError(false);
    props.onValidationChange?.(chapter.id, false);
    props.onStartTimestampChange?.(newMs);
    props.onBlur?.();
  };

  // M4B handler: Increment timestamp by 1 second
  const handleIncrementTimestamp = () => {
    const newMs = Math.min(maxDurationMs, currentTimestampMs + 1000);
    setLocalTimestampValue(formatTimestampMs(newMs));
    setHasTimestampError(false);
    props.onValidationChange?.(chapter.id, false);
    props.onStartTimestampChange?.(newMs);
    props.onBlur?.();
  };

  // EPUB edit mode
  if (isEditing && isEpub) {
    const descendantCount = countDescendants(chapter);
    const hasDescendants = descendantCount > 0;

    return (
      <>
        <div
          className="flex items-center gap-3 py-2 border-b border-border last:border-b-0"
          style={{ paddingLeft: `${indentPx}px` }}
        >
          {/* Expand/collapse toggle for chapters with children */}
          {hasChildren ? (
            <button
              className="p-0.5 rounded hover:bg-muted text-muted-foreground"
              onClick={() => setExpanded(!expanded)}
              type="button"
            >
              {expanded ? (
                <ChevronDown className="h-4 w-4" />
              ) : (
                <ChevronRight className="h-4 w-4" />
              )}
            </button>
          ) : (
            // Spacer for alignment when no toggle needed
            <div className="w-5" />
          )}

          {/* Title input */}
          <Input
            className="flex-1"
            onChange={(e) => props.onTitleChange?.(e.target.value)}
            placeholder="Chapter title"
            value={chapter.title}
          />

          {/* Delete button */}
          <Button
            onClick={() => {
              if (hasDescendants) {
                setDeleteDialogOpen(true);
              } else {
                props.onDelete?.();
              }
            }}
            size="icon"
            title="Delete chapter"
            type="button"
            variant="ghost"
          >
            <Trash2 className="h-4 w-4 text-muted-foreground hover:text-destructive" />
          </Button>
        </div>

        {/* Delete confirmation dialog for chapters with children */}
        <Dialog onOpenChange={setDeleteDialogOpen} open={deleteDialogOpen}>
          <DialogContent>
            <DialogHeader className="pr-8">
              <DialogTitle>Delete Chapter</DialogTitle>
              <DialogDescription>
                Delete "{chapter.title}" and its {descendantCount} chapter
                {descendantCount === 1 ? "" : "s"}?
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button
                onClick={() => setDeleteDialogOpen(false)}
                variant="outline"
              >
                Cancel
              </Button>
              <Button
                onClick={() => {
                  setDeleteDialogOpen(false);
                  props.onDelete?.();
                }}
                variant="destructive"
              >
                Delete
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* Render children in edit mode when expanded */}
        {hasChildren &&
          expanded &&
          children.map((child, index) => (
            <ChapterRow
              chapter={child}
              depth={depth + 1}
              fileId={fileId}
              fileType={fileType}
              isEditing={isEditing}
              key={child.id ?? `new-${index}`}
              onDelete={props.onChildDelete?.(index)}
              onPlay={onPlay}
              onStop={onStop}
              onTitleChange={props.onChildTitleChange?.(index)}
              playingChapterIndex={playingChapterIndex}
            />
          ))}
      </>
    );
  }

  // CBZ edit mode
  if (isEditing && isCbz) {
    return (
      <div className="flex items-center gap-3 py-2 border-b border-border last:border-b-0">
        {/* Small thumbnail with hover preview (clickable to open page picker) */}
        {fileId != null && (
          <CBZPagePreview
            fileId={fileId}
            onClick={() => setPagePickerOpen(true)}
            page={currentPage}
            thumbnailSize={60}
          />
        )}

        {/* Title input */}
        <Input
          className="flex-1"
          onChange={(e) => props.onTitleChange?.(e.target.value)}
          placeholder="Chapter title"
          value={chapter.title}
        />

        {/* Start page input with -/+ buttons */}
        <div className="flex items-center gap-1">
          <Button
            disabled={currentPage <= 0}
            onClick={handleDecrementPage}
            size="icon"
            title="Previous page"
            type="button"
            variant="ghost"
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <Input
            className={cn(
              "w-16 text-center",
              hasPageError && "border-red-500 focus-visible:ring-red-500",
            )}
            onBlur={() => props.onBlur?.()}
            onChange={handlePageInputChange}
            type="number"
            value={localPageValue}
          />
          <Button
            disabled={currentPage >= pageCount - 1}
            onClick={handleIncrementPage}
            size="icon"
            title="Next page"
            type="button"
            variant="ghost"
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>

        {/* Delete button (immediate, no confirmation for CBZ) */}
        <Button
          onClick={() => props.onDelete?.()}
          size="icon"
          title="Delete chapter"
          type="button"
          variant="ghost"
        >
          <Trash2 className="h-4 w-4 text-muted-foreground hover:text-destructive" />
        </Button>

        {/* Page picker dialog */}
        {fileId != null && (
          <CBZPagePicker
            currentPage={currentPage}
            fileId={fileId}
            key={fileId}
            onOpenChange={setPagePickerOpen}
            onSelect={handlePageSelect}
            open={pagePickerOpen}
            pageCount={pageCount}
          />
        )}
      </div>
    );
  }

  // M4B edit mode
  if (isEditing && isM4b) {
    return (
      <div className="flex items-center gap-3 py-2 border-b border-border last:border-b-0">
        {/* Title input */}
        <Input
          className="flex-1"
          onChange={(e) => props.onTitleChange?.(e.target.value)}
          placeholder="Chapter title"
          value={chapter.title}
        />

        {/* Play button and timestamp controls grouped together */}
        <div className="flex items-center gap-1">
          {/* Play/Stop button with progress ring */}
          <PlayButton
            isPlaying={isPlaying}
            onPlay={() => {
              if (chapterIndex != null) {
                // Use parsed local value which is always up-to-date, falling back to prop value
                const timestampMs =
                  parseTimestampMs(localTimestampValue) ?? currentTimestampMs;
                onPlay?.(chapterIndex, timestampMs);
              }
            }}
            onStop={() => onStop?.()}
          />
          <Button
            disabled={currentTimestampMs <= 0}
            onClick={handleDecrementTimestamp}
            size="icon"
            title="Subtract 1 second"
            type="button"
            variant="ghost"
          >
            <Minus className="h-4 w-4" />
          </Button>
          <Tooltip open={hasTimestampError}>
            <TooltipTrigger asChild>
              <Input
                className={cn(
                  "w-28 text-center font-mono",
                  hasTimestampError &&
                    "border-red-500 focus-visible:ring-red-500",
                )}
                onBlur={handleTimestampBlur}
                onChange={handleTimestampInputChange}
                placeholder="00:00:00.000"
                value={localTimestampValue}
              />
            </TooltipTrigger>
            <TooltipContent side="bottom">
              Timestamp exceeds audiobook duration
            </TooltipContent>
          </Tooltip>
          <Button
            disabled={currentTimestampMs >= maxDurationMs}
            onClick={handleIncrementTimestamp}
            size="icon"
            title="Add 1 second"
            type="button"
            variant="ghost"
          >
            <Plus className="h-4 w-4" />
          </Button>
        </div>

        {/* Delete button (immediate, no confirmation for M4B) */}
        <Button
          onClick={() => props.onDelete?.()}
          size="icon"
          title="Delete chapter"
          type="button"
          variant="ghost"
        >
          <Trash2 className="h-4 w-4 text-muted-foreground hover:text-destructive" />
        </Button>
      </div>
    );
  }

  // View mode rendering
  return (
    <>
      <div
        className="flex items-center gap-3 py-2 border-b border-border last:border-b-0"
        style={{ paddingLeft: `${indentPx}px` }}
      >
        {/* EPUB: Expand/collapse toggle for chapters with children */}
        {isEpub && hasChildren ? (
          <button
            className="p-0.5 rounded hover:bg-muted text-muted-foreground"
            onClick={() => setExpanded(!expanded)}
            type="button"
          >
            {expanded ? (
              <ChevronDown className="h-4 w-4" />
            ) : (
              <ChevronRight className="h-4 w-4" />
            )}
          </button>
        ) : isEpub ? (
          // Spacer for alignment when no toggle needed
          <div className="w-5" />
        ) : null}

        {/* CBZ: Page thumbnail with hover preview */}
        {isCbz && chapter.start_page != null && fileId != null && (
          <CBZPagePreview
            fileId={fileId}
            page={chapter.start_page}
            thumbnailSize={60}
          />
        )}

        {/* Chapter title - clickable for CBZ to open reader at this chapter */}
        {isCbz &&
        libraryId &&
        bookId &&
        fileId &&
        chapter.start_page != null ? (
          <Link
            className="flex-1 truncate hover:underline text-primary"
            to={`/libraries/${libraryId}/books/${bookId}/files/${fileId}/read?page=${chapter.start_page}`}
          >
            {chapter.title}
          </Link>
        ) : (
          <span className="flex-1 truncate">{chapter.title}</span>
        )}

        {/* Position display based on file type (1-indexed for user display) */}
        {isCbz && chapter.start_page != null && (
          <span className="text-muted-foreground text-sm">
            Page {chapter.start_page + 1}
          </span>
        )}
        {/* M4B: Play button and timestamp grouped together */}
        {isM4b && chapter.start_timestamp_ms != null && (
          <div className="flex items-center gap-1">
            <PlayButton
              isPlaying={isPlaying}
              onPlay={() =>
                chapterIndex != null &&
                onPlay?.(chapterIndex, chapter.start_timestamp_ms!)
              }
              onStop={() => onStop?.()}
            />
            <span className="text-muted-foreground text-sm font-mono">
              {formatTimestampMs(chapter.start_timestamp_ms)}
            </span>
          </div>
        )}
      </div>

      {/* EPUB: Render children recursively when expanded */}
      {isEpub &&
        hasChildren &&
        expanded &&
        children.map((child) => (
          <ChapterRow
            bookId={bookId}
            chapter={child}
            depth={depth + 1}
            fileId={fileId}
            fileType={fileType}
            isEditing={isEditing}
            key={child.id}
            libraryId={libraryId}
            onPlay={onPlay}
            onStop={onStop}
            playingChapterIndex={playingChapterIndex}
          />
        ))}
    </>
  );
};

export default ChapterRow;
