import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeAll, describe, expect, it, vi } from "vitest";

import { useDebounce } from "@/hooks/useDebounce";
import {
  advancePastSearch,
  createSearchQuery,
  SEARCH_LATENCY_MS,
} from "@/testing/searchQuery";
import type { Genre, Tag } from "@/types";
import type { TagScope } from "@/utils/tagNamespaces";

import { FilterSheet } from "./FilterSheet";

// jsdom has no matchMedia. Matching renders the Radix Sheet, which opens
// synchronously in jsdom, unlike the vaul Drawer.
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

const GENRES = [
  { id: 1, name: "Action", book_count: 1 },
  { id: 2, name: "Biography", book_count: 1 },
  { id: 3, name: "Fantasy", book_count: 1 },
] as unknown as Genre[];

// Renders the sheet over a real genre search, debounced as Home does, waits
// for the first results, and opens the genre picker.
async function openGenrePicker(
  user: ReturnType<typeof userEvent.setup>,
  onToggleGenre = vi.fn(),
) {
  const { useSearch, wrapper } = createSearchQuery(GENRES, (g) => g.name);
  const Harness = () => {
    const [search, setSearch] = useState("");
    const debounced = useDebounce(search, 300);
    const results = useSearch(debounced);
    return (
      <FilterSheet
        fileTypeOptions={[]}
        genreSearchInput={search}
        genres={results.data ?? []}
        genresLoading={results.isLoading}
        genresQuery={query}
        genresSearchPending={search !== debounced || results.isPlaceholderData}
        hasActiveFilters={false}
        languageOptions={[{ value: "en", label: "English" }]}
        languageParam=""
        onClearAll={vi.fn()}
        onGenreSearchChange={setSearch}
        onLanguageChange={vi.fn()}
        onRatingChange={vi.fn()}
        onReviewedFilterChange={vi.fn()}
        onTagScopeChange={vi.fn()}
        onTagSearchChange={vi.fn()}
        onToggleFileType={vi.fn()}
        onToggleGenre={onToggleGenre}
        onToggleReadingStatus={vi.fn()}
        onToggleTag={vi.fn()}
        reading={{ statuses: [] }}
        reviewedFilter=""
        selectedFileTypes={[]}
        selectedGenreIds={[]}
        selectedGenres={[]}
        selectedTagIds={[]}
        selectedTags={[]}
        tagNamespaces={[]}
        tagScope={{ kind: "all" }}
        tagSearchInput=""
        tags={[]}
        tagsLoading={false}
        tagsQuery={query}
        tagsSearchPending={false}
      />
    );
  };
  render(<Harness />, { wrapper });
  await act(() => vi.advanceTimersByTimeAsync(SEARCH_LATENCY_MS));
  await user.click(screen.getByRole("button", { name: "Filter" }));
  await user.click(screen.getByRole("combobox", { name: "Genres" }));
  return onToggleGenre;
}

const query = {
  data: [],
  error: null,
  isEnabled: true,
  isFetching: false,
  refetch: vi.fn(),
};

describe("FilterSheet", () => {
  it("highlights the first genre once results for the typed text arrive, so Enter toggles it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onToggleGenre = await openGenrePicker(user);

    await user.keyboard("fan");
    await advancePastSearch();
    await user.keyboard("{Enter}");

    expect(onToggleGenre).toHaveBeenCalledWith(3);
  });

  it("holds an Enter pressed before results arrive and toggles the first fresh genre", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onToggleGenre = await openGenrePicker(user);

    await user.keyboard("fan{Enter}");
    // Past the debounce, the previous results still show while the search
    // runs.
    await act(() => vi.advanceTimersByTimeAsync(250));
    expect(onToggleGenre).not.toHaveBeenCalled();
    await advancePastSearch();

    expect(onToggleGenre).toHaveBeenCalledExactlyOnceWith(3);
  });

  it("drops a held Enter when the genre picker closes", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onToggleGenre = await openGenrePicker(user);

    await user.keyboard("fan{Enter}");
    // The modal popover turns off pointer events outside it, which
    // userEvent refuses to click through unless told not to check.
    await user
      .setup({ pointerEventsCheck: 0 })
      .click(screen.getByRole("heading", { hidden: true, name: "Filters" }));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    await advancePastSearch();

    expect(onToggleGenre).not.toHaveBeenCalled();
  });

  it("names each filter control by its section, not the prompt or value shown", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(
      <FilterSheet
        fileTypeOptions={[]}
        genreSearchInput=""
        genres={[]}
        genresLoading={false}
        genresQuery={query}
        genresSearchPending={false}
        hasActiveFilters={false}
        languageOptions={[{ value: "en", label: "English" }]}
        languageParam=""
        onClearAll={vi.fn()}
        onGenreSearchChange={vi.fn()}
        onLanguageChange={vi.fn()}
        onRatingChange={vi.fn()}
        onReviewedFilterChange={vi.fn()}
        onTagScopeChange={vi.fn()}
        onTagSearchChange={vi.fn()}
        onToggleFileType={vi.fn()}
        onToggleGenre={vi.fn()}
        onToggleReadingStatus={vi.fn()}
        onToggleTag={vi.fn()}
        reading={{ statuses: [] }}
        reviewedFilter=""
        selectedFileTypes={[]}
        selectedGenreIds={[]}
        selectedGenres={[]}
        selectedTagIds={[]}
        selectedTags={[]}
        tagNamespaces={[]}
        tagScope={{ kind: "all" }}
        tagSearchInput=""
        tags={[]}
        tagsLoading={false}
        tagsQuery={query}
        tagsSearchPending={false}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Filter" }));

    expect(screen.getByRole("combobox", { name: "Tags" })).toHaveTextContent(
      "Add tags...",
    );
    expect(
      screen.getByRole("combobox", { name: "Language" }),
    ).toHaveTextContent("All Languages");
    await user.click(screen.getByRole("combobox", { name: "Genres" }));
    expect(
      screen.getByRole("combobox", { name: "Search genres" }),
    ).toBeInTheDocument();
  });

  it("picks file types from a combobox that shows what's chosen, names the review status group, and says which genres are chosen", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onToggleFileType = vi.fn();
    const genres = [
      { id: 1, name: "Fantasy", book_count: 1200 },
      { id: 2, name: "Horror", book_count: 1 },
    ] as unknown as Genre[];
    render(
      <FilterSheet
        fileTypeOptions={[
          { value: "epub", label: "EPUB" },
          { value: "cbz", label: "CBZ" },
        ]}
        genreSearchInput=""
        genres={genres}
        genresLoading={false}
        genresQuery={query}
        genresSearchPending={false}
        hasActiveFilters
        languageOptions={[]}
        languageParam=""
        onClearAll={vi.fn()}
        onGenreSearchChange={vi.fn()}
        onLanguageChange={vi.fn()}
        onRatingChange={vi.fn()}
        onReviewedFilterChange={vi.fn()}
        onTagScopeChange={vi.fn()}
        onTagSearchChange={vi.fn()}
        onToggleFileType={onToggleFileType}
        onToggleGenre={vi.fn()}
        onToggleReadingStatus={vi.fn()}
        onToggleTag={vi.fn()}
        reading={{ statuses: [] }}
        reviewedFilter="needs_review"
        selectedFileTypes={["epub"]}
        selectedGenreIds={[1]}
        selectedGenres={[genres[0]]}
        selectedTagIds={[]}
        selectedTags={[]}
        tagNamespaces={[]}
        tagScope={{ kind: "all" }}
        tagSearchInput=""
        tags={[]}
        tagsLoading={false}
        tagsQuery={query}
        tagsSearchPending={false}
      />,
    );
    await user.click(screen.getByRole("button", { name: /Filter/ }));

    const fileTypes = screen.getByRole("combobox", { name: "File type" });
    expect(fileTypes).toHaveTextContent("EPUB");
    await user.click(fileTypes);
    expect(screen.getByRole("option", { name: /EPUB/ })).toHaveTextContent(
      "(chosen)",
    );
    await user.click(screen.getByRole("option", { name: "CBZ" }));
    expect(onToggleFileType).toHaveBeenCalledWith("cbz");
    await user.keyboard("{Escape}");

    const review = screen.getByRole("radiogroup", { name: "Review status" });
    expect(
      within(review).getByRole("radio", { name: "Needs review" }),
    ).toBeChecked();

    await user.click(screen.getByRole("combobox", { name: "Genres" }));
    expect(screen.getByRole("option", { name: /Fantasy/ })).toHaveTextContent(
      "(chosen)",
    );
    expect(screen.getByRole("option", { name: /Fantasy/ })).toHaveTextContent(
      "1,200",
    );
    expect(
      screen.getByRole("option", { name: /Horror/ }),
    ).not.toHaveTextContent("(chosen)");
  });

  describe("tag namespaces", () => {
    const tags = [
      { id: 1, name: "Full Color", book_count: 3 },
      { id: 2, name: "Circle: Moon Factory", book_count: 2 },
    ] as unknown as Tag[];

    const renderWithNamespaces = (
      tagScope: TagScope,
      onTagScopeChange = vi.fn(),
      tagNamespaces = ["Circle", "Magazine"],
    ) =>
      render(
        <FilterSheet
          fileTypeOptions={[]}
          genreSearchInput=""
          genres={[]}
          genresLoading={false}
          genresQuery={query}
          genresSearchPending={false}
          hasActiveFilters={false}
          languageOptions={[]}
          languageParam=""
          onClearAll={vi.fn()}
          onGenreSearchChange={vi.fn()}
          onLanguageChange={vi.fn()}
          onRatingChange={vi.fn()}
          onReviewedFilterChange={vi.fn()}
          onTagScopeChange={onTagScopeChange}
          onTagSearchChange={vi.fn()}
          onToggleFileType={vi.fn()}
          onToggleGenre={vi.fn()}
          onToggleReadingStatus={vi.fn()}
          onToggleTag={vi.fn()}
          reading={{ statuses: [] }}
          reviewedFilter=""
          selectedFileTypes={[]}
          selectedGenreIds={[]}
          selectedGenres={[]}
          selectedTagIds={[]}
          selectedTags={[]}
          tagNamespaces={tagNamespaces}
          tagScope={tagScope}
          tagSearchInput=""
          tags={tags}
          tagsLoading={false}
          tagsQuery={query}
          tagsSearchPending={false}
        />,
      );

    it("offers a scope per namespace and reports the choice", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const onTagScopeChange = vi.fn();
      renderWithNamespaces({ kind: "all" }, onTagScopeChange);
      await user.click(screen.getByRole("button", { name: "Filter" }));
      await user.click(screen.getByRole("combobox", { name: "Tags" }));

      const scopes = screen.getByRole("group", { name: "Show tags from" });
      expect(
        within(scopes)
          .getAllByRole("button")
          .map((b) => b.textContent),
      ).toEqual(["All", "Tags", "Circle", "Magazine"]);
      expect(
        within(scopes).getByRole("button", { name: "All" }),
      ).toHaveAttribute("aria-pressed", "true");

      await user.click(within(scopes).getByRole("button", { name: "Circle" }));
      expect(onTagScopeChange).toHaveBeenCalledWith({
        kind: "namespace",
        namespace: "Circle",
      });
    });

    it("returns focus to the search box after a scope is chosen", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderWithNamespaces({ kind: "all" });
      await user.click(screen.getByRole("button", { name: "Filter" }));
      await user.click(screen.getByRole("combobox", { name: "Tags" }));

      await user.click(screen.getByRole("button", { name: "Circle" }));
      // Escape and typing then reach the picker, not the scope button.
      expect(
        screen.getByRole("combobox", { name: "Search tags" }),
      ).toHaveFocus();
    });

    it("closes the picker, not the sheet, on one Escape", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderWithNamespaces({ kind: "all" }, vi.fn(), []);
      await user.click(screen.getByRole("button", { name: "Filter" }));
      await user.click(screen.getByRole("combobox", { name: "Tags" }));
      expect(
        screen.getByRole("combobox", { name: "Search tags" }),
      ).toBeInTheDocument();

      await user.keyboard("{Escape}");
      expect(
        screen.queryByRole("combobox", { name: "Search tags" }),
      ).not.toBeInTheDocument();
      expect(
        screen.getByRole("dialog", { name: "Filters" }),
      ).toBeInTheDocument();
    });

    it("labels a namespaced tag with its namespace under All", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderWithNamespaces({ kind: "all" });
      await user.click(screen.getByRole("button", { name: "Filter" }));
      await user.click(screen.getByRole("combobox", { name: "Tags" }));

      const option = screen.getByRole("option", { name: /Moon Factory/ });
      expect(option).toHaveTextContent("Circle");
      expect(option).not.toHaveTextContent("Circle:");
    });

    it("shows a namespace's tags without the prefix", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderWithNamespaces({ kind: "namespace", namespace: "Circle" });
      await user.click(screen.getByRole("button", { name: "Filter" }));
      await user.click(screen.getByRole("combobox", { name: "Tags" }));

      expect(
        screen.getByRole("combobox", { name: "Search tags" }),
      ).toHaveAttribute("placeholder", "Search Circle...");
      const option = screen.getByRole("option", { name: /Moon Factory/ });
      expect(option.textContent).toMatch(/^Moon Factory/);
    });

    it("has no scope switch when the library has no namespaces", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      renderWithNamespaces({ kind: "all" }, vi.fn(), []);
      await user.click(screen.getByRole("button", { name: "Filter" }));
      await user.click(screen.getByRole("combobox", { name: "Tags" }));

      expect(
        screen.queryByRole("group", { name: "Show tags from" }),
      ).not.toBeInTheDocument();
      expect(
        screen.getByRole("option", { name: /Circle: Moon Factory/ }),
      ).toBeInTheDocument();
    });
  });
});
