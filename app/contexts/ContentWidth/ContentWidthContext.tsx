import {
  useContext,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { cn } from "@/libraries/utils";

import {
  ContentWidthContext,
  createContentWidthStore,
  FALLBACK_WIDTH,
} from "./context";

/**
 * Holds the page's content width for `useContentWidth`. It sits above the
 * routes because a page reads the width in the same component that renders
 * its layout, which a provider inside the layout would not reach.
 */
export const ContentWidthProvider = ({ children }: { children: ReactNode }) => {
  const [store] = useState(createContentWidthStore);
  return (
    <ContentWidthContext.Provider value={store}>
      {children}
    </ContentWidthContext.Provider>
  );
};

/**
 * Measures the width its children get and reports it to the provider, so a
 * gallery can size its page to the columns that fit before it fetches. Wrap
 * a page's content inside its padding; a page with a gallery needs one.
 */
export const ContentWidth = ({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) => {
  const ref = useRef<HTMLDivElement>(null);
  const store = useContext(ContentWidthContext);

  useLayoutEffect(() => {
    const element = ref.current;
    if (!element || !store) return;
    const measure = () => {
      // A modal menu or dialog locks scrolling and pads the body by the
      // scrollbar's width, which narrows the page only while it is open.
      // Measuring then would reflow and refetch the page behind it.
      if (document.body.hasAttribute("data-scroll-locked")) return;
      store.set(element.clientWidth || FALLBACK_WIDTH);
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => {
      observer.disconnect();
      store.set(null);
    };
  }, [store]);

  return (
    <div className={cn("w-full min-w-0", className)} ref={ref}>
      {children}
    </div>
  );
};
