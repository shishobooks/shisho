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
import { readingStatusLabel } from "@/utils/readingStatus";

const LOOKS: Record<ReadingStatus, { icon: LucideIcon; color: string }> = {
  currently_reading: { icon: BookOpen, color: "bg-violet-600" },
  paused: { icon: Pause, color: "bg-amber-500" },
  to_read: { icon: Clock, color: "bg-sky-500" },
  read: { icon: Check, color: "bg-emerald-600" },
  did_not_finish: { icon: X, color: "bg-rose-600" },
};

interface ReadingStatusBookmarkProps {
  status: ReadingStatus;
  className?: string;
}

// A ribbon bookmark hanging from a cover's top edge, colored and marked by
// Reading Status. It's a shape rather than a text badge so it can't be
// mistaken for the Needs review badge. The title names it on hover.
const ReadingStatusBookmark = ({
  status,
  className,
}: ReadingStatusBookmarkProps) => {
  const { icon: Icon, color } = LOOKS[status];
  const label = readingStatusLabel(status);
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
          "flex h-8 w-6 justify-center pt-1.5 [clip-path:polygon(0_0,100%_0,100%_100%,50%_78%,0_100%)]",
          color,
        )}
      >
        <Icon aria-hidden className="h-3.5 w-3.5 text-white" strokeWidth={3} />
      </div>
    </div>
  );
};

export default ReadingStatusBookmark;
