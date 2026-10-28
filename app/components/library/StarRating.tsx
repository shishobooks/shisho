import * as SliderPrimitive from "@radix-ui/react-slider";
import { Star } from "lucide-react";
import { useEffect, useState } from "react";

import { cn } from "@/libraries/utils";
import { formatCount } from "@/utils/format";

const STARS = [0, 1, 2, 3, 4];

// The lowest Rating. Zero stars doesn't exist: an unrated Book has none.
const MIN_RATING = 0.25;

interface StarRatingProps {
  /** Stars from 0.25 to 5 in quarter steps, or null when unrated. */
  value: number | null;
  /** Called once the user settles on a value, not on every drag step. */
  onChange: (stars: number) => void;
  disabled?: boolean;
}

// A five-star row that rates in quarter stars. It's a slider underneath, so
// a pointer picks the quarter under it and the arrow keys step by a quarter.
const StarRating = ({ value, onChange, disabled }: StarRatingProps) => {
  // The value shown while dragging, before it's committed.
  const [preview, setPreview] = useState<number | null>(value);
  useEffect(() => setPreview(value), [value]);

  const shown = preview ?? 0;
  return (
    <SliderPrimitive.Root
      className={cn(
        "relative flex h-6 w-[120px] touch-none select-none items-center",
        disabled ? "opacity-50" : "cursor-pointer",
      )}
      data-testid="star-rating"
      disabled={disabled}
      max={5}
      min={0}
      onValueChange={([stars]) => setPreview(Math.max(MIN_RATING, stars))}
      onValueCommit={([stars]) => onChange(Math.max(MIN_RATING, stars))}
      step={0.25}
      value={[shown]}
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
          style={{ width: `${(shown / 5) * 100}%` }}
        >
          {STARS.map((i) => (
            <Star
              className="h-6 w-6 shrink-0 fill-amber-400 text-amber-400"
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
  );
};

export default StarRating;
