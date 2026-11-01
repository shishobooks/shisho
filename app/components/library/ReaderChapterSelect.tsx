import { useRef } from "react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export interface ReaderChapterEntry {
  value: string;
  label: string;
  // A heading with no place to jump to.
  disabled?: boolean;
  // How many levels the entry nests under a heading.
  depth?: number;
}

interface ReaderChapterSelectProps {
  entries: ReaderChapterEntry[];
  // The current entry, or empty when the reader isn't in one.
  value: string;
  onValueChange: (value: string) => void;
}

// The chapter menu in a reader's header. After a pick with the mouse, focus
// doesn't return to the menu, so the arrow keys turn pages right away; a
// keyboard user keeps their place in the tab order.
const ReaderChapterSelect = ({
  entries,
  value,
  onValueChange,
}: ReaderChapterSelectProps) => {
  const current = entries.find((entry) => entry.value === value);
  const usedPointerRef = useRef(false);
  const markPointer = () => {
    usedPointerRef.current = true;
  };
  const markKeyboard = () => {
    usedPointerRef.current = false;
  };

  return (
    <Select onValueChange={onValueChange} value={value}>
      <SelectTrigger
        aria-label="Jump to chapter"
        className="w-auto max-w-[50vw] gap-2 [&>span]:truncate"
        onKeyDown={markKeyboard}
        onPointerDown={markPointer}
        title={current?.label}
      >
        <SelectValue placeholder="Chapters" />
      </SelectTrigger>
      <SelectContent
        align="end"
        onCloseAutoFocus={(e) => {
          if (usedPointerRef.current) e.preventDefault();
        }}
        onKeyDown={markKeyboard}
        onPointerDown={markPointer}
      >
        {entries.map((entry) => (
          <SelectItem
            disabled={entry.disabled}
            key={entry.value}
            style={{ paddingLeft: `${0.5 + (entry.depth ?? 0)}rem` }}
            value={entry.value}
          >
            {entry.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
};

export default ReaderChapterSelect;
