import { useCallback, useEffect } from "react";
import { useBlocker, type BlockerFunction } from "react-router-dom";

export interface UseUnsavedChangesOptions {
  /**
   * Lets one kind of in-app navigation through even with unsaved changes.
   * Only for a navigation that keeps the page and everything typed on it,
   * like Add User switching between its two forms; any other navigation
   * still asks. Browser close and refresh always ask.
   */
  allowNavigation?: (args: Parameters<BlockerFunction>[0]) => boolean;
}

interface UseUnsavedChangesResult {
  /** Whether the blocker dialog should be shown */
  showBlockerDialog: boolean;
  /** Call this to proceed with navigation and discard changes */
  proceedNavigation: () => void;
  /** Call this to cancel navigation and stay on the page */
  cancelNavigation: () => void;
}

/**
 * Hook to warn users about unsaved changes when navigating away.
 *
 * Handles two scenarios:
 * 1. Browser close/refresh: Shows native browser dialog via `beforeunload`
 * 2. SPA navigation: Uses react-router's `useBlocker` to show custom dialog
 *
 * @param hasChanges - Whether there are unsaved changes to protect
 * @param options - `allowNavigation` exempts a navigation that keeps the input
 * @returns Controls for the blocker dialog
 *
 * @example
 * const { showBlockerDialog, proceedNavigation, cancelNavigation } = useUnsavedChanges(hasChanges);
 *
 * return (
 *   <>
 *     <YourForm />
 *     <UnsavedChangesDialog
 *       open={showBlockerDialog}
 *       onStay={cancelNavigation}
 *       onDiscard={proceedNavigation}
 *     />
 *   </>
 * );
 */
export function useUnsavedChanges(
  hasChanges: boolean,
  { allowNavigation }: UseUnsavedChangesOptions = {},
): UseUnsavedChangesResult {
  // Handle browser close/refresh with native dialog
  useEffect(() => {
    if (!hasChanges) return;

    const handleBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      // Modern browsers ignore custom messages, but we need to set returnValue
      // for the native dialog to appear in some browsers
      e.returnValue = "";
    };

    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [hasChanges]);

  // Handle SPA navigation with react-router blocker
  const shouldBlock = useCallback<BlockerFunction>(
    (args) => hasChanges && !(allowNavigation?.(args) ?? false),
    [hasChanges, allowNavigation],
  );
  const blocker = useBlocker(shouldBlock);

  const proceedNavigation = useCallback(() => {
    if (blocker.state === "blocked") {
      blocker.proceed();
    }
  }, [blocker]);

  const cancelNavigation = useCallback(() => {
    if (blocker.state === "blocked") {
      blocker.reset();
    }
  }, [blocker]);

  return {
    showBlockerDialog: blocker.state === "blocked",
    proceedNavigation,
    cancelNavigation,
  };
}
