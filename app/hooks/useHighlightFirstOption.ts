import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";

export interface HighlightableOption {
  /** The `CommandItem`'s value. */
  value: string;
  /** What the item's `onSelect` does. */
  select: () => void;
}

// Keys that don't change the search text, so they keep a held Enter.
const MODIFIER_KEYS = new Set(["Shift", "Control", "Alt", "Meta"]);

// Keys that move cmdk's highlight, including its default Ctrl bindings.
const MOVE_KEYS = new Set(["ArrowDown", "ArrowUp", "Home", "End"]);
const CTRL_MOVE_KEYS = new Set(["n", "j", "p", "k"]);

// A cmdk value matching no option. cmdk auto-picks the first option only when
// it has no value, and this counts as one.
const NO_HIGHLIGHT = "__no-highlight-";

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
 * A caller that can act on that Enter itself passes `onEnterWhileWaiting`, and
 * nothing is held.
 *
 * With `highlightFirst` off, nothing is highlighted until an arrow key or the
 * pointer moves to an option, and cmdk's Enter does nothing without a
 * highlight.
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
  highlightFirst = true,
  onEnterWhileWaiting,
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
  /** Whether the first option starts highlighted. */
  highlightFirst?: boolean;
  /** Takes over an Enter in the search box while the options are `waiting`. */
  onEnterWhileWaiting?: () => void;
}) {
  // cmdk trims item values before comparing them with the highlighted one.
  const values = loading ? [] : options.map((o) => o.value.trim());
  const [highlighted, setHighlighted] = useState("");
  // With highlightFirst off, only a report after a user's move counts. cmdk
  // also auto-picks the first option, setting it internally before reporting
  // it, and re-syncs only when its value prop changes: rejectedPicks changes
  // that prop to take such a pick back.
  const userMoved = useRef(false);
  const [rejectedPicks, setRejectedPicks] = useState(0);
  const resetKey = `${open}\n${highlightFirst}\n${values.join("\n")}`;
  const [prevResetKey, setPrevResetKey] = useState(resetKey);
  if (resetKey !== prevResetKey) {
    setPrevResetKey(resetKey);
    setHighlighted("");
  }
  // Before cmdk's auto-pick for the new options, which comes a commit later.
  useLayoutEffect(() => {
    userMoved.current = false;
  }, [resetKey]);
  const value = values.includes(highlighted)
    ? highlighted
    : highlightFirst
      ? (values[0] ?? "")
      : `${NO_HIGHLIGHT}${rejectedPicks}`;
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
    if (MOVE_KEYS.has(e.key) || (e.ctrlKey && CTRL_MOVE_KEYS.has(e.key))) {
      userMoved.current = true;
    }
    if (e.key === "Enter") {
      // Only the search box's Enter, not one on a control beside the list.
      const inSearchBox = (e.target as HTMLElement).hasAttribute("cmdk-input");
      if (!waiting || !inSearchBox) return;
      // Stops cmdk picking a stale option.
      e.preventDefault();
      if (onEnterWhileWaiting) onEnterWhileWaiting();
      else enterHeld.current = true;
    } else if (!MODIFIER_KEYS.has(e.key)) {
      enterHeld.current = false;
    }
  };

  const onPointerDown = () => {
    enterHeld.current = false;
  };

  const onValueChange = (next: string) => {
    if (next.startsWith(NO_HIGHLIGHT)) return;
    if (highlightFirst || userMoved.current) setHighlighted(next);
    else setRejectedPicks((n) => n + 1);
  };

  // Capture, because cmdk's item moves the highlight in its own pointermove.
  const onPointerMoveCapture = () => {
    userMoved.current = true;
  };

  return {
    value,
    onValueChange,
    onKeyDown,
    onPointerDown,
    onPointerMoveCapture,
  };
}
