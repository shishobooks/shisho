import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { MultiSelectCombobox } from "./MultiSelectCombobox";

function makeHook(items: string[], isLoading = false) {
  return () => ({ data: items, isLoading });
}

describe("MultiSelectCombobox", () => {
  it("renders chips for selected values", () => {
    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook([])}
        label="Genre"
        onChange={vi.fn()}
        values={["Sci-Fi", "Fantasy"]}
      />,
    );

    expect(screen.getByText("Sci-Fi")).toBeInTheDocument();
    expect(screen.getByText("Fantasy")).toBeInTheDocument();
  });

  it("removes a chip when X is clicked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook([])}
        label="Genre"
        onChange={onChange}
        values={["Sci-Fi", "Fantasy"]}
      />,
    );

    await user.click(screen.getByLabelText(/Remove Sci-Fi/i));
    expect(onChange).toHaveBeenCalledWith(["Fantasy"]);
  });

  it("shows 'In your library' heading when options exist", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook(["Fantasy", "Horror"])}
        label="Genre"
        onChange={vi.fn()}
        values={[]}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    expect(screen.getByText("In your library")).toBeInTheDocument();
  });

  it("selects an option and calls onChange", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = vi.fn();

    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook(["Fantasy", "Horror"])}
        label="Genre"
        onChange={onChange}
        values={[]}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.click(screen.getByText("Fantasy"));
    expect(onChange).toHaveBeenCalledWith(["Fantasy"]);
  });

  it("excludes already-selected values from dropdown", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook(["Fantasy", "Horror"])}
        label="Genre"
        onChange={vi.fn()}
        values={["Fantasy"]}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    expect(
      screen.queryByRole("option", { name: "Fantasy" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Horror")).toBeInTheDocument();
  });

  it("resolves counts for selected values not in the initial item list", () => {
    interface Item {
      name: string;
      count: number;
    }

    const libraryItems: Item[] = [
      { name: "Adventure", count: 10 },
      { name: "Fiction", count: 54 },
      { name: "Horror", count: 8 },
    ];

    const beyondLimitItem: Item = { name: "Young Adult", count: 23 };

    function useItemSearch(query: string): {
      data: Item[];
      isLoading: boolean;
    } {
      if (!query) {
        return { data: libraryItems, isLoading: false };
      }
      const all = [...libraryItems, beyondLimitItem];
      const matches = all.filter((i) =>
        i.name.toLowerCase().includes(query.toLowerCase()),
      );
      return { data: matches, isLoading: false };
    }

    render(
      <MultiSelectCombobox<Item>
        getOptionCount={(i) => i.count}
        getOptionLabel={(i) => i.name}
        hook={useItemSearch}
        label="Genre"
        onChange={vi.fn()}
        values={["Fiction", "Young Adult"]}
      />,
    );

    const chips = screen.getAllByTestId("ms-chip");
    expect(chips[0]).toHaveTextContent("Fiction");
    expect(chips[0]).toHaveTextContent("(54)");
    expect(chips[1]).toHaveTextContent("Young Adult");
    expect(chips[1]).toHaveTextContent("(23)");
  });

  it("resolves counts for multiple missing values sequentially", async () => {
    interface Item {
      name: string;
      count: number;
    }

    const libraryItems: Item[] = [
      { name: "Adventure", count: 10 },
      { name: "Fiction", count: 54 },
    ];

    const beyondLimitItems: Item[] = [
      { name: "Science Fiction", count: 31 },
      { name: "Young Adult", count: 23 },
    ];

    function useItemSearch(query: string): {
      data: Item[];
      isLoading: boolean;
    } {
      if (!query) {
        return { data: libraryItems, isLoading: false };
      }
      const all = [...libraryItems, ...beyondLimitItems];
      const matches = all.filter((i) =>
        i.name.toLowerCase().includes(query.toLowerCase()),
      );
      return { data: matches, isLoading: false };
    }

    const { rerender } = render(
      <MultiSelectCombobox<Item>
        getOptionCount={(i) => i.count}
        getOptionLabel={(i) => i.name}
        hook={useItemSearch}
        label="Genre"
        onChange={vi.fn()}
        values={["Fiction", "Science Fiction", "Young Adult"]}
      />,
    );

    // Trigger re-render to allow useEffect advancement
    rerender(
      <MultiSelectCombobox<Item>
        getOptionCount={(i) => i.count}
        getOptionLabel={(i) => i.name}
        hook={useItemSearch}
        label="Genre"
        onChange={vi.fn()}
        values={["Fiction", "Science Fiction", "Young Adult"]}
      />,
    );

    const chips = screen.getAllByTestId("ms-chip");
    expect(chips[0]).toHaveTextContent("Fiction");
    expect(chips[0]).toHaveTextContent("(54)");
    expect(chips[1]).toHaveTextContent("Science Fiction");
    expect(chips[1]).toHaveTextContent("(31)");
    expect(chips[2]).toHaveTextContent("Young Adult");
    expect(chips[2]).toHaveTextContent("(23)");
  });

  it("shows Create option for unmatched search text", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook([])}
        label="Genre"
        onChange={vi.fn()}
        values={[]}
      />,
    );

    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText(/Search genre/i), "Thriller");
    expect(screen.getByText(/Create new genre "Thriller"/)).toBeInTheDocument();
  });
});
