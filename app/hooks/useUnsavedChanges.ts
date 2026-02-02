import { useCallback, useEffect } from "react";
import { useBlocker } from "react-router-dom";

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
  const blocker = useBlocker(hasChanges);

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
