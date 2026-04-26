import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SortableEntityList } from "./SortableEntityList";

interface Person {
  id: number;
  name: string;
}

const HOOK = () => ({ data: [] as Person[], isLoading: false });

describe("SortableEntityList", () => {
  it("renders one row per item with the entity label", () => {
    render(
      <SortableEntityList<Person>
        comboboxProps={{
          getOptionLabel: (p) => p.name,
          hook: HOOK,
          label: "Person",
        }}
        items={[
          { id: 1, name: "Alice" },
          { id: 2, name: "Bob" },
        ]}
        onAppend={vi.fn()}
        onRemove={vi.fn()}
        onReorder={vi.fn()}
      />,
    );

    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
  });

  it("calls onRemove with the row index when remove button clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onRemove = vi.fn();
    render(
      <SortableEntityList<Person>
        comboboxProps={{
          getOptionLabel: (p) => p.name,
          hook: HOOK,
          label: "Person",
        }}
        items={[{ id: 1, name: "Alice" }]}
        onAppend={vi.fn()}
        onRemove={onRemove}
        onReorder={vi.fn()}
      />,
    );

    await user.click(screen.getByLabelText(/Remove Alice/i));
    expect(onRemove).toHaveBeenCalledWith(0);
  });

  it("renders renderExtras output adjacent to each row", () => {
    render(
      <SortableEntityList<Person>
        comboboxProps={{
          getOptionLabel: (p) => p.name,
          hook: HOOK,
          label: "Person",
        }}
        items={[{ id: 1, name: "Alice" }]}
        onAppend={vi.fn()}
        onRemove={vi.fn()}
        onReorder={vi.fn()}
        renderExtras={(item, idx) => (
          <span data-testid="extras">
            extra-{item.name}-{idx}
          </span>
        )}
      />,
    );

    expect(screen.getByTestId("extras")).toHaveTextContent("extra-Alice-0");
  });

  it("resolves per-row status via the status prop", () => {
    render(
      <SortableEntityList<Person>
        comboboxProps={{
          getOptionLabel: (p) => p.name,
          hook: HOOK,
          label: "Person",
        }}
        items={[
          { id: 1, name: "A" },
          { id: 2, name: "B" },
        ]}
        onAppend={vi.fn()}
        onRemove={vi.fn()}
        onReorder={vi.fn()}
        status={(_, idx) => (idx === 0 ? "new" : "unchanged")}
      />,
    );

    const badges = screen.getAllByTestId("entity-status-badge");
    expect(badges[0]).toHaveTextContent(/new/i);
    expect(badges[1]).toHaveTextContent(/unchanged/i);
  });

  it("forwards onAppend when the embedded combobox emits a value", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onAppend = vi.fn();
    const hook = () => ({
      data: [{ id: 9, name: "Carol" }],
      isLoading: false,
    });

    render(
      <SortableEntityList<Person>
        comboboxProps={{ getOptionLabel: (p) => p.name, hook, label: "Person" }}
        items={[]}
        onAppend={onAppend}
        onRemove={vi.fn()}
        onReorder={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByText("Carol"));

    expect(onAppend).toHaveBeenCalledWith({ id: 9, name: "Carol" });
  });
});
