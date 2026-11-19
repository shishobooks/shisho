import { act, fireEvent, render, screen } from "@testing-library/react";
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

  it("shows every example file, a comic and a book with two files among them", () => {
    reducedMotion = true;
    render(<OrganizeAnimation />);

    const after = screen.getByRole("figure", { name: "After" });
    // One book's EPUB and M4B land in its one folder.
    expect(after).toHaveTextContent("[Jane Austen] Pride and Prejudice/");
    expect(after).toHaveTextContent("Pride and Prejudice.epub");
    expect(after).toHaveTextContent("Pride and Prejudice.m4b");
    expect(after).toHaveTextContent("[Winsor McCay] Little Nemo v001/");
    expect(after).toHaveTextContent("Little Nemo.cbz");
    const before = screen.getByRole("figure", { name: "Before" });
    expect(before).toHaveTextContent("little_nemo_vol01.cbz");
    expect(before).toHaveTextContent("P&P audiobook.m4b");
  });

  it("plays once, then offers to play it again", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { container } = render(<OrganizeAnimation />);
    expect(
      screen.getByRole("img", { name: /files in a messy folder move/i }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Replay" })).toBeNull();

    finishPlaying(container);
    const replay = screen.getByRole("button", { name: "Replay" });

    // Replaying starts the example over from the messy folder.
    const before = container.querySelector("[data-organize-last-step]");
    await user.click(replay);
    expect(screen.queryByRole("button", { name: "Replay" })).toBeNull();
    expect(container.querySelector("[data-organize-last-step]")).not.toBe(
      before,
    );
  });

  it("holds the messy folder for 2 seconds before anything changes", () => {
    const { container } = render(<OrganizeAnimation />);

    // When each animation (app/index.css) first changes what's on screen, as
    // a share of its 3s: the messy folders start fading, files start moving,
    // names start swapping, book folders start appearing.
    const firstChange: Record<string, number> = {
      "setup-folder-out": 0.12,
      "setup-file-move": 0.2,
      "setup-name-before": 0.55,
      "setup-name-after": 0.62,
      "setup-folder-in": 0.55,
    };
    const changes = [
      ...container.querySelectorAll<HTMLElement>("[class*='animate-[setup-']"),
    ].map((el) => {
      const name = /animate-\[(setup-[a-z-]+)_/.exec(el.className)?.[1] ?? "";
      expect(firstChange[name], name).toBeDefined();
      return parseFloat(el.style.animationDelay) + firstChange[name] * 3;
    });
    expect(changes.length).toBeGreaterThan(0);
    expect(Math.min(...changes)).toBeCloseTo(2, 5);
  });

  it("waits until it's in view to play", () => {
    let report: IntersectionObserverCallback = () => {};
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        constructor(callback: IntersectionObserverCallback) {
          report = callback;
        }
        observe() {}
        disconnect() {}
      },
    );
    try {
      render(<OrganizeAnimation />);
      const example = screen.getByRole("img", {
        name: /files in a messy folder move/i,
      });
      expect(example.className).toContain("animation-play-state:paused");

      act(() =>
        report(
          [{ isIntersecting: true } as IntersectionObserverEntry],
          {} as IntersectionObserver,
        ),
      );
      expect(example.className).not.toContain("animation-play-state:paused");
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it("gives the focus back to Replay once a replay ends", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { container } = render(<OrganizeAnimation />);
    finishPlaying(container);

    await user.click(screen.getByRole("button", { name: "Replay" }));
    expect(document.body).not.toHaveFocus();
    finishPlaying(container);

    expect(screen.getByRole("button", { name: "Replay" })).toHaveFocus();
  });

  it("has nothing to replay with reduced motion", () => {
    reducedMotion = true;
    render(<OrganizeAnimation />);

    expect(screen.getByRole("figure", { name: "Before" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Replay" })).toBeNull();
  });
});
