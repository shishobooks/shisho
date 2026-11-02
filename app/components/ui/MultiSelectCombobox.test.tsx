import { onlineManager } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";

import {
  advancePastFailedSearch,
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

// Keeps the chosen values as a form does, so each change builds on the last.
function renderForm(fails?: (query: string) => boolean) {
  const { useSearch, wrapper } = createSearchQuery(TAGS, (s) => s, fails);
  function Form() {
    const [values, setValues] = useState<string[]>([]);
    return (
      <MultiSelectCombobox
        getOptionLabel={(s) => s}
        hook={useSearch}
        label="Tag"
        onChange={setValues}
        values={values}
      />
    );
  }
  render(<Form />, { wrapper });
  return act(() => vi.advanceTimersByTimeAsync(SEARCH_LATENCY_MS));
}

async function renderReporting(onPendingChange: (pending: boolean) => void) {
  const { useSearch, wrapper } = createSearchQuery(TAGS, (s) => s);
  const view = render(
    <MultiSelectCombobox
      getOptionLabel={(s) => s}
      hook={useSearch}
      label="Tag"
      onChange={vi.fn()}
      onPendingChange={onPendingChange}
      values={[]}
    />,
    { wrapper },
  );
  await act(() => vi.advanceTimersByTimeAsync(SEARCH_LATENCY_MS));
  return view;
}

const chips = () =>
  screen.queryAllByTestId("ms-chip").map((chip) => chip.textContent);

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

  it("clears the box on an early Enter, so the next letters start a new search", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}");
    expect(screen.getByRole("combobox", { name: /search/i })).toHaveValue("");
    await user.keyboard("t");
    expect(screen.getByRole("combobox", { name: /search/i })).toHaveValue("t");
    await advancePastSearch();

    expect(chips()).toEqual(["Fantasy"]);
  });

  it("adds every tag when each next one is typed before the last search answers", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}hor{Enter}cozy{Enter}");
    await advancePastSearch();

    expect(chips()).toEqual(["Fantasy", "Horror", "cozy"]);
  });

  it("ignores an Enter on the empty box while an earlier one is waiting", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}{Enter}");
    await advancePastSearch();

    expect(chips()).toEqual(["Fantasy"]);
  });

  it("keeps an early Enter when the user also clicks an option", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}");
    await user.click(screen.getByRole("option", { name: "Horror" }));
    await advancePastSearch();

    expect(chips()).toEqual(["Horror", "Fantasy"]);
  });

  it("keeps an early Enter when the dropdown closes", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}");
    // The modal popover turns off pointer events outside it, which
    // userEvent refuses to click through unless told not to check.
    await user.setup({ pointerEventsCheck: 0 }).click(document.body);
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    await advancePastSearch();

    expect(chips()).toEqual(["Fantasy"]);
  });

  it("reports an early Enter whose search fails, and keeps the others", async () => {
    const toastError = vi.spyOn(toast, "error");
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm((query) => query === "fan");

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}hor{Enter}");
    await advancePastFailedSearch();

    expect(chips()).toEqual(["Horror"]);
    expect(toastError).toHaveBeenCalledExactlyOnceWith(
      'Failed to search for "fan"',
      undefined,
    );
    toastError.mockRestore();
  });

  it("holds an Enter pressed before results arrive and creates the typed tag when none match", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onChange = await renderSearching();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("cozy{Enter}");
    await advancePastSearch();
    expect(onChange).toHaveBeenCalledExactlyOnceWith(["cozy"]);
  });

  it("leaves the empty box unhighlighted, so Enter adds nothing", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    expect(screen.getByRole("option", { name: "Action" })).toHaveAttribute(
      "aria-selected",
      "false",
    );
    await user.keyboard("{Enter}");
    await advancePastSearch();

    expect(chips()).toEqual([]);
  });

  it("adds nothing on another Enter after a tag was added", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan");
    await advancePastSearch();
    await user.keyboard("{Enter}");
    await advancePastSearch();
    await user.keyboard("{Enter}");
    await advancePastSearch();

    expect(chips()).toEqual(["Fantasy"]);
  });

  it("adds nothing when the typed text is deleted again", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("a");
    await advancePastSearch();
    await user.keyboard("{Backspace}");
    await advancePastSearch();
    await user.keyboard("{Enter}");
    await advancePastSearch();

    expect(chips()).toEqual([]);
  });

  it("adds the option an arrow key moved to on the empty box", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("option", { name: "Action" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await user.keyboard("{ArrowDown}{Enter}");
    await advancePastSearch();

    expect(chips()).toEqual(["Fantasy"]);
    // The list changed, so the next Enter needs a new choice.
    await user.keyboard("{Enter}");
    await advancePastSearch();
    expect(chips()).toEqual(["Fantasy"]);
  });

  it("reports while an early Enter is waiting for its search", async () => {
    const onPendingChange = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderReporting(onPendingChange);

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}");
    expect(onPendingChange).toHaveBeenLastCalledWith(true);
    await advancePastSearch();
    expect(onPendingChange).toHaveBeenLastCalledWith(false);
  });

  it("stops reporting a wait when it unmounts mid-search", async () => {
    const onPendingChange = vi.fn();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { unmount } = await renderReporting(onPendingChange);

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}");
    unmount();

    expect(onPendingChange).toHaveBeenLastCalledWith(false);
  });

  it("adds nothing more when an early Enter's match was already added", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}");
    await user.click(screen.getByRole("option", { name: "Fantasy" }));
    await advancePastSearch();

    expect(chips()).toEqual(["Fantasy"]);
  });

  it("adds a retyped early Enter once, not its next match too", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}fan{Enter}");
    await advancePastSearch();

    expect(chips()).toEqual(["Fantasy"]);
  });

  it("shows what an early Enter is still adding", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan{Enter}");
    expect(screen.getByTestId("ms-pending-chip")).toHaveTextContent("fan");
    await advancePastSearch();

    expect(screen.queryByTestId("ms-pending-chip")).not.toBeInTheDocument();
  });

  it("waits out being offline instead of adding the typed text", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();
    await user.click(screen.getByRole("combobox"));

    onlineManager.setOnline(false);
    try {
      await user.keyboard("fan{Enter}");
      await advancePastSearch();
      expect(chips()).toEqual([]);
    } finally {
      await act(async () => onlineManager.setOnline(true));
    }
    await advancePastSearch();

    expect(chips()).toEqual(["Fantasy"]);
  });

  it("adds an option the pointer moved to on the empty box", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.hover(screen.getByRole("option", { name: "Horror" }));
    await user.keyboard("{Enter}");
    await advancePastSearch();

    expect(chips()).toEqual(["Horror"]);
  });

  it("forgets a hovered option when the dropdown closes", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.hover(screen.getByRole("option", { name: "Horror" }));
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("combobox"));
    await user.keyboard("{Enter}");
    await advancePastSearch();

    expect(chips()).toEqual([]);
  });

  it("moves to an option with cmdk's Ctrl bindings on the empty box", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("{Control>}j{/Control}{Enter}");
    await advancePastSearch();

    expect(chips()).toEqual(["Action"]);
  });

  it("adds an arrowed-to option right after the last add", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await renderForm();

    await user.click(screen.getByRole("combobox"));
    await user.keyboard("fan");
    await advancePastSearch();
    await user.keyboard("{Enter}{ArrowDown}{Enter}");
    await advancePastSearch();

    expect(chips()).toEqual(["Fantasy", "Urban Fantasy"]);
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
