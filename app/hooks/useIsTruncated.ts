import { useEffect, useRef, useState } from "react";

export function useIsTruncated<T extends HTMLElement>(): [
  React.RefObject<T | null>,
  boolean,
] {
  const ref = useRef<T | null>(null);
  const [isTruncated, setIsTruncated] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;

    const check = () => {
      setIsTruncated(el.scrollHeight > el.clientHeight);
    };

    check();
    const observer = new ResizeObserver(check);
    observer.observe(el);

    return () => observer.disconnect();
  }, []);

  return [ref, isTruncated];
}
