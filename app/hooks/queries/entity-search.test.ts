import { describe, expect, it, vi } from "vitest";

import {
  useGenreSearch,
  useImprintSearch,
  usePeopleSearch,
  usePublisherSearch,
  useSeriesSearch,
  useTagSearch,
} from "./entity-search";

const mockPeopleData = {
  people: [
    { id: 1, name: "Alice", authored_book_count: 3, narrated_file_count: 0 },
    { id: 2, name: "Bob", authored_book_count: 1, narrated_file_count: 5 },
  ],
  total: 2,
};

vi.mock("./people", () => ({
  usePeopleList: () => ({ data: mockPeopleData, isLoading: false }),
}));
vi.mock("./genres", () => ({
  useGenresList: () => ({
    data: {
      genres: [
        { name: "Fantasy", book_count: 5 },
        { name: "Sci-Fi", book_count: 12 },
      ],
      total: 2,
    },
    isLoading: false,
  }),
}));
vi.mock("./tags", () => ({
  useTagsList: () => ({
    data: { tags: [{ name: "favorite", book_count: 2 }], total: 1 },
    isLoading: false,
  }),
}));
vi.mock("./series", () => ({
  useSeriesList: () => ({
    data: { series: [{ name: "Dune", book_count: 6 }], total: 1 },
    isLoading: false,
  }),
}));
vi.mock("./publishers", () => ({
  usePublishersList: () => ({
    data: { publishers: [{ name: "Tor", file_count: 8 }], total: 1 },
    isLoading: false,
  }),
}));
vi.mock("./imprints", () => ({
  useImprintsList: () => ({
    data: { imprints: [{ name: "Orbit", file_count: 3 }], total: 1 },
    isLoading: false,
  }),
}));

describe("entity-search adapter hooks", () => {
  it("usePeopleSearch includes counts", () => {
    const result = usePeopleSearch(1, true, "");
    expect(result.isLoading).toBe(false);
    expect(result.data).toEqual([
      { id: 1, name: "Alice", authored_book_count: 3, narrated_file_count: 0 },
      { id: 2, name: "Bob", authored_book_count: 1, narrated_file_count: 5 },
    ]);
  });

  it("useGenreSearch returns objects with book_count", () => {
    const result = useGenreSearch(1, true, "");
    expect(result.data).toEqual([
      { name: "Fantasy", book_count: 5 },
      { name: "Sci-Fi", book_count: 12 },
    ]);
  });

  it("useTagSearch returns objects with book_count", () => {
    const result = useTagSearch(1, true, "");
    expect(result.data).toEqual([{ name: "favorite", book_count: 2 }]);
  });

  it("useSeriesSearch includes book_count", () => {
    const result = useSeriesSearch(1, true, "");
    expect(result.data).toEqual([{ name: "Dune", book_count: 6 }]);
  });

  it("usePublisherSearch includes file_count", () => {
    const result = usePublisherSearch(1, true, "");
    expect(result.data).toEqual([{ name: "Tor", file_count: 8 }]);
  });

  it("useImprintSearch includes file_count", () => {
    const result = useImprintSearch(1, true, "");
    expect(result.data).toEqual([{ name: "Orbit", file_count: 3 }]);
  });
});
