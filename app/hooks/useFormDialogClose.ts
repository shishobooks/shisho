import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Hook to safely close a FormDialog after saving.
 *
 * The problem: When you update state (e.g., setInitialValues) and immediately
 * call onOpenChange(false), React batches these updates. FormDialog sees the
 * OLD hasChanges value and shows the confirmation dialog even though you just saved.
 *
 * The solution: This hook defers the close until after React processes state
 * updates, ensuring hasChanges reflects the saved state before closing.
 *
 * Session tracking: Each time the dialog opens, a new "session" begins. Close
 * requests are tagged with the session ID when made, and only processed if
 * they match the current session. This prevents two race conditions:
 * 1. Stale closeRequested from a previous session closing a newly opened dialog
 * 2. closeRequested persisting after a failed save and unexpectedly closing
 *    the dialog when the user later resets the form
 *
 * @example
 * const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);
 *
 * const handleSubmit = async () => {
 *   await mutation.mutateAsync({...});
 *   setInitialValues({...});  // hasChanges becomes false
 *   requestClose();           // Closes after hasChanges updates
 * };
 */
export function useFormDialogClose(
  open: boolean,
  onOpenChange: (open: boolean) => void,
  hasChanges: boolean,
): { requestClose: () => void } {
  // Session ID increments each time the dialog opens, invalidating any
  // pending close requests from previous sessions
  const sessionRef = useRef(0);

  // Track previous open state to detect open transitions
  const prevOpenRef = useRef(open);

  // Tracks which session the close was requested in (null = no request)
  const [closeRequestedSession, setCloseRequestedSession] = useState<
    number | null
  >(null);

  // Single effect handles both session increment and close logic.
  // By combining them, we guarantee the session increments BEFORE
  // we check it in the close logic, avoiding race conditions.
  useEffect(() => {
    // Detect transition: closed -> open (dialog just opened)
    const justOpened = open && !prevOpenRef.current;
    prevOpenRef.current = open;

    if (justOpened) {
      // Start a new session. This invalidates any pending close request
      // from a previous session.
      sessionRef.current += 1;
      // Clear any stale close request state
      setCloseRequestedSession(null);
      // Exit early - don't process close logic when dialog just opened
      return;
    }

    // Process close request only if:
    // 1. Dialog is currently open (we're trying to close it)
    // 2. Close was requested in THIS session (not stale from a previous session)
    // 3. Form has no unsaved changes
    if (
      open &&
      closeRequestedSession !== null &&
      closeRequestedSession === sessionRef.current &&
      !hasChanges
    ) {
      onOpenChange(false);
      setCloseRequestedSession(null);
    }
  }, [open, closeRequestedSession, hasChanges, onOpenChange]);

  // Request close tagged with current session ID.
  // If the dialog closes and reopens before this processes, the session ID
  // will have changed and this request will be ignored.
  const requestClose = useCallback(
    () => setCloseRequestedSession(sessionRef.current),
    [],
  );

  return { requestClose };
}
