import {
  BookOpen,
  Check,
  Clock,
  Pause,
  X,
  type LucideIcon,
} from "lucide-react";

import { cn } from "@/libraries/utils";
import type { ReadingStatus } from "@/types";
import { formatProgress, readingStatusLabel } from "@/utils/readingStatus";

const LOOKS: Record<ReadingStatus, { icon: LucideIcon; color: string }> = {
  currently_reading: { icon: BookOpen, color: "bg-violet-600" },
  paused: { icon: Pause, color: "bg-amber-500" },
  to_read: { icon: Clock, color: "bg-sky-500" },
  read: { icon: Check, color: "bg-emerald-600" },
  did_not_finish: { icon: X, color: "bg-rose-600" },
};

interface ReadingStatusBookmarkProps {
  status: ReadingStatus;
  // How far through the Book the user is, from 0 to 1, which a book being read
  // shows in place of its icon.
  progress?: number | null;
  className?: string;
}

// A ribbon bookmark hanging from a cover's top edge, colored and marked by
// Reading Status, or by progress for a book being read. It's a shape rather than a text badge so it can't be
// mistaken for the Needs review badge. The title names it on hover.
const ReadingStatusBookmark = ({
  status,
  progress,
  className,
}: ReadingStatusBookmarkProps) => {
  const { icon: Icon, color } = LOOKS[status];
  const percent =
    status === "currently_reading" && progress != null
      ? formatProgress(progress)
      : null;
  const label = percent
    ? `${readingStatusLabel(status)}, ${percent} through`
    : readingStatusLabel(status);
  return (
    <div
      aria-label={label}
      className={cn("drop-shadow-md", className)}
      data-status={status}
      role="img"
      title={label}
    >
      <div
        className={cn(
          "flex h-8 justify-center pt-1.5 [clip-path:polygon(0_0,100%_0,100%_100%,50%_78%,0_100%)]",
          percent ? "w-7" : "w-6",
          color,
        )}
      >
        {percent ? (
          <span className="text-[9px] leading-[14px] font-bold tabular-nums text-white">
            {percent}
          </span>
        ) : (
          <Icon
            aria-hidden
            className="h-3.5 w-3.5 text-white"
            strokeWidth={3}
          />
        )}
      </div>
    </div>
  );
};

export default ReadingStatusBookmark;
