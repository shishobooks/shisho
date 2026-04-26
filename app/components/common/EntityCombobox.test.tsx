import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { EntityCombobox } from "./EntityCombobox";

interface Person {
  id: number;
  name: string;
}

function makeHook(items: Person[], isLoading = false) {
  return () => ({ data: items, isLoading });
}

describe("EntityCombobox", () => {
  it("calls onChange with an existing match when the user selects it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    const hook = makeHook([{ id: 1, name: "Tor Books" }]);

    render(
      <EntityCombobox<Person>
        getOptionLabel={(p) => p.name}
        hook={hook}
        label="Publisher"
        onChange={onChange}
        value={null}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByText("Tor Books"));

    expect(onChange).toHaveBeenCalledWith({ id: 1, name: "Tor Books" });
  });

  it("offers Create when typed value has no match and emits __create payload", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();
    const hook = makeHook([]);

    render(
      <EntityCombobox<Person>
        getOptionLabel={(p) => p.name}
        hook={hook}
        label="Publisher"
        onChange={onChange}
        value={null}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.type(
      screen.getByPlaceholderText(/Search publisher/i),
      "Penguin",
    );
    await user.click(screen.getByText(/Create "Penguin"/));

    expect(onChange).toHaveBeenCalledWith({ __create: "Penguin" });
  });

  it("hides items returned by exclude predicate", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const hook = makeHook([
      { id: 1, name: "A" },
      { id: 2, name: "B" },
    ]);

    render(
      <EntityCombobox<Person>
        exclude={(p) => p.name === "B"}
        getOptionLabel={(p) => p.name}
        hook={hook}
        label="Person"
        onChange={vi.fn()}
        value={null}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    expect(screen.getByText("A")).toBeInTheDocument();
    expect(screen.queryByText("B")).not.toBeInTheDocument();
  });

  it("hides Create CTA when canCreate=false", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const hook = makeHook([]);

    render(
      <EntityCombobox<Person>
        canCreate={false}
        getOptionLabel={(p) => p.name}
        hook={hook}
        label="Person"
        onChange={vi.fn()}
        value={null}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText(/Search person/i), "X");

    expect(screen.queryByText(/Create "X"/)).not.toBeInTheDocument();
  });

  it("renders status badge when status prop set", () => {
    const hook = makeHook([]);

    render(
      <EntityCombobox<Person>
        getOptionLabel={(p) => p.name}
        hook={hook}
        label="Person"
        onChange={vi.fn()}
        status="new"
        value={{ id: 1, name: "X" }}
      />,
    );

    expect(screen.getByTestId("entity-status-badge")).toHaveTextContent(/new/i);
  });

  it("shows loading state when hook reports isLoading", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const hook = makeHook([], true);

    render(
      <EntityCombobox<Person>
        getOptionLabel={(p) => p.name}
        hook={hook}
        label="Person"
        onChange={vi.fn()}
        value={null}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    expect(screen.getByText(/Loading/i)).toBeInTheDocument();
  });
});
