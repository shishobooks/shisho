import { useEffect, useRef, useState } from "react";

import { Input } from "@/components/ui/input";
import { useDebounce } from "@/hooks/useDebounce";

interface SearchInputProps {
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
  initialValue,
  onDebouncedChange,
  placeholder = "Search...",
  className = "max-w-xs",
}: SearchInputProps) => {
  const [value, setValue] = useState(initialValue);
  const debouncedValue = useDebounce(value, 300);
  const prevInitialValue = useRef(initialValue);
  const lastReportedValue = useRef(initialValue);

  // Sync from parent when external navigation changes the URL
  useEffect(() => {
    if (initialValue !== prevInitialValue.current) {
      // Only sync if this wasn't caused by our own debounced report
      if (initialValue !== lastReportedValue.current) {
        setValue(initialValue);
      }
      prevInitialValue.current = initialValue;
    }
  }, [initialValue]);

  // Report debounced changes to parent
  useEffect(() => {
    lastReportedValue.current = debouncedValue;
    onDebouncedChange(debouncedValue);
  }, [debouncedValue, onDebouncedChange]);

  return (
    <Input
      className={`${className} [&::-webkit-search-cancel-button]:hidden [&::-webkit-search-decoration]:hidden`}
      onChange={(e) => setValue(e.target.value)}
      placeholder={placeholder}
      type="search"
      value={value}
    />
  );
};
