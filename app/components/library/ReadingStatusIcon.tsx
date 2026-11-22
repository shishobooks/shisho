import {
  BookOpen,
  Check,
  Clock,
  Library,
  Pause,
  X,
  type LucideIcon,
  type LucideProps,
} from "lucide-react";

import type { ReadingStatus } from "@/types";

const ICONS: Record<ReadingStatus, LucideIcon> = {
  currently_reading: BookOpen,
  paused: Pause,
  to_read: Clock,
  read: Check,
  did_not_finish: X,
  reference: Library,
};

interface ReadingStatusIconProps extends LucideProps {
  // Null is a book with no status, which shows the open book of Track
  // reading.
  status: ReadingStatus | null | undefined;
}

// The icon that marks a Reading Status wherever it shows, on a cover's
// bookmark and on the status button alike.
const ReadingStatusIcon = ({ status, ...props }: ReadingStatusIconProps) => {
  const Icon = status ? ICONS[status] : BookOpen;
  return <Icon {...props} />;
};

export default ReadingStatusIcon;
