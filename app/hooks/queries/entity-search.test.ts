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
    data: { genres: [{ name: "Fantasy" }, { name: "Sci-Fi" }], total: 2 },
    isLoading: false,
  }),
}));
vi.mock("./tags", () => ({
  useTagsList: () => ({
    data: { tags: [{ name: "favorite" }], total: 1 },
    isLoading: false,
  }),
}));
vi.mock("./series", () => ({
  useSeriesList: () => ({
    data: { series: [{ name: "Dune" }], total: 1 },
    isLoading: false,
  }),
}));
vi.mock("./publishers", () => ({
  usePublishersList: () => ({
    data: { publishers: [{ name: "Tor" }], total: 1 },
    isLoading: false,
  }),
}));
vi.mock("./imprints", () => ({
  useImprintsList: () => ({
    data: { imprints: [{ name: "Orbit" }], total: 1 },
    isLoading: false,
  }),
}));

describe("entity-search adapter hooks", () => {
  it("usePeopleSearch adapts PersonWithCounts to { name, id }", () => {
    const result = usePeopleSearch(1, true, "");
    expect(result.isLoading).toBe(false);
    expect(result.data).toEqual([
      { id: 1, name: "Alice" },
      { id: 2, name: "Bob" },
    ]);
  });

  it("useGenreSearch returns string[]", () => {
    const result = useGenreSearch(1, true, "");
    expect(result.data).toEqual(["Fantasy", "Sci-Fi"]);
  });

  it("useTagSearch returns string[]", () => {
    const result = useTagSearch(1, true, "");
    expect(result.data).toEqual(["favorite"]);
  });

  it("useSeriesSearch adapts to { name }", () => {
    const result = useSeriesSearch(1, true, "");
    expect(result.data).toEqual([{ name: "Dune" }]);
  });

  it("usePublisherSearch adapts to { name }", () => {
    const result = usePublisherSearch(1, true, "");
    expect(result.data).toEqual([{ name: "Tor" }]);
  });

  it("useImprintSearch adapts to { name }", () => {
    const result = useImprintSearch(1, true, "");
    expect(result.data).toEqual([{ name: "Orbit" }]);
  });
});
