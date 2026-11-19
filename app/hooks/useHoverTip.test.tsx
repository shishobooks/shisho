import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useHoverTip } from "./useHoverTip";

const Squares = () => {
  const hoverTip = useHoverTip();
  return (
    <>
      <div data-testid="grid" onPointerLeave={hoverTip.hide}>
        {["a", "b"].map((name) => (
          <span
            data-testid={name}
            key={name}
            onPointerEnter={(event) =>
              hoverTip.show(event.currentTarget, `Square ${name}`)
            }
          />
        ))}
      </div>
      <span data-testid="elsewhere" />
      {hoverTip.tip}
    </>
  );
};

// What a phone sends for a tap: the pointer arrives, presses, lifts, and
// leaves the moment it lifts.
const tap = (el: Element) => {
  fireEvent.pointerEnter(el, { pointerType: "touch" });
  fireEvent.pointerDown(el, { pointerType: "touch" });
  fireEvent.pointerUp(el, { pointerType: "touch" });
  fireEvent.pointerLeave(el, { pointerType: "touch" });
  fireEvent.pointerLeave(screen.getByTestId("grid"), { pointerType: "touch" });
};

// jsdom has no layout: place an element at a rect, and say what sits at a
// point.
const placeAt = (el: Element, top: number) =>
  vi
    .spyOn(el, "getBoundingClientRect")
    .mockReturnValue(new DOMRect(100, top, 20, 20));
const topmost = (el: Element | null) => {
  document.elementFromPoint = () => el;
};

afterEach(() => {
  vi.restoreAllMocks();
  // @ts-expect-error jsdom doesn't define it; the tests add it.
  delete document.elementFromPoint;
});

describe("useHoverTip", () => {
  it("hides when a mouse leaves", () => {
    render(<Squares />);
    fireEvent.pointerEnter(screen.getByTestId("a"), { pointerType: "mouse" });
    expect(screen.getByRole("tooltip")).toHaveTextContent("Square a");
    fireEvent.pointerLeave(screen.getByTestId("grid"), {
      pointerType: "mouse",
    });
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("stays up after a tap, until the next tap lands somewhere else", () => {
    render(<Squares />);
    act(() => tap(screen.getByTestId("a")));
    expect(screen.getByRole("tooltip")).toHaveTextContent("Square a");

    act(() => tap(screen.getByTestId("b")));
    expect(screen.getByRole("tooltip")).toHaveTextContent("Square b");

    act(() => {
      fireEvent.pointerDown(screen.getByTestId("elsewhere"), {
        pointerType: "touch",
      });
    });
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("follows its target through a scroll that leaves it in view", () => {
    render(<Squares />);
    const a = screen.getByTestId("a");
    placeAt(a, 300);
    act(() => tap(a));
    expect(screen.getByRole("tooltip").style.top).toBe("294px");

    placeAt(a, 288);
    topmost(a);
    act(() => {
      fireEvent.scroll(window);
    });
    expect(screen.getByRole("tooltip").style.top).toBe("282px");
  });

  it("goes away when a scroll hides or covers its target", () => {
    render(<Squares />);
    const a = screen.getByTestId("a");
    placeAt(a, 300);
    act(() => tap(a));

    topmost(screen.getByTestId("elsewhere"));
    act(() => {
      fireEvent.scroll(window);
    });
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("goes away when a touch turns into a scroll", () => {
    render(<Squares />);
    const a = screen.getByTestId("a");
    act(() => {
      fireEvent.pointerEnter(a, { pointerType: "touch" });
      fireEvent.pointerDown(a, { pointerType: "touch" });
    });
    expect(screen.getByRole("tooltip")).toBeInTheDocument();
    act(() => {
      fireEvent.pointerCancel(a, { pointerType: "touch" });
      fireEvent.pointerLeave(screen.getByTestId("grid"), {
        pointerType: "touch",
      });
    });
    expect(screen.queryByRole("tooltip")).toBeNull();
  });
});
