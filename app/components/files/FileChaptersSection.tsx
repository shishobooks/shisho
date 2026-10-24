import { Download, Loader2, Pencil, Plus } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";

import FileChaptersEditor, {
  type ChaptersActionState,
  type FileChaptersEditorHandle,
} from "@/components/files/FileChaptersEditor";
import { Button } from "@/components/ui/button";
import { useFileChapters } from "@/hooks/queries/chapters";
import { SCROLL_CLEAR_OF_NAV } from "@/hooks/useSectionPaging";
import { FileRoleSupplement, FileTypeM4B, type File } from "@/types";
import { canAddChapters } from "@/utils/fileTypes";
import { formatCount } from "@/utils/format";

interface FileChaptersSectionProps {
  file: File;
  canEdit: boolean;
  /** Reports the editor's state, so the page's unsaved-changes guard sees it. */
  onActionStateChange: (state: ChaptersActionState) => void;
  /**
   * Scrolls the section into view once its chapters load, for an old link
   * to the page's former Chapters tab. Before they load the page is too
   * short to scroll that far.
   */
  scrollIntoViewOnLoad?: boolean;
  /** Called once it has, so the next file's section does not scroll too. */
  onScrolledIntoView?: () => void;
}

/**
 * The file's chapters in a window about ten rows tall, so a 3,000-chapter
 * EPUB neither buries the page below it nor does so while being edited. One
 * bar above the window holds every chapter action: Fetch from Audible and
 * Edit chapters, then while editing Add chapter, Fetch from Audible,
 * Cancel, and Save. Fetch from Audible enters edit mode and starts the
 * Audible lookup in one click.
 */
const FileChaptersSection = ({
  file,
  canEdit,
  onActionStateChange,
  scrollIntoViewOnLoad = false,
  onScrolledIntoView,
}: FileChaptersSectionProps) => {
  const count = useFileChapters(file.id).data?.length;
  const editorRef = useRef<FileChaptersEditorHandle>(null);
  const sectionRef = useRef<HTMLElement>(null);
  const windowRef = useRef<HTMLDivElement>(null);
  const [editing, setEditing] = useState(false);
  const [state, setState] = useState<ChaptersActionState>({
    isSaving: false,
    canSave: false,
    hasChanges: false,
  });

  const loaded = count !== undefined;
  useEffect(() => {
    if (!scrollIntoViewOnLoad || !loaded) return;
    requestAnimationFrame(() =>
      sectionRef.current?.scrollIntoView({ block: "start" }),
    );
    onScrolledIntoView?.();
  }, [scrollIntoViewOnLoad, loaded, onScrolledIntoView]);

  // Stable: the editor reports from an effect that depends on it.
  const reportState = useCallback(
    (next: ChaptersActionState) => {
      setState(next);
      onActionStateChange(next);
    },
    [onActionStateChange],
  );

  // Switching modes swaps every row, and edit rows are taller, so the window
  // would lose its place and briefly collapse, pulling the page up with it.
  // Remember the chapter at the window's top, hold the window's height, and
  // bring that chapter back as the new rows arrive (the editor fills them
  // from an effect, and a save refetches them). It lets go after a moment,
  // or as soon as the reader scrolls or adds a chapter.
  const stopAnchoring = useRef<(() => void) | null>(null);
  useEffect(() => () => stopAnchoring.current?.(), []);

  const switchEditing = useCallback((next: boolean) => {
    stopAnchoring.current?.();
    const el = windowRef.current;
    if (el) {
      const rows = [...el.querySelectorAll<HTMLElement>("[data-chapter-row]")];
      const anchor = rows.findIndex(
        (row) => row.offsetTop + row.offsetHeight > el.scrollTop,
      );
      const offset = anchor >= 0 ? el.scrollTop - rows[anchor].offsetTop : 0;
      el.style.minHeight = `${el.offsetHeight}px`;
      const restore = () => {
        const now = el.querySelectorAll<HTMLElement>("[data-chapter-row]");
        if (anchor >= 0 && now[anchor]) {
          el.scrollTop = now[anchor].offsetTop + offset;
        }
      };
      const observer = new MutationObserver(restore);
      observer.observe(el, { childList: true, subtree: true });
      const frame = requestAnimationFrame(restore);
      const events = ["wheel", "touchstart", "keydown"] as const;
      const stop = () => {
        observer.disconnect();
        cancelAnimationFrame(frame);
        window.clearTimeout(timer);
        for (const event of events) el.removeEventListener(event, stop);
        el.style.minHeight = "";
        stopAnchoring.current = null;
      };
      const timer = window.setTimeout(stop, 1500);
      for (const event of events) el.addEventListener(event, stop);
      stopAnchoring.current = stop;
    }
    setEditing(next);
  }, []);

  const fetchFromAudible = () => {
    switchEditing(true);
    editorRef.current?.fetchFromAudible();
  };
  const addChapter = () => {
    stopAnchoring.current?.();
    editorRef.current?.addChapter();
    // The new chapter lands at the end; bring it into view.
    requestAnimationFrame(() => {
      const el = windowRef.current;
      if (el) el.scrollTop = el.scrollHeight;
    });
  };

  // A supplement rarely has chapters; it shows the section only when it does.
  if (file.file_role === FileRoleSupplement && !count && !editing) return null;

  const audio = file.file_type === FileTypeM4B;
  const canEditChapters = (count ?? 0) > 0 || canAddChapters(file.file_type);

  return (
    <section
      aria-labelledby="file-chapters-heading"
      className={SCROLL_CLEAR_OF_NAV}
      ref={sectionRef}
    >
      <div className="mb-3 flex min-h-8 flex-wrap items-center justify-between gap-2">
        <h2
          className="text-sm text-muted-foreground"
          id="file-chapters-heading"
        >
          {editing
            ? "Editing chapters"
            : count
              ? formatCount(count, "chapter")
              : "Chapters"}
        </h2>
        {canEdit && (
          <div className="flex flex-wrap items-center gap-2">
            {editing && canAddChapters(file.file_type) && (
              <Button onClick={addChapter} size="sm" variant="outline">
                <Plus aria-hidden="true" className="h-3.5 w-3.5" />
                Add chapter
              </Button>
            )}
            {audio && (
              <Button onClick={fetchFromAudible} size="sm" variant="outline">
                <Download aria-hidden="true" className="h-3.5 w-3.5" />
                Fetch from Audible
              </Button>
            )}
            {editing ? (
              <>
                <Button
                  disabled={state.isSaving}
                  onClick={() => editorRef.current?.cancel()}
                  size="sm"
                  variant="outline"
                >
                  Cancel
                </Button>
                <Button
                  disabled={!state.canSave || state.isSaving}
                  onClick={() => editorRef.current?.save()}
                  size="sm"
                >
                  {state.isSaving && (
                    <Loader2
                      aria-hidden="true"
                      className="h-3.5 w-3.5 animate-spin"
                    />
                  )}
                  Save
                </Button>
              </>
            ) : (
              canEditChapters && (
                <Button
                  onClick={() => switchEditing(true)}
                  size="sm"
                  variant="outline"
                >
                  <Pencil aria-hidden="true" className="h-3.5 w-3.5" />
                  Edit chapters
                </Button>
              )
            )}
          </div>
        )}
      </div>
      <div
        className="relative max-h-[460px] overflow-y-auto rounded-md border border-border px-3 py-1"
        ref={windowRef}
      >
        <FileChaptersEditor
          canEdit={canEdit}
          file={file}
          isEditing={editing}
          onActionStateChange={reportState}
          onEditingChange={switchEditing}
          ref={editorRef}
        />
      </div>
    </section>
  );
};

export default FileChaptersSection;
