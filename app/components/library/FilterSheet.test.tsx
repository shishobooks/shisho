import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it, vi } from "vitest";

import type { Genre } from "@/types";

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
      { id: 1, name: "Fantasy", book_count: 3 },
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
    expect(
      screen.getByRole("option", { name: /Horror/ }),
    ).not.toHaveTextContent("(chosen)");
  });
});
