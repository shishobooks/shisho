import { cn } from "@/libraries/utils";
import { formatProgress } from "@/utils/readingStatus";

const RADIUS = 15;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;

interface ProgressRingProps {
  // How far through, from 0 to 1.
  progress: number;
  className?: string;
}

// How far through a File the reader is: a circle filling clockwise from the top,
// with the percentage inside.
const ProgressRing = ({ progress, className }: ProgressRingProps) => {
  const percent = formatProgress(progress);
  const label = `${percent} through`;
  return (
    <div
      aria-label={label}
      aria-valuemax={100}
      aria-valuemin={0}
      aria-valuenow={Math.round(progress * 100)}
      className={cn("relative h-9 w-9 shrink-0", className)}
      role="progressbar"
      title={label}
    >
      <svg aria-hidden className="h-full w-full -rotate-90" viewBox="0 0 36 36">
        <circle
          className="stroke-muted"
          cx="18"
          cy="18"
          fill="none"
          r={RADIUS}
          strokeWidth="3"
        />
        <circle
          className="stroke-primary"
          cx="18"
          cy="18"
          fill="none"
          r={RADIUS}
          strokeDasharray={CIRCUMFERENCE}
          strokeDashoffset={CIRCUMFERENCE * (1 - Math.min(progress, 1))}
          strokeLinecap="round"
          strokeWidth="3"
        />
      </svg>
      <span
        aria-hidden
        className="absolute inset-0 flex items-center justify-center text-[9px] font-semibold tabular-nums"
      >
        {percent}
      </span>
    </div>
  );
};

export default ProgressRing;
