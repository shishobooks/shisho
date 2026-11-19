import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OrganizeAnimation } from "./OrganizeAnimation";

let reducedMotion = false;

const finishPlaying = (container: HTMLElement) => {
  const last = container.querySelector("[data-organize-last-step]");
  if (!last) throw new Error("no last step to finish");
  fireEvent.animationEnd(last);
};

describe("OrganizeAnimation", () => {
  beforeEach(() => {
    reducedMotion = false;
    window.matchMedia = vi.fn((query: string) => ({
      matches: query.includes("prefers-reduced-motion") && reducedMotion,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })) as unknown as typeof window.matchMedia;
  });

  afterEach(() => {
    delete (window as { matchMedia?: unknown }).matchMedia;
  });

  it("plays once, then offers to play it again", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { container } = render(<OrganizeAnimation />);
    expect(
      screen.getByRole("img", { name: /files in a messy folder move/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Replay the example" }),
    ).toBeNull();

    finishPlaying(container);
    const replay = screen.getByRole("button", { name: "Replay the example" });

    // Replaying starts the example over from the messy folder.
    const before = container.querySelector("[data-organize-last-step]");
    await user.click(replay);
    expect(
      screen.queryByRole("button", { name: "Replay the example" }),
    ).toBeNull();
    expect(container.querySelector("[data-organize-last-step]")).not.toBe(
      before,
    );
  });

  it("has nothing to replay with reduced motion", () => {
    reducedMotion = true;
    render(<OrganizeAnimation />);

    expect(screen.getByRole("figure", { name: "Before" })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Replay the example" }),
    ).toBeNull();
  });
});
