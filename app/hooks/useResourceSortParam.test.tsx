import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { describe, expect, it } from "vitest";

import { PERSON_SORT } from "@/libraries/resourceSort";

import { useResourceSortParam } from "./useResourceSortParam";

const renderAt = (entry: string) =>
  renderHook(
    () => ({
      sort: useResourceSortParam(PERSON_SORT),
      location: useLocation(),
    }),
    {
      wrapper: ({ children }: { children: ReactNode }) => (
        <MemoryRouter initialEntries={[entry]}>{children}</MemoryRouter>
      ),
    },
  );

describe("useResourceSortParam", () => {
  it("uses the default sort when the URL has none", () => {
    const { result } = renderAt("/");
    expect(result.current.sort.levels).toEqual([
      { field: "name", direction: "asc" },
    ]);
    expect(result.current.sort.isDirty).toBe(false);
    expect(result.current.sort.param).toBe("name:asc");
  });

  it("reads a sort from the URL", () => {
    const { result } = renderAt("/?sort=authored_book_count:desc");
    expect(result.current.sort.levels).toEqual([
      { field: "authored_book_count", direction: "desc" },
    ]);
    expect(result.current.sort.isDirty).toBe(true);
    expect(result.current.sort.param).toBe("authored_book_count:desc");
  });

  it("ignores a sort with a field this list does not offer", () => {
    // book_count belongs to tags, genres, and series, not people.
    const { result } = renderAt("/?sort=book_count:desc");
    expect(result.current.sort.levels).toEqual([
      { field: "name", direction: "asc" },
    ]);
    expect(result.current.sort.isDirty).toBe(false);
  });

  it("writes a new sort to the URL and returns to the first page", () => {
    const { result } = renderAt("/?page=3&search=jo");
    act(() =>
      result.current.sort.apply([
        { field: "narrated_file_count", direction: "desc" },
      ]),
    );
    const params = new URLSearchParams(result.current.location.search);
    expect(params.get("sort")).toBe("narrated_file_count:desc");
    expect(params.get("page")).toBe("1");
    expect(params.get("search")).toBe("jo");
  });

  it("drops the URL param when the sort returns to the default", () => {
    const { result } = renderAt("/?sort=date_added:desc");
    act(() => result.current.sort.apply([{ field: "name", direction: "asc" }]));
    expect(result.current.location.search).not.toContain("sort=");

    act(() =>
      result.current.sort.apply([{ field: "date_added", direction: "asc" }]),
    );
    act(() => result.current.sort.reset());
    expect(result.current.location.search).not.toContain("sort=");
  });
});
