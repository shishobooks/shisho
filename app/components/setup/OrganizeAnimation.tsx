import {
  ArrowDown,
  ArrowRight,
  FileText,
  Folder,
  Headphones,
} from "lucide-react";
import { useId, type CSSProperties } from "react";

import { useMediaQuery } from "@/hooks/useMediaQuery";
import { cn } from "@/libraries/utils";

// The example is canned: three books in a messy folder, and where organizing
// puts them. Shisho names a book's folder "[Author] Title" and its files
// "Title.ext" (pkg/fileutils/naming.go).
const ROW = 28; // px, the height of one row
const INDENT = 18; // px per folder level
// Tailwind reads class names from the source, so each animation is spelled
// out in full. Every one runs on the same 10s loop.
const FOLDER_OUT = "animate-[setup-folder-out_10s_ease-in-out_infinite_both]";
const FOLDER_IN = "animate-[setup-folder-in_10s_ease-out_infinite_both]";
const FILE_MOVE =
  "animate-[setup-file-move_10s_cubic-bezier(0.65,0,0.35,1)_infinite_both]";
const NAME_BEFORE = "animate-[setup-name-before_10s_linear_infinite_both]";
const NAME_AFTER = "animate-[setup-name-after_10s_linear_infinite_both]";

type Kind = "folder" | "ebook" | "audiobook";

interface MovingFile {
  kind: Kind;
  before: { name: string; level: number; row: number };
  after: { name: string; level: number; row: number };
}

const FILES: MovingFile[] = [
  {
    kind: "ebook",
    before: { name: "pride_and_prejudice_FINAL.epub", level: 1, row: 1 },
    after: { name: "Pride and Prejudice.epub", level: 2, row: 2 },
  },
  {
    kind: "ebook",
    before: { name: "frankenstein (1).epub", level: 1, row: 2 },
    after: { name: "Frankenstein.epub", level: 2, row: 4 },
  },
  {
    kind: "audiobook",
    before: { name: "TimeMachine-audiobook.m4b", level: 2, row: 4 },
    after: { name: "The Time Machine.m4b", level: 2, row: 6 },
  },
];

// Folders only the messy side has, and those organizing creates.
const MESSY_FOLDERS = [{ name: "misc", level: 1, row: 3 }];
const BOOK_FOLDERS = [
  { name: "[Jane Austen] Pride and Prejudice", level: 1, row: 1 },
  { name: "[Mary Shelley] Frankenstein", level: 1, row: 3 },
  { name: "[H. G. Wells] The Time Machine", level: 1, row: 5 },
];
const ROOT = "books";
const ROWS = 7;

const ICONS = { folder: Folder, ebook: FileText, audiobook: Headphones };

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
      <span className={cn(kind === "folder" && "font-medium")}>
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

const Animated = () => {
  // Rows that start together move a beat apart, so they read as files moving
  // one after another rather than the list jumping.
  const stagger = (i: number) => `${i * 0.25}s`;
  return (
    <div
      aria-label="Example: files in a messy folder move into a folder per book, named for its author and title."
      className="relative overflow-hidden font-mono text-xs text-foreground"
      role="img"
      style={{ height: ROWS * ROW }}
    >
      <div aria-hidden="true">
        <div className={rowClass}>
          <Entry kind="folder" name={ROOT} />
        </div>
        {MESSY_FOLDERS.map((folder) => {
          const { x, y } = at(folder.level, folder.row);
          return (
            <div
              className={cn(rowClass, FOLDER_OUT)}
              key={folder.name}
              style={{ transform: `translate(${x}, ${y})` }}
            >
              <Entry kind="folder" name={folder.name} />
            </div>
          );
        })}
        {BOOK_FOLDERS.map((folder, i) => {
          const { x, y } = at(folder.level, folder.row);
          return (
            <div
              className={cn(rowClass, FOLDER_IN)}
              key={folder.name}
              style={
                {
                  "--x": x,
                  "--y": y,
                  animationDelay: stagger(i),
                } as CSSProperties
              }
            >
              <Entry kind="folder" name={folder.name} />
            </div>
          );
        })}
        {FILES.map((file, i) => {
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

const BEFORE: StillRow[] = [
  { kind: "folder", name: ROOT, level: 0 },
  { kind: "ebook", name: FILES[0].before.name, level: 1 },
  { kind: "ebook", name: FILES[1].before.name, level: 1 },
  { kind: "folder", name: MESSY_FOLDERS[0].name, level: 1 },
  { kind: "audiobook", name: FILES[2].before.name, level: 2 },
];

const AFTER: StillRow[] = [
  { kind: "folder", name: ROOT, level: 0 },
  ...BOOK_FOLDERS.flatMap((folder, i): StillRow[] => [
    { kind: "folder", name: folder.name, level: 1 },
    { kind: FILES[i].kind, name: FILES[i].after.name, level: 2 },
  ]),
];

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

const Still = () => (
  <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] sm:items-center">
    <StillTree label="Before" rows={BEFORE} />
    <ArrowRight
      aria-hidden="true"
      className="hidden h-4 w-4 text-muted-foreground sm:block"
    />
    <ArrowDown
      aria-hidden="true"
      className="h-4 w-4 justify-self-center text-muted-foreground sm:hidden"
    />
    <StillTree label="After" rows={AFTER} />
  </div>
);

/**
 * Shows what organizing does, with example files rather than the admin's own:
 * a loop of a messy folder rearranging into a folder per book. With reduced
 * motion it is a still before and after.
 */
export const OrganizeAnimation = ({ className }: { className?: string }) => {
  const reducedMotion = useMediaQuery("(prefers-reduced-motion: reduce)");
  return (
    <div
      className={cn(
        "overflow-x-auto rounded-md border border-border bg-muted/40 p-3 sm:p-4",
        className,
      )}
    >
      {reducedMotion ? <Still /> : <Animated />}
    </div>
  );
};
