import {
  ArrowDown,
  ArrowRight,
  BookImage,
  FileText,
  Folder,
  Headphones,
  RotateCcw,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type CSSProperties,
} from "react";

import { Button } from "@/components/ui/button";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { cn } from "@/libraries/utils";

import organizeExample from "./organizeExample.json";

// The example is canned: a few books in a messy folder, and where organizing
// puts them. Their organized names come from organizeExample.json, which a Go
// test (pkg/fileutils/organize_example_test.go) checks against the naming
// functions a scan uses, so a change to the naming can't leave it behind.
const ROW = 28; // px, the height of one row
const INDENT = 18; // px per folder level
// Tailwind reads class names from the source, so each animation is spelled
// out in full. Every one plays once over the same 3s and holds its end.
const FOLDER_OUT = "animate-[setup-folder-out_3s_ease-in-out_both]";
const FOLDER_IN = "animate-[setup-folder-in_3s_ease-out_both]";
const FILE_MOVE =
  "animate-[setup-file-move_3s_cubic-bezier(0.65,0,0.35,1)_both]";
const NAME_BEFORE = "animate-[setup-name-before_3s_linear_both]";
const NAME_AFTER = "animate-[setup-name-after_3s_linear_both]";
// Holds every animation at its start until the example scrolls into view.
const PAUSED = "[&_*]:[animation-play-state:paused]";
// Rows that start together move a beat apart, so they read as files moving
// one after another rather than the list jumping.
const STAGGER_S = 0.15;
// How long the messy folder shows before anything moves, on every play, so
// the admin sees what it looked like first.
const START_DELAY_S = 2;

type FileKind = "ebook" | "audiobook" | "comic";
type Kind = "folder" | FileKind;

interface Place {
  name: string;
  level: number;
  row: number;
}

interface MovingFile {
  kind: FileKind;
  before: Place;
  after: Place;
  /** Its place in the order files move, for the stagger. */
  index: number;
}

interface Layout {
  root: string;
  /** Folders only the messy side has. */
  messyFolders: Place[];
  /** A folder per book, with the index of its first file. */
  bookFolders: (Place & { firstFile: number })[];
  files: MovingFile[];
  rows: number;
}

// Lays the books out as two trees in the same order. In the messy one a
// book's files sit loose in the root or together in a folder of their own,
// which is what makes them one book; in the organized one each book has its
// folder, named for its author and title.
const layOut = (example: typeof organizeExample): Layout => {
  const layout: Layout = {
    root: example.root,
    messyFolders: [],
    bookFolders: [],
    files: [],
    rows: 0,
  };
  let messyRow = 1;
  let organizedRow = 1;
  for (const book of example.books) {
    let level = 1;
    if (book.messyFolder) {
      layout.messyFolders.push({
        name: book.messyFolder,
        level,
        row: messyRow++,
      });
      level = 2;
    }
    layout.bookFolders.push({
      name: book.organizedFolder,
      level: 1,
      row: organizedRow++,
      firstFile: layout.files.length,
    });
    for (const file of book.files) {
      layout.files.push({
        kind: file.type as FileKind,
        before: { name: file.messyName, level, row: messyRow++ },
        after: { name: file.organizedName, level: 2, row: organizedRow++ },
        index: layout.files.length,
      });
    }
  }
  layout.rows = Math.max(messyRow, organizedRow);
  return layout;
};

const LAYOUT = layOut(organizeExample);

const ICONS = {
  folder: Folder,
  ebook: FileText,
  audiobook: Headphones,
  comic: BookImage,
};

const Entry = ({ kind, name }: { kind: Kind; name: string }) => {
  const Icon = ICONS[kind];
  return (
    <>
      <Icon
        aria-hidden="true"
        className={cn(
          "h-3.5 w-3.5 shrink-0",
          kind === "folder" ? "text-primary" : "text-muted-foreground",
        )}
      />
      <span
        className={cn("whitespace-nowrap", kind === "folder" && "font-medium")}
      >
        {kind === "folder" ? `${name}/` : name}
      </span>
    </>
  );
};

const rowClass =
  "absolute left-0 top-0 flex h-7 items-center gap-1.5 whitespace-nowrap";

const at = (level: number, row: number) => ({
  x: `${level * INDENT}px`,
  y: `${row * ROW}px`,
});

interface AnimatedProps {
  playing: boolean;
  /** Called when the last file has its organized name. */
  onDone: () => void;
}

const Animated = ({ playing, onDone }: AnimatedProps) => {
  // A native listener, which hears the event wherever it comes from.
  const lastStepRef = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    const el = lastStepRef.current;
    if (!el) return;
    el.addEventListener("animationend", onDone);
    return () => el.removeEventListener("animationend", onDone);
  }, [onDone]);
  const stagger = (i: number) => `${START_DELAY_S + i * STAGGER_S}s`;
  const lastFile = LAYOUT.files.length - 1;
  return (
    <div
      aria-label="Example: files in a messy folder move into a folder per book, named for its author and title."
      className={cn(
        "relative overflow-hidden font-mono text-xs text-foreground",
        !playing && PAUSED,
      )}
      role="img"
      style={{ height: LAYOUT.rows * ROW }}
    >
      <div aria-hidden="true">
        <div className={rowClass}>
          <Entry kind="folder" name={LAYOUT.root} />
        </div>
        {LAYOUT.messyFolders.map((folder) => {
          const { x, y } = at(folder.level, folder.row);
          return (
            <div
              className={cn(rowClass, FOLDER_OUT)}
              key={folder.name}
              style={{
                transform: `translate(${x}, ${y})`,
                animationDelay: stagger(0),
              }}
            >
              <Entry kind="folder" name={folder.name} />
            </div>
          );
        })}
        {LAYOUT.bookFolders.map((folder) => {
          const { x, y } = at(folder.level, folder.row);
          return (
            <div
              className={cn(rowClass, FOLDER_IN)}
              key={folder.name}
              style={
                {
                  "--x": x,
                  "--y": y,
                  // A book's folder waits for its first file to leave the row.
                  animationDelay: stagger(folder.firstFile),
                } as CSSProperties
              }
            >
              <Entry kind="folder" name={folder.name} />
            </div>
          );
        })}
        {LAYOUT.files.map((file) => {
          const i = file.index;
          const from = at(file.before.level, file.before.row);
          const to = at(file.after.level, file.after.row);
          return (
            <div
              className={cn(rowClass, FILE_MOVE)}
              key={file.after.name}
              style={
                {
                  "--from-x": from.x,
                  "--from-y": from.y,
                  "--to-x": to.x,
                  "--to-y": to.y,
                  animationDelay: stagger(i),
                } as CSSProperties
              }
            >
              <span className="relative flex items-center gap-1.5">
                <span
                  className={cn("flex items-center gap-1.5", NAME_BEFORE)}
                  style={{ animationDelay: stagger(i) }}
                >
                  <Entry kind={file.kind} name={file.before.name} />
                </span>
                <span
                  className={cn(
                    "absolute left-0 top-0 flex items-center gap-1.5",
                    NAME_AFTER,
                  )}
                  // The last file's new name is the example's final step.
                  data-organize-last-step={i === lastFile ? "" : undefined}
                  ref={i === lastFile ? lastStepRef : undefined}
                  style={{ animationDelay: stagger(i) }}
                >
                  <Entry kind={file.kind} name={file.after.name} />
                </span>
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
};

interface StillRow {
  kind: Kind;
  name: string;
  level: number;
}

// The still is the two trees the animation moves between.
const stillRows = (side: "before" | "after"): StillRow[] => {
  const places: (StillRow & { row: number })[] = [
    { kind: "folder", name: LAYOUT.root, level: 0, row: 0 },
    ...LAYOUT.files.map((file) => ({ kind: file.kind, ...file[side] })),
    ...(side === "before" ? LAYOUT.messyFolders : LAYOUT.bookFolders).map(
      (folder) => ({
        kind: "folder" as const,
        name: folder.name,
        level: folder.level,
        row: folder.row,
      }),
    ),
  ];
  return places.sort((a, b) => a.row - b.row);
};

const BEFORE = stillRows("before");
const AFTER = stillRows("after");

const StillTree = ({ label, rows }: { label: string; rows: StillRow[] }) => {
  const captionId = useId();
  return (
    <figure aria-labelledby={captionId} className="min-w-0">
      <figcaption
        className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground"
        id={captionId}
      >
        {label}
      </figcaption>
      <ul className="font-mono text-xs text-foreground">
        {rows.map((row) => (
          <li
            className="flex h-7 min-w-0 items-center gap-1.5"
            key={`${row.level}-${row.name}`}
            style={{ paddingLeft: row.level * INDENT }}
          >
            <Entry kind={row.kind} name={row.name} />
          </li>
        ))}
      </ul>
    </figure>
  );
};

// Side by side only where both trees fit unwrapped; names never wrap, and
// the frame scrolls rather than squeeze them. The trees share a top edge so
// their rows line up.
const Still = () => (
  <div className="grid gap-3 md:grid-cols-[auto_auto_auto] md:items-start md:justify-between">
    <StillTree label="Before" rows={BEFORE} />
    <ArrowRight
      aria-hidden="true"
      className="hidden h-4 w-4 self-center text-muted-foreground md:block"
    />
    <ArrowDown
      aria-hidden="true"
      className="h-4 w-4 justify-self-center text-muted-foreground md:hidden"
    />
    <StillTree label="After" rows={AFTER} />
  </div>
);

/**
 * Plays the example once the frame is mostly in view, so on a phone it isn't
 * over before the admin scrolls to it. Without IntersectionObserver it plays
 * at once.
 */
const usePlayWhenSeen = (ref: React.RefObject<HTMLElement | null>) => {
  const [seen, setSeen] = useState(
    () => typeof IntersectionObserver === "undefined",
  );
  useEffect(() => {
    const el = ref.current;
    if (seen || !el) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting) setSeen(true);
      },
      { threshold: 0.6 },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [ref, seen]);
  return seen;
};

/**
 * Shows what organizing does, with example files rather than the admin's own:
 * a messy folder rearranging into a folder per book. It plays once and holds
 * the result, with a button to play it again. With reduced motion it is a
 * still before and after.
 */
export const OrganizeAnimation = ({ className }: { className?: string }) => {
  const reducedMotion = useMediaQuery("(prefers-reduced-motion: reduce)");
  const frameRef = useRef<HTMLDivElement>(null);
  const seen = usePlayWhenSeen(frameRef);
  const [run, setRun] = useState(0);
  const [done, setDone] = useState(false);

  const markDone = useCallback(() => setDone(true), []);
  const replay = () => {
    setDone(false);
    // A new key mounts the rows afresh, which starts every animation over.
    setRun((n) => n + 1);
  };

  return (
    <div
      className={cn(
        "relative overflow-x-auto rounded-md border border-border bg-muted/40 p-3 sm:p-4",
        className,
      )}
      ref={frameRef}
    >
      {reducedMotion ? (
        <Still />
      ) : (
        <>
          <Animated key={run} onDone={markDone} playing={seen} />
          {done && (
            <Button
              className="absolute right-2 top-2 text-muted-foreground"
              onClick={replay}
              size="sm"
              variant="ghost"
            >
              Replay
              <RotateCcw aria-hidden="true" />
            </Button>
          )}
        </>
      )}
    </div>
  );
};
