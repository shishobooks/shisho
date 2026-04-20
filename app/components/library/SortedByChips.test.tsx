import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { SortLevel } from "@/libraries/sortSpec";

import SortedByChips from "./SortedByChips";

describe("SortedByChips", () => {
  it("renders a chip per level", () => {
    const levels: SortLevel[] = [
      { field: "author", direction: "asc" },
      { field: "series", direction: "asc" },
    ];
    render(
      <SortedByChips
        levels={levels}
        onRemoveLevel={vi.fn()}
        onReset={vi.fn()}
      />,
    );
    expect(screen.getByText(/Author/)).toBeInTheDocument();
    expect(screen.getByText(/Series/)).toBeInTheDocument();
  });

  it("renders ascending arrow for asc and descending for desc", () => {
    render(
      <SortedByChips
        levels={[
          { field: "title", direction: "asc" },
          { field: "date_added", direction: "desc" },
        ]}
        onRemoveLevel={vi.fn()}
        onReset={vi.fn()}
      />,
    );
    // arrows come from lucide-react (ArrowUp/ArrowDown SVGs). They render
    // as inline <svg> with no text content, so asserting on text is
    // brittle. Instead assert both chips are present — the direction is
    // reflected in the aria-label, not visible text.
    expect(
      screen.getByRole("button", { name: /Title ascending/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Date added descending/i }),
    ).toBeInTheDocument();
  });

  it("clicking a chip removes its level", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onRemoveLevel = vi.fn();
    render(
      <SortedByChips
        levels={[
          { field: "title", direction: "asc" },
          { field: "author", direction: "asc" },
        ]}
        onRemoveLevel={onRemoveLevel}
        onReset={vi.fn()}
      />,
    );
    await user.click(screen.getByRole("button", { name: /Title/i }));
    expect(onRemoveLevel).toHaveBeenCalledWith(0);
  });

  it("clicking reset fires onReset", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onReset = vi.fn();
    render(
      <SortedByChips
        levels={[{ field: "title", direction: "asc" }]}
        onRemoveLevel={vi.fn()}
        onReset={onReset}
      />,
    );
    await user.click(screen.getByRole("button", { name: /reset to default/i }));
    expect(onReset).toHaveBeenCalled();
  });

  it("renders nothing when levels is empty", () => {
    const { container } = render(
      <SortedByChips levels={[]} onRemoveLevel={vi.fn()} onReset={vi.fn()} />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
