import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";

/**
 * Hook to safely navigate after saving on a page with unsaved changes protection.
 *
 * The problem: When you update state (e.g., setChangesSaved(true)) and immediately
 * call navigate(), React batches these updates. The useBlocker hook sees the OLD
 * hasChanges value and blocks navigation even though you just saved.
 *
 * The solution: This hook defers navigation until after React processes state
 * updates, ensuring hasChanges reflects the saved state before navigating.
 *
 * Session tracking: Each navigation request is tagged with a "session" that
 * tracks whether hasChanges was ever true after the request was made. Navigation
 * only triggers when hasChanges transitions from true to false AFTER the request.
 * This prevents:
 * 1. Navigation on initial render when hasChanges starts as false
 * 2. Unexpected navigation if the user resets the form after a failed save
 *
 * @example
 * const { requestNavigate } = useNavigateAfterSave(hasUnsavedChanges);
 *
 * const handleCreate = async () => {
 *   const result = await mutation.mutateAsync({...});
 *   setChangesSaved(true);  // hasUnsavedChanges becomes false
 *   requestNavigate(`/items/${result.id}`);  // Navigates after state updates
 * };
 */
export function useNavigateAfterSave(hasChanges: boolean): {
  requestNavigate: (to: string) => void;
} {
  const navigate = useNavigate();

  // Pending navigation destination (null = no request)
  const [pendingNavigation, setPendingNavigation] = useState<string | null>(
    null,
  );

  // Tracks whether hasChanges was ever true AFTER requestNavigate was called.
  // Navigation only triggers when this is true AND hasChanges becomes false.
  // This prevents navigation on initial render or if form is reset without
  // hasChanges ever being true during this navigation request session.
  const sawChangesAfterRequest = useRef(false);

  // Track previous hasChanges to detect transitions
  const prevHasChanges = useRef(hasChanges);

  useEffect(() => {
    const wasChanged = prevHasChanges.current;
    prevHasChanges.current = hasChanges;

    // If there's no pending navigation, nothing to do
    if (!pendingNavigation) {
      return;
    }

    // Track if hasChanges was ever true after the navigation was requested.
    // Check BOTH current and previous value to handle React batching:
    // When setChangesSaved(true) and requestNavigate(url) are called in the same
    // handler, React batches both updates. The effect runs with hasChanges already
    // false, but wasChanged (the previous render's value) is true.
    if (hasChanges || wasChanged) {
      sawChangesAfterRequest.current = true;
    }

    // Clear pending navigation if user makes NEW changes after we already
    // saw hasChanges become true. This handles the scenario:
    // 1. User clicks save, requestNavigate called, hasChanges becomes false
    // 2. Before navigation completes, user makes new edits (hasChanges: false -> true)
    // 3. We should cancel the pending navigation since there are now new unsaved changes
    if (!wasChanged && hasChanges && sawChangesAfterRequest.current) {
      setPendingNavigation(null);
      sawChangesAfterRequest.current = false;
      return;
    }

    // Only navigate when:
    // 1. We have a pending navigation request
    // 2. hasChanges was true at some point after the request (sawChangesAfterRequest)
    // 3. hasChanges is now false (save completed)
    if (sawChangesAfterRequest.current && !hasChanges) {
      navigate(pendingNavigation);
      setPendingNavigation(null);
      sawChangesAfterRequest.current = false;
    }
  }, [pendingNavigation, hasChanges, navigate]);

  const requestNavigate = useCallback((to: string) => {
    // Reset the session tracking when a new navigation is requested
    sawChangesAfterRequest.current = false;
    setPendingNavigation(to);
  }, []);

  return { requestNavigate };
}
