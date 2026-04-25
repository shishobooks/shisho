import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it, vi } from "vitest";

import { Button } from "@/components/ui/button";
import type { SortLevel } from "@/libraries/sortSpec";

import SortSheet from "./SortSheet";

// jsdom does not implement window.matchMedia. Mock it to return matches=true
// so useMediaQuery("(min-width: 768px)") returns true and the Sheet (Radix
// Dialog-based) branch is rendered. The Radix Sheet opens synchronously in
// jsdom, whereas the vaul Drawer relies on native animations that don't fire
// under jsdom.
beforeAll(() => {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
});

// Trigger must forward props so that SheetTrigger asChild can merge its
// onClick onto the underlying button element.
const Trigger = (props: React.ComponentPropsWithoutRef<typeof Button>) => (
  <Button {...props}>Open Sort</Button>
);

describe("SortSheet", () => {
  it("adds a sort level when the first-field add button is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    render(
      <SortSheet
        isDirty={false}
        isSaving={false}
        levels={[]}
        onChange={onChange}
        onSaveAsDefault={vi.fn()}
        trigger={<Trigger />}
      />,
    );
    await user.click(screen.getByRole("button", { name: /Open Sort/i }));
    await user.click(screen.getByRole("button", { name: /Title/i }));
    expect(onChange).toHaveBeenCalledWith([
      { field: "title", direction: "asc" },
    ]);
  });

  it("toggles direction when the arrow button is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    const levels: SortLevel[] = [{ field: "title", direction: "asc" }];
    render(
      <SortSheet
        isDirty={true}
        isSaving={false}
        levels={levels}
        onChange={onChange}
        onSaveAsDefault={vi.fn()}
        trigger={<Trigger />}
      />,
    );
    await user.click(screen.getByRole("button", { name: /Open Sort/i }));
    await user.click(
      screen.getByRole("button", { name: /Direction: ascending/i }),
    );
    expect(onChange).toHaveBeenCalledWith([
      { field: "title", direction: "desc" },
    ]);
  });

  it("removes a level when the X is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    const levels: SortLevel[] = [
      { field: "title", direction: "asc" },
      { field: "author", direction: "asc" },
    ];
    render(
      <SortSheet
        isDirty={false}
        isSaving={false}
        levels={levels}
        onChange={onChange}
        onSaveAsDefault={vi.fn()}
        trigger={<Trigger />}
      />,
    );
    await user.click(screen.getByRole("button", { name: /Open Sort/i }));
    await user.click(
      screen.getByRole("button", { name: /Remove Title sort level/i }),
    );
    expect(onChange).toHaveBeenCalledWith([
      { field: "author", direction: "asc" },
    ]);
  });

  it("fires onSaveAsDefault when Save as default clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onSaveAsDefault = vi.fn();
    const levels: SortLevel[] = [{ field: "title", direction: "asc" }];
    render(
      <SortSheet
        isDirty={true}
        isSaving={false}
        levels={levels}
        onChange={vi.fn()}
        onSaveAsDefault={onSaveAsDefault}
        trigger={<Trigger />}
      />,
    );
    await user.click(screen.getByRole("button", { name: /Open Sort/i }));
    await user.click(
      screen.getByRole("button", { name: /Save as my default/i }),
    );
    expect(onSaveAsDefault).toHaveBeenCalled();
  });

  it("hides 'Save as default' when not dirty", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const levels: SortLevel[] = [{ field: "title", direction: "asc" }];
    render(
      <SortSheet
        isDirty={false}
        isSaving={false}
        levels={levels}
        onChange={vi.fn()}
        onSaveAsDefault={vi.fn()}
        trigger={<Trigger />}
      />,
    );
    await user.click(screen.getByRole("button", { name: /Open Sort/i }));
    expect(
      screen.queryByRole("button", { name: /Save as default/i }),
    ).not.toBeInTheDocument();
  });
});
