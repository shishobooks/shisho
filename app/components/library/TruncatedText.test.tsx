import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import TruncatedText from "./TruncatedText";

const overflow = (scrollWidth: number) => {
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(
    scrollWidth,
  );
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(100);
};

afterEach(() => vi.restoreAllMocks());

describe("TruncatedText", () => {
  it("shows the full text in a tooltip when it is cut off", async () => {
    overflow(300);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(
      <TruncatedText className="truncate">A very long name</TruncatedText>,
    );

    await user.hover(screen.getByText("A very long name"));

    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "A very long name",
    );
  });

  it("shows no tooltip when the text fits", async () => {
    overflow(100);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<TruncatedText className="truncate">Short</TruncatedText>);

    await user.hover(screen.getByText("Short"));
    vi.advanceTimersByTime(1000);

    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("stays a controlled tooltip, so React does not warn as truncation changes", async () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => {});
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    overflow(300);
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(
      <TruncatedText className="truncate">A very long name</TruncatedText>,
    );

    await user.hover(screen.getByText("A very long name"));
    act(() => vi.advanceTimersByTime(400));
    await screen.findByRole("tooltip");

    const warnings = [...error.mock.calls, ...warn.mock.calls].flat().join(" ");
    expect(warnings).not.toContain("controlled to uncontrolled");
  });

  it("does not pop open when text that was hovered while it fit becomes cut off", async () => {
    let scrollWidth = 100;
    vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockImplementation(
      () => scrollWidth,
    );
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(100);
    let resize = () => {};
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(callback: () => void) {
          resize = callback;
        }
        observe() {}
        disconnect() {}
      },
    );
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<TruncatedText className="truncate">A name</TruncatedText>);

    await user.hover(screen.getByText("A name"));
    act(() => vi.advanceTimersByTime(400));
    await user.unhover(screen.getByText("A name"));

    // The window narrows and the name no longer fits, with no pointer on it.
    scrollWidth = 300;
    act(() => resize());
    act(() => vi.advanceTimersByTime(400));

    expect(screen.queryByRole("tooltip")).toBeNull();
    vi.unstubAllGlobals();
  });
});
