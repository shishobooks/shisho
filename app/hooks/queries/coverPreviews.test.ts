import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import { useSetFileCoverPage } from "./books";
import { QueryKey as GenresQueryKey } from "./genres";
import { useUpdateLibrary } from "./libraries";
import { QueryKey as PeopleQueryKey } from "./people";
import { QueryKey as PublishersQueryKey } from "./publishers";
import { QueryKey as TagsQueryKey } from "./tags";

vi.mock("@/libraries/api", async () => {
  const actual = await vi.importActual<object>("@/libraries/api");
  return {
    ...actual,
    API: { request: vi.fn().mockResolvedValue({ id: 1 }) },
  };
});

// The lists whose entries show book covers, so their data drives cover keys.
const previewLists = [
  PeopleQueryKey.ListPeople,
  GenresQueryKey.ListGenres,
  TagsQueryKey.ListTags,
  PublishersQueryKey.ListPublishers,
];

const setup = () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  for (const key of previewLists) {
    client.setQueryData([key, {}], { items: [], total: 0 });
  }
  const wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client }, children);
  const invalidated = () =>
    previewLists.filter(
      (key) => client.getQueryState([key, {}])?.isInvalidated,
    );
  return { wrapper, invalidated };
};

describe("cover preview lists", () => {
  it("refresh after a file's cover page changes", async () => {
    const { wrapper, invalidated } = setup();
    const { result } = renderHook(() => useSetFileCoverPage(), { wrapper });

    await act(() => result.current.mutateAsync({ id: 1, page: 2 }));

    expect(invalidated()).toEqual(previewLists);
  });

  it("refresh after a library's settings change, which can change its cover aspect ratio", async () => {
    const { wrapper, invalidated } = setup();
    const { result } = renderHook(() => useUpdateLibrary(), { wrapper });

    await act(() =>
      result.current.mutateAsync({
        id: "1",
        payload: { cover_aspect_ratio: "audiobook" },
      }),
    );

    expect(invalidated()).toEqual(previewLists);
  });
});
