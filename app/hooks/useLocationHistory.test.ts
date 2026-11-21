import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { useLocationHistory } from "./useLocationHistory";

// A stand-in for <foliate-view>. goTo lands where it's sent and reports it
// as foliate does, and a test can hold its renderer in the middle of a page
// turn, when the real one ignores goTo.
const makeView = () => {
  const view = document.createElement("div");
  const relocate = (cfi: string) =>
    view.dispatchEvent(new CustomEvent("relocate", { detail: { cfi } }));
  const goTo = vi.fn((cfi: string) => {
    relocate(cfi);
    return Promise.resolve();
  });
  let turning = Promise.resolve();
  let land = () => {};
  Object.assign(view, { goTo, renderer: { settled: () => turning } });
  return {
    view,
    goTo,
    relocate,
    startTurn: () => {
      turning = new Promise((resolve) => (land = resolve));
    },
    // A turn reports where it landed before it lets go.
    landTurn: (cfi: string) => {
      relocate(cfi);
      land();
    },
  };
};

const flush = () =>
  act(async () => {
    for (let i = 0; i < 10; i++) await Promise.resolve();
  });

describe("useLocationHistory", () => {
  it("goes back once a page turn in progress lands, keeping where it landed for forward", async () => {
    const { view, goTo, relocate, startTurn, landTurn } = makeView();
    const { result } = renderHook(() => useLocationHistory(view));
    relocate("start");
    await act(() => result.current.jump(() => relocate("jumped-to")));
    expect(result.current.canGoBack).toBe(true);

    startTurn();
    act(() => result.current.goBack());
    await flush();
    expect(goTo).not.toHaveBeenCalled();

    landTurn("turned-to");
    await flush();
    expect(goTo).toHaveBeenCalledWith("start");

    act(() => result.current.goForward());
    await flush();
    expect(goTo).toHaveBeenLastCalledWith("turned-to");
  });

  it("records a jump made during a page turn as leaving from where the turn landed", async () => {
    const { view, goTo, relocate, startTurn, landTurn } = makeView();
    const { result } = renderHook(() => useLocationHistory(view));
    relocate("start");

    startTurn();
    const go = vi.fn(() => relocate("jumped-to"));
    let jumped!: Promise<unknown>;
    act(() => {
      jumped = result.current.jump(go);
    });
    await flush();
    expect(go).not.toHaveBeenCalled();

    landTurn("turned-to");
    await act(() => jumped);
    expect(go).toHaveBeenCalled();

    act(() => result.current.goBack());
    await flush();
    expect(goTo).toHaveBeenCalledWith("turned-to");
  });

  // foliate replaces the renderer when a Book opens, and the old one can be
  // left mid-turn for good, such as on a section that never loads.
  it("starts a Book opened again on moves of its own, not ones still waiting on the last", async () => {
    const { view, relocate, startTurn, landTurn } = makeView();
    const { result } = renderHook(() => useLocationHistory(view));
    relocate("start");

    startTurn();
    const stale = vi.fn(() => relocate("old-book"));
    act(() => void result.current.jump(stale).catch(() => {}));
    await flush();

    act(() => result.current.reset());
    Object.assign(view, { renderer: { settled: () => Promise.resolve() } });
    relocate("new-start");
    const fresh = vi.fn(() => relocate("new-jumped-to"));
    await act(() => result.current.jump(fresh));

    expect(fresh).toHaveBeenCalled();
    expect(stale).not.toHaveBeenCalled();
    expect(result.current.canGoBack).toBe(true);

    // Nor does it run once the old turn lands after all.
    landTurn("new-jumped-to");
    await flush();
    expect(stale).not.toHaveBeenCalled();
  });
});
