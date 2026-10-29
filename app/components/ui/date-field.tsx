import { CalendarIcon } from "lucide-react";
import * as React from "react";

import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/libraries/utils";
import { formatDate, localDay } from "@/utils/format";

// Days are YYYY-MM-DD strings, read and written in local time so a day
// never shifts across a time zone.
const parseDay = (day: string): Date | undefined => {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(day);
  if (!match) return undefined;
  return new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
};

interface DateFieldProps {
  id?: string;
  /**
   * The id of the field's visible label. The trigger's name is the label
   * followed by the date it shows, so a screen reader hears both; a plain
   * <Label htmlFor> would replace the date with the label alone.
   */
  labelId?: string;
  /** The day as YYYY-MM-DD, or "" for none. */
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  /** Days after this one can't be picked. */
  max?: Date;
  disabled?: boolean;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
}

// A day picker themed like the rest of the app, in place of the browser's
// native date input.
export const DateField = ({
  id,
  labelId,
  value,
  onChange,
  placeholder = "Pick a date",
  max,
  disabled,
  ...aria
}: DateFieldProps) => {
  const [open, setOpen] = React.useState(false);
  const valueId = React.useId();
  const selected = parseDay(value);
  const endMonth = max ?? new Date(new Date().getFullYear() + 10, 11);

  const pick = (day: string) => {
    onChange(day);
    setOpen(false);
  };

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger asChild>
        <Button
          {...aria}
          aria-labelledby={labelId ? `${labelId} ${valueId}` : undefined}
          // Left-aligned like the text input it replaces.
          className={cn(
            "w-full justify-start font-normal",
            !selected && "text-muted-foreground",
          )}
          disabled={disabled}
          id={id}
          variant="outline"
        >
          <CalendarIcon aria-hidden className="h-4 w-4" />
          <span id={valueId}>{selected ? formatDate(value) : placeholder}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-auto p-0">
        {/* required: picking the selected day again keeps it rather than
            deselecting it; Clear is how a day is removed. */}
        <Calendar
          autoFocus
          className="bg-transparent"
          captionLayout="dropdown"
          defaultMonth={selected ?? max}
          disabled={max ? { after: max } : undefined}
          endMonth={endMonth}
          mode="single"
          onSelect={(date: Date) => pick(localDay(date))}
          required
          selected={selected}
          startMonth={new Date(1900, 0)}
        />
        <div className="flex justify-between border-t p-2">
          <Button onClick={() => pick("")} size="sm" variant="ghost">
            Clear
          </Button>
          <Button
            disabled={max !== undefined && localDay(max) < localDay(new Date())}
            onClick={() => pick(localDay(new Date()))}
            size="sm"
            variant="ghost"
          >
            Today
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
};
