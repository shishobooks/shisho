import { useEffect, useRef, useState, type KeyboardEvent } from "react";

export interface HighlightableOption {
  /** The `CommandItem`'s value. */
  value: string;
  /** What the item's `onSelect` does. */
  select: () => void;
}

// Keys that don't change the search text, so they keep a held Enter.
const MODIFIER_KEYS = new Set(["Shift", "Control", "Alt", "Meta"]);

/**
 * Controls which option a cmdk `Command` highlights, so the first option is
 * highlighted whenever the options change and Enter picks it.
 *
 * cmdk highlights the first option only when the search text changes. With
 * `shouldFilter={false}` the options arrive later, after a debounce or a
 * request, and cmdk keeps the stale highlight of an option that is gone, so
 * nothing is highlighted and Enter does nothing.
 *
 * An Enter in the search box while the options are loading or `pending` is
 * held, and picks the first option once they answer the typed text. Typing
 * on, clicking in the dropdown, closing it, or the search failing drops it.
 *
 * Pass every `CommandItem` in display order and spread the result onto
 * `Command`. Arrow keys and hovering still move the highlight.
 */
export function useHighlightFirstOption({
  options,
  loading,
  pending,
  failed = false,
  open,
}: {
  options: HighlightableOption[];
  /** No options are shown yet. */
  loading: boolean;
  /** The shown options don't answer the typed text yet. */
  pending: boolean;
  /** The search for the typed text failed, so its options never come. */
  failed?: boolean;
  /** Whether the dropdown holding the `Command` is open. */
  open: boolean;
}) {
  // cmdk trims item values before comparing them with the highlighted one.
  const values = loading ? [] : options.map((o) => o.value.trim());
  const [highlighted, setHighlighted] = useState("");
  const optionsKey = values.join("\n");
  const [prevOptionsKey, setPrevOptionsKey] = useState(optionsKey);
  if (optionsKey !== prevOptionsKey) {
    setPrevOptionsKey(optionsKey);
    setHighlighted("");
  }
  const value = values.includes(highlighted) ? highlighted : (values[0] ?? "");
  const waiting = loading || pending;

  const enterHeld = useRef(false);
  useEffect(() => {
    if (!open || failed) enterHeld.current = false;
    if (!enterHeld.current || waiting) return;
    enterHeld.current = false;
    options[values.indexOf(value)]?.select();
  });

  const onKeyDown = (e: KeyboardEvent) => {
    // cmdk ignores these too: Safari commits a composition with an Enter
    // whose keyCode is 229 but isComposing is false.
    if (e.nativeEvent.isComposing || e.keyCode === 229) return;
    if (e.key === "Enter") {
      // Only the search box's Enter, not one on a control beside the list.
      const inSearchBox = (e.target as HTMLElement).hasAttribute("cmdk-input");
      if (!waiting || !inSearchBox) return;
      // Stops cmdk picking a stale option.
      e.preventDefault();
      enterHeld.current = true;
    } else if (!MODIFIER_KEYS.has(e.key)) {
      enterHeld.current = false;
    }
  };

  const onPointerDown = () => {
    enterHeld.current = false;
  };

  return { value, onValueChange: setHighlighted, onKeyDown, onPointerDown };
}
