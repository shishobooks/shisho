import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { API } from "@/libraries/api";
import type { BookReadingResponse } from "@/types";

import { useDeleteBook, useDeleteBooks } from "./books";
import {
  QueryKey,
  useBulkReadingChange,
  useReorderToRead,
  useSetOnToRead,
} from "./reading";

const reading = (
  bookId: number,
  overrides: Partial<BookReadingResponse> = {},
): BookReadingResponse => ({
  book_id: bookId,
  status: "to_read",
  rating: null,
  on_to_read: true,
  read_throughs: [],
  session_counts: {},
  positions: [],
  ...overrides,
});

// Two queued Books' reading records in the cache, as two visited book pages
// leave them.
const setup = () => {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  client.setQueryData(
    [QueryKey.BookReading, 1],
    reading(1, { to_read_place: 1 }),
  );
  client.setQueryData(
    [QueryKey.BookReading, 2],
    reading(2, { to_read_place: 2 }),
  );
  const wrapper = ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client }, children);
  const invalidated = (bookId: number) =>
    client.getQueryState([QueryKey.BookReading, bookId])?.isInvalidated;
  return { client, wrapper, invalidated };
};

afterEach(() => vi.restoreAllMocks());

// A Book joining or leaving To Read moves every Book after it, so the other
// cached records refetch for their places; the changed Book's comes from the
// response.
describe("To Read places stay current", () => {
  it("refetches other Books' records when one leaves To Read", async () => {
    const { client, wrapper, invalidated } = setup();
    vi.spyOn(API, "request").mockResolvedValue(
      reading(1, { status: null, on_to_read: false }),
    );
    const { result } = renderHook(() => useSetOnToRead(), { wrapper });

    await act(() => result.current.mutateAsync({ bookId: 1, onToRead: false }));

    expect(client.getQueryData([QueryKey.BookReading, 1])).toMatchObject({
      on_to_read: false,
    });
    expect(invalidated(1)).toBe(false);
    expect(invalidated(2)).toBe(true);
  });

  it("refetches every record after a reorder", async () => {
    const { wrapper, invalidated } = setup();
    vi.spyOn(API, "request").mockResolvedValue(undefined);
    const { result } = renderHook(() => useReorderToRead(), { wrapper });

    await act(() => result.current.mutateAsync({ book_ids: [2, 1] }));

    expect(invalidated(1)).toBe(true);
    expect(invalidated(2)).toBe(true);
  });

  it("refetches every record after deleting a Book", async () => {
    const { wrapper, invalidated } = setup();
    vi.spyOn(API, "request").mockResolvedValue({});
    const { result } = renderHook(() => useDeleteBook(), { wrapper });

    await act(() => result.current.mutateAsync(1));

    expect(invalidated(2)).toBe(true);
  });

  it("refetches every record after deleting Books in bulk", async () => {
    const { wrapper, invalidated } = setup();
    vi.spyOn(API, "request").mockResolvedValue({});
    const { result } = renderHook(() => useDeleteBooks(), { wrapper });

    await act(() => result.current.mutateAsync({ book_ids: [1] }));

    expect(invalidated(2)).toBe(true);
  });
});

// A bulk change can touch any Book, so every reading record and the reading
// summaries refetch.
describe("bulk reading changes", () => {
  it("sends today with marking read and refetches what it changed", async () => {
    const { client, wrapper, invalidated } = setup();
    client.setQueryData([QueryKey.ReadingHistory, {}], { items: [] });
    client.setQueryData([QueryKey.ReadingStats, {}], {});
    const request = vi
      .spyOn(API, "request")
      .mockResolvedValue({ changed: 2, skipped: 0 });
    const { result } = renderHook(() => useBulkReadingChange(), { wrapper });

    await act(() =>
      result.current.mutateAsync({ action: "finish", bookIds: [1, 2] }),
    );

    expect(request).toHaveBeenCalledWith(
      "POST",
      "/reading/books/bulk/finish",
      { book_ids: [1, 2], today: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/) },
      null,
    );
    expect(invalidated(1)).toBe(true);
    expect(invalidated(2)).toBe(true);
    expect(
      client.getQueryState([QueryKey.ReadingHistory, {}])?.isInvalidated,
    ).toBe(true);
    expect(
      client.getQueryState([QueryKey.ReadingStats, {}])?.isInvalidated,
    ).toBe(true);
  });

  it("marks not started with the books alone", async () => {
    const { wrapper } = setup();
    const request = vi
      .spyOn(API, "request")
      .mockResolvedValue({ changed: 1, skipped: 0 });
    const { result } = renderHook(() => useBulkReadingChange(), { wrapper });

    await act(() =>
      result.current.mutateAsync({ action: "not-started", bookIds: [1] }),
    );

    expect(request).toHaveBeenCalledWith(
      "POST",
      "/reading/books/bulk/not-started",
      { book_ids: [1] },
      null,
    );
  });
});
