import { useEffect, useState } from "react";

export function useDebounce<T>(
  value: T,
  delay: number = 300,
  options?: { immediate?: (v: T) => boolean },
): T {
  const isImmediate = options?.immediate?.(value) ?? false;
  const [debouncedValue, setDebouncedValue] = useState(value);

  useEffect(() => {
    if (isImmediate) {
      setDebouncedValue(value);
      return;
    }

    const timer = setTimeout(() => {
      setDebouncedValue(value);
    }, delay);

    return () => {
      clearTimeout(timer);
    };
  }, [value, delay, isImmediate]);

  return debouncedValue;
}
