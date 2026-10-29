import * as SliderPrimitive from "@radix-ui/react-slider";
import { Star } from "lucide-react";
import { useEffect, useState } from "react";

import { cn } from "@/libraries/utils";
import { formatCount } from "@/utils/format";
import { formatRating } from "@/utils/readingStatus";

const STARS = [0, 1, 2, 3, 4];

// The lowest Rating. Zero stars doesn't exist: an unrated Book has none.
const MIN_RATING = 0.25;
const MAX_RATING = 5;

// The quarter star a pointer at fraction of the way across picks. Radix
// snaps a click to the nearest step, so the hover preview rounds the same
// way and shows exactly what a click would save.
const starsAt = (fraction: number): number =>
  Math.min(
    MAX_RATING,
    Math.max(MIN_RATING, Math.round(fraction * MAX_RATING * 4) / 4),
  );

interface StarRatingProps {
  /** Stars from 0.25 to 5 in quarter steps, or null when unrated. */
  value: number | null;
  /** Called once the user settles on a value, not on every drag step. */
  onChange: (stars: number) => void;
  disabled?: boolean;
}

// A five-star row that rates in quarter stars, with the number beside it.
// It's a slider underneath, so a pointer picks the quarter under it and the
// arrow keys step by a quarter. Hovering previews what a click would set.
const StarRating = ({ value, onChange, disabled }: StarRatingProps) => {
  // The value shown while dragging, before it's committed.
  const [preview, setPreview] = useState<number | null>(value);
  useEffect(() => setPreview(value), [value]);
  // The value under the pointer, before any click.
  const [hover, setHover] = useState<number | null>(null);

  const shown = hover ?? preview;
  const fill = shown ?? 0;
  return (
    <div className="flex items-center gap-2">
      <SliderPrimitive.Root
        className={cn(
          "relative flex h-6 w-[120px] touch-none select-none items-center",
          disabled ? "opacity-50" : "cursor-pointer",
        )}
        data-testid="star-rating"
        disabled={disabled}
        max={MAX_RATING}
        min={0}
        onPointerLeave={() => setHover(null)}
        onPointerMove={(e) => {
          if (disabled) return;
          const rect = e.currentTarget.getBoundingClientRect();
          setHover(starsAt((e.clientX - rect.left) / rect.width));
        }}
        onValueChange={([stars]) => setPreview(Math.max(MIN_RATING, stars))}
        onValueCommit={([stars]) => onChange(Math.max(MIN_RATING, stars))}
        step={0.25}
        value={[preview ?? 0]}
      >
        <SliderPrimitive.Track className="relative h-6 w-full">
          <div aria-hidden className="absolute inset-0 flex">
            {STARS.map((i) => (
              <Star className="h-6 w-6 text-muted-foreground/40" key={i} />
            ))}
          </div>
          <div
            aria-hidden
            className="absolute inset-y-0 left-0 flex overflow-hidden"
            style={{ width: `${(fill / MAX_RATING) * 100}%` }}
          >
            {STARS.map((i) => (
              <Star
                className={cn(
                  "h-6 w-6 shrink-0",
                  // A hover preview is paler than the saved rating.
                  hover === null
                    ? "fill-amber-400 text-amber-400"
                    : "fill-amber-400/60 text-amber-400/60",
                )}
                key={i}
              />
            ))}
          </div>
        </SliderPrimitive.Track>
        <SliderPrimitive.Thumb
          aria-label="Rating"
          aria-valuetext={
            preview === null ? "Not rated" : formatCount(preview, "star")
          }
          className="block h-6 w-0 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </SliderPrimitive.Root>
      {/* The slider already announces its value, so the number is visual. */}
      <span
        aria-hidden
        className={cn(
          "min-w-8 text-sm tabular-nums",
          hover === null ? "text-muted-foreground" : "text-foreground",
        )}
      >
        {shown !== null && formatRating(shown)}
      </span>
    </div>
  );
};

export default StarRating;
