import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { formatDateTime } from "@/utils/format";
import { formatLastRead } from "@/utils/readingProgress";

interface LastReadProps {
  lastReadAt: string;
  className?: string;
}

// "Last read 5 days ago", with the full date and time in a tooltip. The line
// isn't a control, so a screen reader gets the full time as text instead; it
// takes no tab stop, which on a card in selection mode is the overlay's.
const LastRead = ({ lastReadAt, className }: LastReadProps) => {
  const full = formatDateTime(lastReadAt);
  return (
    <>
      <Tooltip delayDuration={300}>
        <TooltipTrigger asChild>
          <time className={className} dateTime={lastReadAt}>
            {formatLastRead(lastReadAt)}
          </time>
        </TooltipTrigger>
        <TooltipContent>{full}</TooltipContent>
      </Tooltip>
      <span className="sr-only">{full}</span>
    </>
  );
};

export default LastRead;
