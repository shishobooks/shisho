import { useFormDialogClose } from "./useFormDialogClose";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

describe("useFormDialogClose", () => {
  it("should not close dialog when hasChanges is true", () => {
    const onOpenChange = vi.fn();

    const { result } = renderHook(() =>
      useFormDialogClose(true, onOpenChange, true),
    );

    act(() => {
      result.current.requestClose();
    });

    // Should not close because hasChanges is still true
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("should close dialog when hasChanges becomes false after requestClose", () => {
    const onOpenChange = vi.fn();

    const { result, rerender } = renderHook(
      ({ open, hasChanges }) =>
        useFormDialogClose(open, onOpenChange, hasChanges),
      { initialProps: { open: true, hasChanges: true } },
    );

    // Request close while hasChanges is true
    act(() => {
      result.current.requestClose();
    });

    // hasChanges is still true, should not close yet
    expect(onOpenChange).not.toHaveBeenCalled();

    // hasChanges becomes false (simulating form save)
    rerender({ open: true, hasChanges: false });

    // Now it should close
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should not close dialog on re-open (session tracking prevents stale close)", () => {
    const onOpenChange = vi.fn();

    const { result, rerender } = renderHook(
      ({ open, hasChanges }) =>
        useFormDialogClose(open, onOpenChange, hasChanges),
      { initialProps: { open: true, hasChanges: true } },
    );

    // Request close while hasChanges is true
    act(() => {
      result.current.requestClose();
    });

    expect(onOpenChange).not.toHaveBeenCalled();

    // Dialog closes externally (e.g., user discards changes)
    rerender({ open: false, hasChanges: false });

    // Dialog reopens - new session begins
    rerender({ open: true, hasChanges: false });

    // The stale close request from previous session should NOT close this new session
    // Even though hasChanges is now false
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("should invalidate close request if dialog closes and reopens", () => {
    const onOpenChange = vi.fn();

    const { result, rerender } = renderHook(
      ({ open, hasChanges }) =>
        useFormDialogClose(open, onOpenChange, hasChanges),
      { initialProps: { open: true, hasChanges: true } },
    );

    // Request close in first session
    act(() => {
      result.current.requestClose();
    });

    // Dialog closes (user clicked discard or something)
    rerender({ open: false, hasChanges: false });

    // Dialog reopens (new session)
    rerender({ open: true, hasChanges: true });

    // Changes reset to false (e.g., form reset after reopening)
    rerender({ open: true, hasChanges: false });

    // The old close request should be invalidated, dialog should stay open
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("should handle multiple requestClose calls", () => {
    const onOpenChange = vi.fn();

    const { result, rerender } = renderHook(
      ({ open, hasChanges }) =>
        useFormDialogClose(open, onOpenChange, hasChanges),
      { initialProps: { open: true, hasChanges: true } },
    );

    // Call requestClose multiple times
    act(() => {
      result.current.requestClose();
      result.current.requestClose();
      result.current.requestClose();
    });

    // Still should not close
    expect(onOpenChange).not.toHaveBeenCalled();

    // hasChanges becomes false
    rerender({ open: true, hasChanges: false });

    // Should close exactly once
    expect(onOpenChange).toHaveBeenCalledTimes(1);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should close immediately if hasChanges is already false when requestClose is called", () => {
    const onOpenChange = vi.fn();

    const { result, rerender } = renderHook(
      ({ open, hasChanges }) =>
        useFormDialogClose(open, onOpenChange, hasChanges),
      { initialProps: { open: true, hasChanges: false } },
    );

    // Request close when hasChanges is already false
    act(() => {
      result.current.requestClose();
    });

    // Need to trigger a rerender to process the effect
    rerender({ open: true, hasChanges: false });

    // Should close
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should not close dialog when dialog is not open", () => {
    const onOpenChange = vi.fn();

    const { result, rerender } = renderHook(
      ({ open, hasChanges }) =>
        useFormDialogClose(open, onOpenChange, hasChanges),
      { initialProps: { open: false, hasChanges: false } },
    );

    act(() => {
      result.current.requestClose();
    });

    rerender({ open: false, hasChanges: false });

    // Should not call onOpenChange when dialog is already closed
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("should handle scenario where save fails and user retries", () => {
    const onOpenChange = vi.fn();

    const { result, rerender } = renderHook(
      ({ open, hasChanges }) =>
        useFormDialogClose(open, onOpenChange, hasChanges),
      { initialProps: { open: true, hasChanges: true } },
    );

    // First save attempt - request close
    act(() => {
      result.current.requestClose();
    });

    // Save fails, hasChanges stays true
    rerender({ open: true, hasChanges: true });

    // Should not close
    expect(onOpenChange).not.toHaveBeenCalled();

    // User makes more changes, then saves again
    act(() => {
      result.current.requestClose();
    });

    // This time save succeeds
    rerender({ open: true, hasChanges: false });

    // Should close
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should clear close request after closing", () => {
    const onOpenChange = vi.fn();

    const { result, rerender } = renderHook(
      ({ open, hasChanges }) =>
        useFormDialogClose(open, onOpenChange, hasChanges),
      { initialProps: { open: true, hasChanges: true } },
    );

    // Request close
    act(() => {
      result.current.requestClose();
    });

    // Save completes
    rerender({ open: true, hasChanges: false });

    // Should close once
    expect(onOpenChange).toHaveBeenCalledTimes(1);

    // Reset mock
    onOpenChange.mockClear();

    // Dialog reopens with new session
    rerender({ open: false, hasChanges: false });
    rerender({ open: true, hasChanges: false });

    // No additional close should happen (close request was cleared)
    expect(onOpenChange).not.toHaveBeenCalled();
  });
});
