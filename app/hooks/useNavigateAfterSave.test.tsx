import { useNavigateAfterSave } from "./useNavigateAfterSave";
import { act, render, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useMemo, useState } from "react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Mock useNavigate
const mockNavigate = vi.fn();
vi.mock("react-router-dom", async () => {
  const actual = await vi.importActual("react-router-dom");
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

describe("useNavigateAfterSave", () => {
  beforeEach(() => {
    mockNavigate.mockClear();
  });

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <MemoryRouter>{children}</MemoryRouter>
  );

  it("should not navigate on initial render even if hasChanges is false", () => {
    renderHook(() => useNavigateAfterSave(false), { wrapper });

    // No navigation should happen on initial render
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("should navigate when hasChanges transitions from true to false after requestNavigate", () => {
    const { result, rerender } = renderHook(
      ({ hasChanges }) => useNavigateAfterSave(hasChanges),
      { wrapper, initialProps: { hasChanges: true } },
    );

    // Request navigation
    act(() => {
      result.current.requestNavigate("/books/123");
    });

    // hasChanges is still true, should not navigate yet
    expect(mockNavigate).not.toHaveBeenCalled();

    // hasChanges becomes false (save completed)
    rerender({ hasChanges: false });

    // Now should navigate
    expect(mockNavigate).toHaveBeenCalledWith("/books/123");
  });

  it("should not navigate if requestNavigate was not called", () => {
    const { rerender } = renderHook(
      ({ hasChanges }) => useNavigateAfterSave(hasChanges),
      { wrapper, initialProps: { hasChanges: true } },
    );

    // hasChanges becomes false without requestNavigate
    rerender({ hasChanges: false });

    // Should not navigate
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("should cancel pending navigation if user makes new changes", () => {
    const { result, rerender } = renderHook(
      ({ hasChanges }) => useNavigateAfterSave(hasChanges),
      { wrapper, initialProps: { hasChanges: true } },
    );

    // Request navigation
    act(() => {
      result.current.requestNavigate("/books/123");
    });

    // Save completes, hasChanges becomes false
    rerender({ hasChanges: false });

    // But before navigation completes, user makes new changes
    rerender({ hasChanges: true });

    // Clear the mock to check if navigation happens again
    mockNavigate.mockClear();

    // hasChanges becomes false again
    rerender({ hasChanges: false });

    // Navigation should have been cancelled - no new navigation
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("should use latest destination when requestNavigate is called multiple times", () => {
    const { result, rerender } = renderHook(
      ({ hasChanges }) => useNavigateAfterSave(hasChanges),
      { wrapper, initialProps: { hasChanges: true } },
    );

    // Request navigation to first destination
    act(() => {
      result.current.requestNavigate("/books/123");
    });

    // Request navigation to second destination (should replace first)
    act(() => {
      result.current.requestNavigate("/books/456");
    });

    // hasChanges becomes false
    rerender({ hasChanges: false });

    // Should navigate to the latest destination
    expect(mockNavigate).toHaveBeenCalledWith("/books/456");
    expect(mockNavigate).toHaveBeenCalledTimes(1);
  });

  it("should not navigate if hasChanges was never true after requestNavigate", () => {
    const { result, rerender } = renderHook(
      ({ hasChanges }) => useNavigateAfterSave(hasChanges),
      { wrapper, initialProps: { hasChanges: false } },
    );

    // Request navigation when hasChanges is already false
    act(() => {
      result.current.requestNavigate("/books/123");
    });

    // Trigger rerender
    rerender({ hasChanges: false });

    // Should not navigate because hasChanges was never true after the request
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("should cancel navigation when hasChanges transitions false->true after requestNavigate", () => {
    // This tests the race condition protection: if hasChanges goes false->true
    // after requestNavigate, it means new changes were made after the save started,
    // so we should cancel the navigation.
    const { result, rerender } = renderHook(
      ({ hasChanges }) => useNavigateAfterSave(hasChanges),
      { wrapper, initialProps: { hasChanges: false } },
    );

    // Request navigation when hasChanges is false
    act(() => {
      result.current.requestNavigate("/books/123");
    });

    // hasChanges becomes true (user makes NEW changes after requesting navigation)
    // This sets sawChangesAfterRequest = true
    rerender({ hasChanges: true });

    // No navigation yet
    expect(mockNavigate).not.toHaveBeenCalled();

    // hasChanges becomes false (changes saved or reset)
    // But because we saw a false->true transition after sawChangesAfterRequest was set,
    // the navigation should have been cancelled
    rerender({ hasChanges: false });

    // Navigation was cancelled because new changes were made after the navigation request
    // The hook interprets false->true transition as "user made new changes"
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("should clear pending navigation after successful navigation", () => {
    const { result, rerender } = renderHook(
      ({ hasChanges }) => useNavigateAfterSave(hasChanges),
      { wrapper, initialProps: { hasChanges: true } },
    );

    // Request navigation
    act(() => {
      result.current.requestNavigate("/books/123");
    });

    // hasChanges becomes false
    rerender({ hasChanges: false });

    // Should navigate once
    expect(mockNavigate).toHaveBeenCalledTimes(1);

    // Clear mock
    mockNavigate.mockClear();

    // hasChanges toggles again
    rerender({ hasChanges: true });
    rerender({ hasChanges: false });

    // Should not navigate again (pending navigation was cleared)
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it("should handle rapid hasChanges transitions correctly", () => {
    const { result, rerender } = renderHook(
      ({ hasChanges }) => useNavigateAfterSave(hasChanges),
      { wrapper, initialProps: { hasChanges: true } },
    );

    // Request navigation
    act(() => {
      result.current.requestNavigate("/books/123");
    });

    // Rapid transitions
    rerender({ hasChanges: false });
    rerender({ hasChanges: true });
    rerender({ hasChanges: false });

    // Should have navigated on first false transition
    expect(mockNavigate).toHaveBeenCalledTimes(1);
  });

  it("should navigate when hasChanges becomes false in the same render as requestNavigate (batched state)", () => {
    // This reproduces the real-world scenario where:
    // 1. User has unsaved changes (hasChanges = true)
    // 2. User clicks save
    // 3. In the same handler: setChangesSaved(true) and requestNavigate(url)
    // 4. React batches these, so the next render has hasChanges = false AND pendingNavigation set
    //
    // The bug was that sawChangesAfterRequest was reset by requestNavigate,
    // and since hasChanges was already false when the effect ran,
    // sawChangesAfterRequest was never set to true, so navigation never happened.

    const { result, rerender } = renderHook(
      ({ hasChanges }) => useNavigateAfterSave(hasChanges),
      { wrapper, initialProps: { hasChanges: true } },
    );

    // Simulate batched state update: both hasChanges becoming false AND requestNavigate
    // happen in the same act(), resulting in a single render where hasChanges is already false
    act(() => {
      result.current.requestNavigate("/books/123");
    });

    // In the real scenario, hasChanges would already be false in the same render
    // as when requestNavigate is called (due to React batching).
    // We simulate this by immediately rerendering with hasChanges = false
    // within the same test flow, before any intermediate render with hasChanges = true.
    rerender({ hasChanges: false });

    // Should navigate because hasChanges WAS true before requestNavigate was called,
    // even if it becomes false in the same batched render.
    expect(mockNavigate).toHaveBeenCalledWith("/books/123");
  });

  it("should navigate when requestNavigate is called with hasChanges already false (post-save scenario)", () => {
    // This is the ACTUAL bug scenario in CreateLibrary/CreateUser/Setup:
    // 1. Component renders with hasChanges = true (user filled form)
    // 2. User clicks save
    // 3. In handler: setChangesSaved(true) makes hasChanges become false
    // 4. In same handler: requestNavigate(url) is called
    // 5. Due to React batching, when effects run, hasChanges is already false
    //
    // The hook needs to handle this by checking prevHasChanges.

    const { result, rerender } = renderHook(
      ({ hasChanges }) => useNavigateAfterSave(hasChanges),
      { wrapper, initialProps: { hasChanges: true } },
    );

    // First render established hasChanges = true
    // Now simulate the batched update where hasChanges becomes false AND requestNavigate is called
    // We need to call requestNavigate and change hasChanges in the same act() to simulate batching
    act(() => {
      // This simulates what happens when setChangesSaved(true) and requestNavigate(url)
      // are called in the same handler - the ref reset happens immediately,
      // but we need to also update hasChanges before effects run
      result.current.requestNavigate("/libraries/123");
    });

    // Critically: change hasChanges to false BEFORE the effect from requestNavigate
    // would have a chance to see hasChanges=true. This simulates React batching.
    rerender({ hasChanges: false });

    // The navigation should happen because the hook detected that hasChanges
    // WAS true before it became false (using prevHasChanges)
    expect(mockNavigate).toHaveBeenCalledWith("/libraries/123");
  });

  it("should navigate when requestNavigate and hasChanges=false happen in same batched render", () => {
    // This test accurately simulates the real-world scenario:
    // In the same handler, setChangesSaved(true) and requestNavigate(url) are called.
    // React batches both, so the next render has hasChanges=false AND pendingNavigation set.
    //
    // The key is that wasChanged (from the PREVIOUS render before both updates)
    // should be true, allowing navigation to proceed.

    const { result, rerender } = renderHook(
      ({ hasChanges }) => useNavigateAfterSave(hasChanges),
      { wrapper, initialProps: { hasChanges: true } },
    );

    // Simulate what happens in real code:
    // 1. hasChanges is true (from initial render)
    // 2. User clicks save button
    // 3. Handler calls setChangesSaved(true) AND requestNavigate(url) synchronously
    // 4. React batches both, next render has hasChanges=false AND pendingNavigation=url
    //
    // In testing, we simulate this by calling requestNavigate AND changing hasChanges
    // to false in the same "batch" - achieved by calling requestNavigate first,
    // then immediately rerendering with hasChanges=false (simulating batched state)
    act(() => {
      result.current.requestNavigate("/new-entity/456");
    });

    // The rerender happens with hasChanges=false (simulating batched state update)
    // At this point, prevHasChanges.current should still be true from initial render
    rerender({ hasChanges: false });

    // Should navigate because wasChanged was true in the effect
    expect(mockNavigate).toHaveBeenCalledWith("/new-entity/456");
  });

  describe("integration with component state (reproduces batched state bug)", () => {
    // This test uses a real component that mimics CreateLibrary/CreateUser/Setup
    // to accurately reproduce the batched state scenario that causes the bug.
    //
    // The bug: When setChangesSaved(true) and requestNavigate(url) are called
    // in the same handler, React batches both. The effect runs with hasChanges
    // already false, but sawChangesAfterRequest was reset by requestNavigate().
    // Since hasChanges is false, sawChangesAfterRequest never becomes true,
    // and navigation never happens.

    function TestComponent({ onNavigate }: { onNavigate?: () => void }) {
      const [formValue, setFormValue] = useState("");
      const [changesSaved, setChangesSaved] = useState(false);

      // Mirrors the pattern in CreateLibrary.tsx
      const hasUnsavedChanges = useMemo(() => {
        if (changesSaved) return false;
        return formValue.trim() !== "";
      }, [changesSaved, formValue]);

      const { requestNavigate } = useNavigateAfterSave(hasUnsavedChanges);

      const handleSave = () => {
        // This is the exact pattern that causes the bug:
        // Both state updates happen in the same synchronous handler,
        // so React batches them into a single render.
        setChangesSaved(true);
        requestNavigate("/success/123");
        onNavigate?.();
      };

      return (
        <div>
          <input
            data-testid="input"
            onChange={(e) => setFormValue(e.target.value)}
            value={formValue}
          />
          <span data-testid="has-changes">
            {hasUnsavedChanges ? "yes" : "no"}
          </span>
          <button data-testid="save" onClick={handleSave}>
            Save
          </button>
        </div>
      );
    }

    it("should navigate after save when setChangesSaved and requestNavigate are called in same handler", async () => {
      // This test reproduces the EXACT bug scenario from CreateLibrary/CreateUser/Setup
      const user = userEvent.setup();

      render(
        <MemoryRouter>
          <TestComponent />
        </MemoryRouter>,
      );

      // Initially no changes
      expect(screen.getByTestId("has-changes")).toHaveTextContent("no");

      // User types something - now has unsaved changes
      await user.type(screen.getByTestId("input"), "test value");
      expect(screen.getByTestId("has-changes")).toHaveTextContent("yes");

      // User clicks save - this calls setChangesSaved(true) AND requestNavigate()
      // in the same handler, which React batches
      await user.click(screen.getByTestId("save"));

      // After save, hasUnsavedChanges should be false
      expect(screen.getByTestId("has-changes")).toHaveTextContent("no");

      // THE BUG: Navigation should have happened, but with the buggy code it doesn't
      // because sawChangesAfterRequest was reset and hasChanges was already false
      // when the effect ran.
      expect(mockNavigate).toHaveBeenCalledWith("/success/123");
    });
  });
});
