import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it, vi } from "vitest";

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

const query = {
  data: [],
  error: null,
  isEnabled: true,
  isFetching: false,
  refetch: vi.fn(),
};

describe("FilterSheet", () => {
  it("names each filter control by its section, not the prompt or value shown", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(
      <FilterSheet
        fileTypeOptions={[]}
        genreSearchInput=""
        genres={[]}
        genresLoading={false}
        genresQuery={query}
        hasActiveFilters={false}
        languageOptions={[{ value: "en", label: "English" }]}
        languageParam=""
        onClearAll={vi.fn()}
        onGenreSearchChange={vi.fn()}
        onLanguageChange={vi.fn()}
        onReviewedFilterChange={vi.fn()}
        onTagScopeChange={vi.fn()}
        onTagSearchChange={vi.fn()}
        onToggleFileType={vi.fn()}
        onToggleGenre={vi.fn()}
        onToggleTag={vi.fn()}
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

  it("names the file type and review state groups and says which genres are chosen", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
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
        hasActiveFilters
        languageOptions={[]}
        languageParam=""
        onClearAll={vi.fn()}
        onGenreSearchChange={vi.fn()}
        onLanguageChange={vi.fn()}
        onReviewedFilterChange={vi.fn()}
        onTagScopeChange={vi.fn()}
        onTagSearchChange={vi.fn()}
        onToggleFileType={vi.fn()}
        onToggleGenre={vi.fn()}
        onToggleTag={vi.fn()}
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
      />,
    );
    await user.click(screen.getByRole("button", { name: /Filter/ }));

    const fileTypes = screen.getByRole("group", { name: "File type" });
    expect(
      within(fileTypes).getByRole("button", { name: "EPUB" }),
    ).toHaveAttribute("aria-pressed", "true");
    const review = screen.getByRole("radiogroup", { name: "Review state" });
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
          hasActiveFilters={false}
          languageOptions={[]}
          languageParam=""
          onClearAll={vi.fn()}
          onGenreSearchChange={vi.fn()}
          onLanguageChange={vi.fn()}
          onReviewedFilterChange={vi.fn()}
          onTagScopeChange={onTagScopeChange}
          onTagSearchChange={vi.fn()}
          onToggleFileType={vi.fn()}
          onToggleGenre={vi.fn()}
          onToggleTag={vi.fn()}
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
