import { useEffect, useEffectEvent, useRef, useState } from "react";

import { Input } from "@/components/ui/input";
import { cn } from "@/libraries/utils";

interface SearchInputProps {
  /** Names the input ("Search books"); a placeholder is not a name. */
  label: string;
  initialValue: string;
  onDebouncedChange: (value: string) => void;
  placeholder?: string;
  className?: string;
}

/**
 * Search input component that manages its own state for responsive typing,
 * debounces changes before reporting to parent, and properly handles
 * external value changes (e.g., from URL navigation) without overwriting
 * user input during typing.
 */
export const SearchInput = ({
  label,
  initialValue,
  onDebouncedChange,
  placeholder = "Search...",
  className = "max-w-xs",
}: SearchInputProps) => {
  const [value, setValue] = useState(initialValue);
  const prevInitialValue = useRef(initialValue);
  const lastReportedValue = useRef(initialValue);
  const reportChange = useEffectEvent(onDebouncedChange);

  // Sync from parent when external navigation changes the URL
  useEffect(() => {
    if (initialValue !== prevInitialValue.current) {
      // Only sync if this wasn't caused by our own debounced report
      if (initialValue !== lastReportedValue.current) {
        setValue(initialValue);
        // The parent already has this value, so don't report it back.
        lastReportedValue.current = initialValue;
      }
      prevInitialValue.current = initialValue;
    }
  }, [initialValue]);

  // Report typed text once it settles. The timer is keyed on the input text,
  // not a debounced copy of it: after back navigation a debounced copy still
  // holds the old query, and would either push it back into the URL or, if
  // that query is retyped, never change and so never report.
  useEffect(() => {
    if (value === lastReportedValue.current) return;
    const timer = setTimeout(() => {
      lastReportedValue.current = value;
      reportChange(value);
    }, 300);
    return () => clearTimeout(timer);
  }, [value]);

  return (
    <Input
      aria-label={label}
      className={cn(
        className,
        "[&::-webkit-search-cancel-button]:hidden [&::-webkit-search-decoration]:hidden",
      )}
      onChange={(e) => setValue(e.target.value)}
      placeholder={placeholder}
      type="search"
      value={value}
    />
  );
};
