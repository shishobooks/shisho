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

  it("has nothing to replay with reduced motion", () => {
    reducedMotion = true;
    render(<OrganizeAnimation />);

    expect(screen.getByRole("figure", { name: "Before" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Replay" })).toBeNull();
  });
});
