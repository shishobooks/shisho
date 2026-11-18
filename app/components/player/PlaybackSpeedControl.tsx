import { Minus, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  formatSpeed,
  MAX_SPEED,
  MIN_SPEED,
  SPEED_PRESETS,
  stepSpeed,
} from "@/utils/playbackSpeed";

interface PlaybackSpeedControlProps {
  speed: number;
  onChange: (speed: number) => void;
}

// Minus and plus step the speed by 0.1 between the limits; the speed itself
// opens a menu of a few presets.
const PlaybackSpeedControl = ({
  speed,
  onChange,
}: PlaybackSpeedControlProps) => (
  <div aria-label="Playback speed" className="flex items-center" role="group">
    <Button
      aria-label="Slower"
      disabled={speed <= MIN_SPEED}
      onClick={() => onChange(stepSpeed(speed, -1))}
      size="icon-sm"
      variant="ghost"
    >
      <Minus />
    </Button>
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          aria-label={`Speed ${formatSpeed(speed)}`}
          className="min-w-14 tabular-nums"
          size="sm"
          variant="ghost"
        >
          {formatSpeed(speed)}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="center">
        <DropdownMenuRadioGroup
          onValueChange={(value) => onChange(Number(value))}
          value={String(speed)}
        >
          {SPEED_PRESETS.map((preset) => (
            <DropdownMenuRadioItem key={preset} value={String(preset)}>
              {formatSpeed(preset)}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
    <Button
      aria-label="Faster"
      disabled={speed >= MAX_SPEED}
      onClick={() => onChange(stepSpeed(speed, 1))}
      size="icon-sm"
      variant="ghost"
    >
      <Plus />
    </Button>
  </div>
);

export default PlaybackSpeedControl;
