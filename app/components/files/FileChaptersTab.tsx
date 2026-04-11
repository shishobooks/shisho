import equal from "fast-deep-equal";
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";
import { toast } from "sonner";

import ChapterRow from "@/components/files/ChapterRow";
import {
  chaptersToInputArray,
  getNextChapterTitle,
  normalizeChapterOrder,
} from "@/components/files/chapterUtils";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Button } from "@/components/ui/button";
import {
  useFileChapters,
  useUpdateFileChapters,
} from "@/hooks/queries/chapters";
import {
  FileTypeCBZ,
  FileTypeEPUB,
  FileTypeM4B,
  FileTypePDF,
  type Chapter,
  type ChapterInput,
  type File,
} from "@/types";

export interface FileChaptersTabHandle {
  save: () => void;
  cancel: () => void;
}

export interface ChaptersActionState {
  isSaving: boolean;
  canSave: boolean;
  hasChanges: boolean;
}

interface FileChaptersTabProps {
  file: File;
  isEditing: boolean;
  onEditingChange: (editing: boolean) => void;
  onActionStateChange?: (state: ChaptersActionState) => void;
}

/**
 * Chapter shape used inside edit mode. The `_editKey` is a stable client-side
 * identifier assigned when a chapter enters edit state (either by loading from
 * the server or by being added via an Add Chapter button). Using `_editKey` as
 * the React `key` prevents index-based reconciliation bugs where editing one
 * chapter could silently mutate a sibling after a reorder.
 *
 * `_editKey` is stripped before submitting to the server via `stripEditKeys`.
 */
interface EditedChapter extends ChapterInput {
  _editKey: string;
  children: EditedChapter[];
}

// Module-level counter for generating unique edit keys. Keys only need to be
// unique within a single edit session, so a monotonic counter is sufficient.
let editKeyCounter = 0;
const nextEditKey = () => `ek-${++editKeyCounter}`;

const toEditedChapters = (chapters: ChapterInput[]): EditedChapter[] =>
  chapters.map((c) => ({
    ...c,
    _editKey: nextEditKey(),
    children: toEditedChapters(c.children ?? []),
  }));

// Destructure-based strip so any future ChapterInput field flows through
// automatically — only _editKey is removed, everything else rides in ...rest.
// children is named explicitly because EditedChapter.children is EditedChapter[]
// while ChapterInput.children is ChapterInput[], so the child list needs its
// own recursive strip to produce a ChapterInput-compatible shape.
const stripEditKeys = (chapters: EditedChapter[]): ChapterInput[] =>
  chapters.map((chapter) => {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    const { _editKey, children, ...rest } = chapter;
    return {
      ...rest,
      children: stripEditKeys(children),
    };
  });

/**
 * Creates a new chapter with appropriate defaults based on file type.
 * - CBZ/PDF: start_page defaults to 0
 * - M4B: start_timestamp_ms defaults to 0
 */
const createNewChapter = (fileType: string): EditedChapter => {
  const chapter: EditedChapter = {
    title: "",
    children: [],
    _editKey: nextEditKey(),
  };

  if (fileType === FileTypeCBZ || fileType === FileTypePDF) {
    chapter.start_page = 0;
  } else if (fileType === FileTypeM4B) {
    chapter.start_timestamp_ms = 0;
  }

  return chapter;
};

const FileChaptersTab = forwardRef<FileChaptersTabHandle, FileChaptersTabProps>(
  (props, ref) => {
    const { file, isEditing, onEditingChange, onActionStateChange } = props;
    const chaptersQuery = useFileChapters(file.id);
    const updateChaptersMutation = useUpdateFileChapters(file.id);

    // State for edited chapters (used in edit mode)
    const [editedChapters, setEditedChapters] = useState<EditedChapter[]>([]);

    // Track validation errors by chapter index (for M4B timestamp validation)
    const [validationErrors, setValidationErrors] = useState<
      Map<number, boolean>
    >(new Map());

    // Track whether we've initialized edit mode for this editing session
    const editInitializedRef = useRef(false);

    // Store initial chapters for change detection
    const [initialChapters, setInitialChapters] = useState<EditedChapter[]>([]);

    // M4B audio playback state
    const audioRef = useRef<HTMLAudioElement>(null);
    const [playingChapterIndex, setPlayingChapterIndex] = useState<
      number | null
    >(null);
    const playbackTimeoutRef = useRef<number | null>(null);

    const chapters = useMemo(
      () => chaptersQuery.data ?? [],
      [chaptersQuery.data],
    );

    // Initialize editedChapters when entering edit mode
    useEffect(() => {
      if (isEditing && !editInitializedRef.current && chapters.length > 0) {
        const chaptersAsInput = toEditedChapters(
          chaptersToInputArray(chapters),
        );
        setEditedChapters(chaptersAsInput);
        setInitialChapters(chaptersAsInput);
        editInitializedRef.current = true;
      }
      if (!isEditing) {
        editInitializedRef.current = false;
        setInitialChapters([]);
        setValidationErrors(new Map()); // Clear validation errors when exiting edit mode
      }
      // chapters reference changes when chaptersQuery.data changes, which is what we want
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [isEditing, chaptersQuery.data]);

    // Compute whether any validation errors exist
    const hasValidationErrors = Array.from(validationErrors.values()).some(
      (hasError) => hasError,
    );

    /**
     * Stops audio playback and clears the playback timeout.
     */
    const handleAudioStop = useCallback(() => {
      if (playbackTimeoutRef.current) {
        clearTimeout(playbackTimeoutRef.current);
        playbackTimeoutRef.current = null;
      }
      if (audioRef.current) {
        audioRef.current.pause();
      }
      setPlayingChapterIndex(null);
    }, []);

    /**
     * Starts audio playback at a chapter's timestamp.
     * Automatically stops after 10 seconds.
     *
     * Note: Setting currentTime triggers a seek operation. The audio may not
     * have enough data buffered at the target position to play immediately.
     * We need to wait for both the seek to complete AND the audio to have
     * enough data (canplay event) before calling play().
     */
    const handleAudioPlay = useCallback(
      (chapterIndex: number, timestampMs: number) => {
        // Stop any current playback first
        handleAudioStop();

        if (audioRef.current) {
          const audio = audioRef.current;

          // Set the audio position to the chapter's timestamp
          audio.currentTime = timestampMs / 1000;

          // Helper to start playback and set up auto-stop timer
          const startPlayback = () => {
            audio.play().catch((error: Error) => {
              // Ignore AbortError - this happens when pause() interrupts play()
              if (error.name !== "AbortError") {
                console.error("Audio playback error:", error);
              }
            });

            // Auto-stop after 10 seconds
            playbackTimeoutRef.current = window.setTimeout(() => {
              handleAudioStop();
            }, 10000);
          };

          // Helper to check if audio is ready to play at current position
          // readyState >= 3 (HAVE_FUTURE_DATA) means we have enough data to play
          const canPlayNow = () => audio.readyState >= 3;

          // Track if playback has started to avoid duplicate starts
          let playbackStarted = false;

          // Try to play, waiting for audio to be ready if needed
          const tryPlay = () => {
            if (playbackStarted) return;

            if (canPlayNow()) {
              playbackStarted = true;
              startPlayback();
            } else {
              // Wait for canplay event which fires when readyState >= 3
              const onCanPlay = () => {
                if (playbackStarted) return;
                playbackStarted = true;
                audio.removeEventListener("canplay", onCanPlay);
                startPlayback();
              };
              audio.addEventListener("canplay", onCanPlay);

              // Timeout fallback: if canplay doesn't fire within 5 seconds,
              // the audio codec may not be supported. Reset and show error.
              setTimeout(() => {
                if (!playbackStarted) {
                  audio.removeEventListener("canplay", onCanPlay);
                  setPlayingChapterIndex(null);
                  toast.error("Unable to play audio preview", {
                    description:
                      "The audio codec may not be supported by your browser.",
                  });
                }
              }, 5000);
            }
          };

          // If seeking, wait for seek to complete first, then check if playable
          if (audio.seeking) {
            const onSeeked = () => {
              audio.removeEventListener("seeked", onSeeked);
              tryPlay();
            };
            audio.addEventListener("seeked", onSeeked);

            // Timeout fallback: if seeking doesn't complete within 5 seconds,
            // the audio codec may not be supported. Reset and show error.
            setTimeout(() => {
              if (!playbackStarted && audio.seeking) {
                audio.removeEventListener("seeked", onSeeked);
                setPlayingChapterIndex(null);
                toast.error("Unable to play audio preview", {
                  description:
                    "The audio codec may not be supported by your browser.",
                });
              }
            }, 5000);
          } else {
            // Not seeking, try to play immediately
            tryPlay();
          }

          setPlayingChapterIndex(chapterIndex);
        }
      },
      [handleAudioStop],
    );

    /**
     * Handles play from ChapterRow, which passes both chapterIndex and timestampMs.
     * Uses the passed timestamp directly (no lookup needed).
     */
    const handleChapterPlay = useCallback(
      (chapterIndex: number, timestampMs: number) => {
        handleAudioPlay(chapterIndex, timestampMs);
      },
      [handleAudioPlay],
    );

    // Cleanup playback timeout on unmount
    useEffect(() => {
      return () => {
        if (playbackTimeoutRef.current) {
          clearTimeout(playbackTimeoutRef.current);
        }
      };
    }, []);

    // Stop playback when entering edit mode
    useEffect(() => {
      if (isEditing) {
        handleAudioStop();
      }
    }, [isEditing, handleAudioStop]);

    /**
     * Handles validation state changes from ChapterRow.
     * Updates the validationErrors map with the chapter's validation state.
     */
    const handleValidationChange = (index: number, hasError: boolean) => {
      setValidationErrors((prev) => {
        const next = new Map(prev);
        if (hasError) {
          next.set(index, true);
        } else {
          next.delete(index);
        }
        return next;
      });
    };

    /**
     * Handles clicking "Add Chapter" from empty state.
     * Creates a new chapter with defaults and enters edit mode.
     */
    const handleAddChapterFromEmpty = () => {
      const newChapter = createNewChapter(file.file_type);
      setEditedChapters([newChapter]);
      setInitialChapters([]); // Starting from empty, so initial is empty
      editInitializedRef.current = true;
      onEditingChange(true);
    };

    /**
     * Handles clicking the uncovered pages warning for page-based files.
     * Adds a new chapter at page 0 and enters edit mode.
     */
    const handleAddChapterAtPageZero = () => {
      const newChapter: EditedChapter = {
        title: "",
        start_page: 0,
        children: [],
        _editKey: nextEditKey(),
      };
      // Prepend the new chapter to existing chapters
      const existingChaptersAsInputs = toEditedChapters(
        chaptersToInputArray(chapters),
      );
      setEditedChapters([newChapter, ...existingChaptersAsInputs]);
      setInitialChapters(existingChaptersAsInputs); // Initial is the existing chapters
      editInitializedRef.current = true;
      onEditingChange(true);
    };

    /**
     * Handles saving edited chapters. Normalizes chapter order (page-based
     * sorted by start_page, M4B sorted by start_timestamp_ms) before the
     * mutation fires so out-of-order edits can't land in the DB, then strips
     * client-only `_editKey` fields before sending to the server.
     */
    const handleSave = () => {
      const normalized = normalizeChapterOrder(
        stripEditKeys(editedChapters),
        file.file_type,
      );
      updateChaptersMutation.mutate(
        { chapters: normalized },
        {
          onSuccess: () => {
            toast.success("Chapters saved");
            onEditingChange(false);
          },
          onError: (error) => {
            toast.error(error.message || "Error saving chapters");
          },
        },
      );
    };

    /**
     * Handles canceling edit mode.
     */
    const handleCancel = () => {
      setEditedChapters([]);
      editInitializedRef.current = false;
      onEditingChange(false);
    };

    // Expose save/cancel methods to parent via ref
    useImperativeHandle(
      ref,
      () => ({
        save: handleSave,
        cancel: handleCancel,
      }),
      // eslint-disable-next-line react-hooks/exhaustive-deps
      [editedChapters],
    );

    // Compute whether chapters have been modified
    const hasChanges = useMemo(() => {
      if (!isEditing) return false;
      return !equal(editedChapters, initialChapters);
    }, [isEditing, editedChapters, initialChapters]);

    // Report action state changes to parent
    useEffect(() => {
      onActionStateChange?.({
        isSaving: updateChaptersMutation.isPending,
        canSave: isEditing && !hasValidationErrors,
        hasChanges,
      });
    }, [
      updateChaptersMutation.isPending,
      hasValidationErrors,
      hasChanges,
      isEditing,
      onActionStateChange,
    ]);

    /**
     * Reorders edited chapters by start_page. Fired from ChapterRow's `onBlur`
     * only on "commit" actions — the page input's real onBlur, and picker
     * selection. The +/- buttons intentionally do NOT trigger this, so rapid
     * button clicks don't swap chapter positions mid-stream (which made the
     * next click land on a different chapter — see the regression test).
     */
    const handleBlurReorder = useCallback(() => {
      setEditedChapters((prev) => {
        const sorted = [...prev].sort(
          (a, b) => (a.start_page ?? 0) - (b.start_page ?? 0),
        );
        return sorted;
      });
    }, []);

    /**
     * Reorders edited chapters by start_timestamp_ms on M4B timestamp commit.
     * Also stops playback since reordering invalidates playingChapterIndex.
     */
    const handleTimestampBlurReorder = useCallback(() => {
      handleAudioStop();
      setEditedChapters((prev) =>
        [...prev].sort(
          (a, b) => (a.start_timestamp_ms ?? 0) - (b.start_timestamp_ms ?? 0),
        ),
      );
    }, [handleAudioStop]);

    /**
     * Handles adding a new chapter for page-based files.
     * Defaults:
     * - Start page: last chapter's start_page + 1 (or 0 if no chapters)
     * - Title: pattern detection from last chapter title
     */
    const handleAddChapter = () => {
      const lastChapter = editedChapters[editedChapters.length - 1];
      const startPage = lastChapter ? (lastChapter.start_page ?? 0) + 1 : 0;
      const title = lastChapter ? getNextChapterTitle(lastChapter.title) : "";

      const newChapter: EditedChapter = {
        title,
        start_page: startPage,
        children: [],
        _editKey: nextEditKey(),
      };

      setEditedChapters((prev) => [...prev, newChapter]);
    };

    /**
     * Handles adding a new chapter for M4B files.
     * Defaults:
     * - Start timestamp: last chapter's start_timestamp_ms + 1000 (or 0 if no chapters)
     * - Title: pattern detection from last chapter title
     */
    const handleAddChapterM4B = () => {
      const lastChapter = editedChapters[editedChapters.length - 1];
      const startTimestampMs = lastChapter
        ? (lastChapter.start_timestamp_ms ?? 0) + 1000
        : 0;
      const title = lastChapter ? getNextChapterTitle(lastChapter.title) : "";

      const newChapter: EditedChapter = {
        title,
        start_timestamp_ms: startTimestampMs,
        children: [],
        _editKey: nextEditKey(),
      };

      setEditedChapters((prev) => [...prev, newChapter]);
    };

    /**
     * Updates a chapter's title at a specific path in the tree.
     * For EPUB, chapters can be nested, so we need to handle the path.
     */
    const updateChapterTitle = (
      chapters: EditedChapter[],
      index: number,
      title: string,
    ): EditedChapter[] => {
      return chapters.map((chapter, i) => {
        if (i === index) {
          return { ...chapter, title };
        }
        return chapter;
      });
    };

    /**
     * Updates a chapter's start_page at a specific index.
     */
    const updateChapterStartPage = (
      chapters: EditedChapter[],
      index: number,
      startPage: number,
    ): EditedChapter[] => {
      return chapters.map((chapter, i) => {
        if (i === index) {
          return { ...chapter, start_page: startPage };
        }
        return chapter;
      });
    };

    /**
     * Updates a chapter's start_timestamp_ms at a specific index.
     */
    const updateChapterStartTimestamp = (
      chapters: EditedChapter[],
      index: number,
      startTimestampMs: number,
    ): EditedChapter[] => {
      return chapters.map((chapter, i) => {
        if (i === index) {
          return { ...chapter, start_timestamp_ms: startTimestampMs };
        }
        return chapter;
      });
    };

    /**
     * Deletes a chapter at a specific index from the array.
     * For EPUB chapters with children, this also removes all descendants.
     */
    const deleteChapter = (
      chapters: EditedChapter[],
      index: number,
    ): EditedChapter[] => {
      return chapters.filter((_, i) => i !== index);
    };

    /**
     * Updates a child chapter's title within a parent chapter.
     */
    const updateChildTitle = (
      chapters: EditedChapter[],
      parentIndex: number,
      childIndex: number,
      title: string,
    ): EditedChapter[] => {
      return chapters.map((chapter, i) => {
        if (i === parentIndex) {
          return {
            ...chapter,
            children: updateChapterTitle(chapter.children, childIndex, title),
          };
        }
        return chapter;
      });
    };

    /**
     * Deletes a child chapter from a parent chapter.
     */
    const deleteChildChapter = (
      chapters: EditedChapter[],
      parentIndex: number,
      childIndex: number,
    ): EditedChapter[] => {
      return chapters.map((chapter, i) => {
        if (i === parentIndex) {
          return {
            ...chapter,
            children: deleteChapter(chapter.children, childIndex),
          };
        }
        return chapter;
      });
    };

    /**
     * Creates callbacks for child chapter editing.
     * These are curried functions that close over the parent index.
     */
    const createChildTitleChangeCallback =
      (parentIndex: number) => (childIndex: number) => (title: string) => {
        setEditedChapters((prev) =>
          updateChildTitle(prev, parentIndex, childIndex, title),
        );
      };

    const createChildDeleteCallback =
      (parentIndex: number) => (childIndex: number) => () => {
        setEditedChapters((prev) =>
          deleteChildChapter(prev, parentIndex, childIndex),
        );
      };

    // Loading state
    if (chaptersQuery.isLoading) {
      return (
        <div className="py-8 flex justify-center">
          <LoadingSpinner />
        </div>
      );
    }

    // Error state
    if (chaptersQuery.isError) {
      return (
        <div className="py-8 text-center">
          <p className="text-destructive">Failed to load chapters</p>
        </div>
      );
    }

    // Empty state (or edit mode with new chapters from empty state)
    if (chapters.length === 0) {
      const canAddChapters =
        file.file_type === FileTypeCBZ ||
        file.file_type === FileTypePDF ||
        file.file_type === FileTypeM4B;

      // When editing with chapters (entered via Add Chapter button), show edit UI
      if (isEditing && editedChapters.length > 0) {
        const isPageBased =
          file.file_type === FileTypeCBZ || file.file_type === FileTypePDF;
        const isM4b = file.file_type === FileTypeM4B;
        const maxDurationMs = file.audiobook_duration_seconds
          ? file.audiobook_duration_seconds * 1000
          : undefined;

        return (
          <div className="py-4">
            {/* Hidden audio element for M4B playback */}
            {isM4b && (
              <audio
                ref={audioRef}
                src={`/api/books/files/${file.id}/stream`}
              />
            )}

            {editedChapters.map((chapter, index) => (
              <ChapterRow
                chapter={chapter as unknown as Chapter}
                chapterIndex={isM4b ? index : undefined}
                depth={0}
                fileId={file.id}
                fileType={file.file_type}
                isEditing={true}
                key={chapter._editKey}
                maxDurationMs={isM4b ? maxDurationMs : undefined}
                onBlur={
                  isPageBased
                    ? handleBlurReorder
                    : isM4b
                      ? handleTimestampBlurReorder
                      : undefined
                }
                onDelete={() =>
                  setEditedChapters((prev) => deleteChapter(prev, index))
                }
                onPlay={isM4b ? handleChapterPlay : undefined}
                onStartPageChange={
                  isPageBased
                    ? (page) =>
                        setEditedChapters((prev) =>
                          updateChapterStartPage(prev, index, page),
                        )
                    : undefined
                }
                onStartTimestampChange={
                  isM4b
                    ? (ms) =>
                        setEditedChapters((prev) =>
                          updateChapterStartTimestamp(prev, index, ms),
                        )
                    : undefined
                }
                onStop={isM4b ? handleAudioStop : undefined}
                onTitleChange={(title) =>
                  setEditedChapters((prev) =>
                    updateChapterTitle(prev, index, title),
                  )
                }
                onValidationChange={
                  isM4b
                    ? (_chapterId, hasError) =>
                        handleValidationChange(index, hasError)
                    : undefined
                }
                pageCount={isPageBased ? (file.page_count ?? 0) : undefined}
                playingChapterIndex={isM4b ? playingChapterIndex : undefined}
              />
            ))}

            {/* Add Chapter button for page-based files */}
            {isPageBased && (
              <Button
                className="mt-2"
                onClick={handleAddChapter}
                type="button"
                variant="outline"
              >
                Add Chapter
              </Button>
            )}

            {/* Add Chapter button for M4B */}
            {isM4b && (
              <Button
                className="mt-2"
                onClick={handleAddChapterM4B}
                type="button"
                variant="outline"
              >
                Add Chapter
              </Button>
            )}
          </div>
        );
      }

      return (
        <div className="py-8 text-center">
          <p className="text-muted-foreground">No chapters</p>
          {canAddChapters && (
            <button
              className="mt-4 px-4 py-2 text-sm bg-primary text-primary-foreground rounded-md hover:bg-primary/90 cursor-pointer"
              onClick={handleAddChapterFromEmpty}
              type="button"
            >
              Add Chapter
            </button>
          )}
        </div>
      );
    }

    // Check for uncovered pages (first chapter starts after page 0)
    const isPageBased =
      file.file_type === FileTypeCBZ || file.file_type === FileTypePDF;
    const firstChapterStartPage =
      isPageBased && chapters.length > 0 ? chapters[0].start_page : null;
    const hasUncoveredPages =
      firstChapterStartPage != null && firstChapterStartPage > 0;

    // Edit mode rendering
    if (isEditing) {
      const isEpub = file.file_type === FileTypeEPUB;
      const isM4b = file.file_type === FileTypeM4B;
      const maxDurationMs = file.audiobook_duration_seconds
        ? file.audiobook_duration_seconds * 1000
        : undefined;

      return (
        <div className="py-4">
          {/* EPUB limitation notice */}
          {isEpub && (
            <div className="mb-4 p-3 rounded-md bg-muted border border-border text-sm text-muted-foreground">
              Currently, only renaming and deleting existing chapters is
              supported for EPUB files. Adding new chapters and reordering
              chapters is on the roadmap.
            </div>
          )}

          {/* Hidden audio element for M4B playback */}
          {isM4b && (
            <audio ref={audioRef} src={`/api/books/files/${file.id}/stream`} />
          )}

          {editedChapters.map((chapter, index) => (
            <ChapterRow
              chapter={chapter as unknown as Chapter}
              chapterIndex={isM4b ? index : undefined}
              depth={0}
              fileId={file.id}
              fileType={file.file_type}
              isEditing={true}
              key={chapter._editKey}
              maxDurationMs={isM4b ? maxDurationMs : undefined}
              onBlur={
                isPageBased
                  ? handleBlurReorder
                  : isM4b
                    ? handleTimestampBlurReorder
                    : undefined
              }
              onChildDelete={
                isEpub ? createChildDeleteCallback(index) : undefined
              }
              onChildTitleChange={
                isEpub ? createChildTitleChangeCallback(index) : undefined
              }
              onDelete={() =>
                setEditedChapters((prev) => deleteChapter(prev, index))
              }
              onPlay={isM4b ? handleChapterPlay : undefined}
              onStartPageChange={
                isPageBased
                  ? (page) =>
                      setEditedChapters((prev) =>
                        updateChapterStartPage(prev, index, page),
                      )
                  : undefined
              }
              onStartTimestampChange={
                isM4b
                  ? (ms) =>
                      setEditedChapters((prev) =>
                        updateChapterStartTimestamp(prev, index, ms),
                      )
                  : undefined
              }
              onStop={isM4b ? handleAudioStop : undefined}
              onTitleChange={(title) =>
                setEditedChapters((prev) =>
                  updateChapterTitle(prev, index, title),
                )
              }
              onValidationChange={
                isM4b
                  ? (_chapterId, hasError) =>
                      handleValidationChange(index, hasError)
                  : undefined
              }
              pageCount={isPageBased ? (file.page_count ?? 0) : undefined}
              playingChapterIndex={isM4b ? playingChapterIndex : undefined}
            />
          ))}

          {/* Add Chapter button for page-based files */}
          {isPageBased && (
            <Button
              className="mt-2"
              onClick={handleAddChapter}
              type="button"
              variant="outline"
            >
              Add Chapter
            </Button>
          )}

          {/* Add Chapter button for M4B */}
          {isM4b && (
            <Button
              className="mt-2"
              onClick={handleAddChapterM4B}
              type="button"
              variant="outline"
            >
              Add Chapter
            </Button>
          )}
        </div>
      );
    }

    const isM4bFile = file.file_type === FileTypeM4B;

    // Chapter list (view mode)
    return (
      <div className="py-4">
        {/* Hidden audio element for M4B playback */}
        {isM4bFile && (
          <audio ref={audioRef} src={`/api/books/files/${file.id}/stream`} />
        )}

        {/* Uncovered pages warning (display uses 1-indexed page numbers) */}
        {hasUncoveredPages && (
          <button
            className="w-full flex items-center gap-3 py-2 px-3 mb-2 border border-amber-500/50 bg-amber-500/10 rounded-md text-left hover:bg-amber-500/20 transition-colors cursor-pointer"
            onClick={handleAddChapterAtPageZero}
            type="button"
          >
            <img
              alt="Page 1"
              className="h-[60px] w-auto rounded border border-border object-contain bg-muted"
              src={`/api/books/files/${file.id}/page/0`}
            />
            <div className="flex-1 min-w-0">
              <span className="text-amber-600 dark:text-amber-400 font-medium">
                Pages 1-{firstChapterStartPage} not in any chapter
              </span>
              <p className="text-muted-foreground text-sm mt-0.5">
                Click to add chapter
              </p>
            </div>
          </button>
        )}

        {chapters.map((chapter, index) => (
          <ChapterRow
            bookId={file.book_id}
            chapter={chapter}
            chapterIndex={isM4bFile ? index : undefined}
            depth={0}
            fileId={file.id}
            fileType={file.file_type}
            isEditing={false}
            key={chapter.id}
            libraryId={file.library_id}
            onPlay={isM4bFile ? handleChapterPlay : undefined}
            onStop={isM4bFile ? handleAudioStop : undefined}
            playingChapterIndex={isM4bFile ? playingChapterIndex : undefined}
          />
        ))}
      </div>
    );
  },
);

FileChaptersTab.displayName = "FileChaptersTab";

export default FileChaptersTab;
