import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { formatExactWordCount, formatWordCount } from "@/utils/format";

/**
 * A word count in the ballpark, "29K words", with the exact count in a
 * tooltip once it's rounded. Like LastRead, the count isn't a control, so a
 * screen reader gets the exact count as text instead, and only that.
 */
const WordCount = ({ count }: { count: number }) => {
  const shown = formatWordCount(count);
  const exact = formatExactWordCount(count);
  if (shown === exact) return <span>{shown}</span>;
  return (
    <>
      <Tooltip delayDuration={300}>
        <TooltipTrigger asChild>
          <span aria-hidden="true">{shown}</span>
        </TooltipTrigger>
        <TooltipContent>{exact}</TooltipContent>
      </Tooltip>
      <span className="sr-only">{exact}</span>
    </>
  );
};

export default WordCount;
