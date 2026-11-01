import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import {
  advancePastSearch,
  createSearchQuery,
  SEARCH_LATENCY_MS,
} from "@/testing/searchQuery";

import { MultiSelectCombobox } from "./MultiSelectCombobox";

interface NameWithCount {
  name: string;
  book_count: number;
}

function makeHook(items: string[], isLoading = false) {
  return () => ({
    data: items,
    isLoading,
    isPlaceholderData: false,
    isError: false,
  });
}

function makeCountHook(items: NameWithCount[], isLoading = false) {
  return () => ({
    data: items,
    isLoading,
    isPlaceholderData: false,
    isError: false,
  });
}

const TAGS = ["Action", "Fantasy", "Horror", "Urban Fantasy"];

// Renders the combobox over a real search query and waits for the first
// results, as the edit dialogs do.
async function renderSearching(onChange = vi.fn()) {
  const { useSearch, wrapper } = createSearchQuery(TAGS, (s) => s);
  render(
    <MultiSelectCombobox
      getOptionLabel={(s) => s}
      hook={useSearch}
      label="Tag"
      onChange={onChange}
      values={[]}
    />,
    { wrapper },
  );
  await act(() => vi.advanceTimersByTimeAsync(SEARCH_LATENCY_MS));
  return onChange;
}

describe("MultiSelectCombobox", () => {
  it("highlights the first option once results for the typed text arrive, so Enter adds it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = await renderSearching();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan");
    await advancePastSearch();
    expect(screen.getByRole("option", { name: "Fantasy" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await user.keyboard("{Enter}");
    expect(onChange).toHaveBeenCalledWith(["Fantasy"]);
  });

  it("holds an Enter pressed before results arrive and adds the first fresh option", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = await renderSearching();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}");
    // Past the debounce, the previous results still show while the search
    // runs.
    await act(() => vi.advanceTimersByTimeAsync(250));
    expect(onChange).not.toHaveBeenCalled();
    await advancePastSearch();
    expect(onChange).toHaveBeenCalledExactlyOnceWith(["Fantasy"]);
  });

  it("drops a held Enter when the user keeps typing", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = await renderSearching();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}t");
    await advancePastSearch();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("drops a held Enter when the user clicks an option instead", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = await renderSearching();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}");
    await user.click(screen.getByRole("option", { name: "Horror" }));
    await advancePastSearch();

    expect(onChange).toHaveBeenCalledExactlyOnceWith(["Horror"]);
  });

  it("drops a held Enter when the dropdown closes", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = await renderSearching();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}");
    // The modal popover turns off pointer events outside it, which
    // userEvent refuses to click through unless told not to check.
    await user.setup({ pointerEventsCheck: 0 }).click(document.body);
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    await advancePastSearch();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("holds an Enter pressed before results arrive and creates the typed tag when none match", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = await renderSearching();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("cozy{Enter}");
    await advancePastSearch();
    expect(onChange).toHaveBeenCalledExactlyOnceWith(["cozy"]);
  });

  it("highlights the create option when nothing matches the typed text", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = await renderSearching();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("cozy");
    await advancePastSearch();
    await user.keyboard("{Enter}");
    expect(onChange).toHaveBeenCalledWith(["cozy"]);
  });

  it("names the trigger and its search box by the field, not the prompt shown", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    render(
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={makeHook(["Sci-Fi"])}
        label="Genre"
        onChange={vi.fn()}
        placeholder="Add genres..."
        values={[]}
      />,
    );

    const trigger = screen.getByRole("combobox", { name: "Genre" });
    expect(trigger).toHaveTextContent("Add genres...");
    await user.click(trigger);
    expect(
      screen.getByRole("combobox", { name: "Search genre" }),
    ).toBeInTheDocument();
  });

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

  it("displays counts for selected values not in the initial hook results", () => {
    const hookItems: NameWithCount[] = [
      { name: "Adventure", book_count: 10 },
      { name: "Fantasy", book_count: 5 },
    ];

    render(
      <MultiSelectCombobox<NameWithCount>
        getOptionCount={(g) => g.book_count}
        getOptionLabel={(g) => g.name}
        hook={makeCountHook(hookItems)}
        label="Genre"
        onChange={vi.fn()}
        useSelectedItemCounts={() =>
          new Map([
            ["Young Adult", 22],
            ["Science Fiction", 15],
          ])
        }
        values={["Fantasy", "Young Adult", "Science Fiction"]}
      />,
    );

    const chips = screen.getAllByTestId("ms-chip");
    expect(chips[0]).toHaveTextContent("Fantasy");
    expect(chips[0]).toHaveTextContent("(5)");
    expect(chips[1]).toHaveTextContent("Young Adult");
    expect(chips[1]).toHaveTextContent("(22)");
    expect(chips[2]).toHaveTextContent("Science Fiction");
    expect(chips[2]).toHaveTextContent("(15)");
  });
});
