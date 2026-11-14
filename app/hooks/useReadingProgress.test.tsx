import {
  QueryClient,
  QueryClientProvider,
  QueryObserver,
} from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { QueryKey as BooksQueryKey } from "@/hooks/queries/books";
import { QueryKey } from "@/hooks/queries/reading";
import { API } from "@/libraries/api";
import { setAuth } from "@/testing/auth";

import {
  POSITION_SAVE_DELAY_MS,
  useReadingProgress,
} from "./useReadingProgress";

vi.mock("@/hooks/useAuth", () => import("@/testing/auth"));

const BOOK_ID = 3;

const setup = (options?: Parameters<typeof useReadingProgress>[1]) => {
  const client = new QueryClient();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const hook = renderHook(() => useReadingProgress(7, options), { wrapper });
  return { client, hook };
};

const flushSave = () =>
  act(async () => {
    await vi.advanceTimersByTimeAsync(POSITION_SAVE_DELAY_MS);
  });

describe("useReadingProgress", () => {
  let posts: Array<Record<string, unknown>>;

  beforeEach(() => {
    setAuth({ permissions: ["books:read"] });
    posts = [];
    vi.spyOn(API, "request").mockImplementation(((
      _method: string,
      _endpoint: string,
      payload: Record<string, unknown>,
    ) => {
      posts.push(payload);
      return Promise.resolve({
        book_id: BOOK_ID,
        accepted: true,
        started: false,
        resumed: false,
        reverted: false,
        finished_read_through_id: null,
      });
    }) as never);
  });

  // Queries never go stale on their own, so the book page's percentages rely
  // on this. The open reader mustn't refetch the record on every page, but the
  // page it returns to must see the save that goes out as it closes.
  it("refreshes the book's reading record after a save only once the reader closes", async () => {
    const { client, hook } = setup();
    const reading = vi.fn().mockResolvedValue({ book_id: BOOK_ID });
    const observer = new QueryObserver(client, {
      queryKey: [QueryKey.BookReading, BOOK_ID],
      queryFn: reading,
    });
    const unsubscribe = observer.subscribe(() => {});
    client.setQueryData([BooksQueryKey.ListBooks, {}], { books: [] });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(reading).toHaveBeenCalledTimes(1);

    act(() =>
      hook.result.current.moved({ page: 1, percentage: 0.1, atEnd: false }),
    );
    await flushSave();
    expect(posts).toHaveLength(1);
    expect(reading).toHaveBeenCalledTimes(1);
    // Book cards show the same progress.
    expect(
      client.getQueryState([BooksQueryKey.ListBooks, {}])?.isInvalidated,
    ).toBe(true);

    act(() =>
      hook.result.current.moved({ page: 2, percentage: 0.2, atEnd: false }),
    );
    hook.unmount();
    await flushSave();
    expect(posts).toHaveLength(2);
    expect(reading).toHaveBeenCalledTimes(2);
    unsubscribe();
  });

  it("keeps a playing session when the hook re-enables", () => {
    const client = new QueryClient();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result, rerender } = renderHook(
      ({ enabled }) => useReadingProgress(7, { enabled, timing: "playback" }),
      { wrapper, initialProps: { enabled: true } },
    );
    act(() =>
      result.current.opened({ seconds: 0, percentage: 0, atEnd: false }),
    );
    act(() => result.current.setPlaying(true));
    rerender({ enabled: false });
    rerender({ enabled: true });

    act(() => vi.advanceTimersByTime(60_000));
    act(() => result.current.setPlaying(false));

    const sessions = posts.filter((post) => post.session);
    expect(sessions).toHaveLength(1);
  });

  it("counts time from when a place opened from a link shows", () => {
    const { hook } = setup();
    act(() => vi.advanceTimersByTime(10_000));
    act(() =>
      hook.result.current.openedFromLink({
        page: 3,
        percentage: 0.3,
        atEnd: false,
      }),
    );
    act(() => vi.advanceTimersByTime(30_000));
    hook.unmount();

    const sessions = posts.filter((post) => post.session);
    expect(sessions).toHaveLength(1);
    expect(sessions[0].session).toMatchObject({ duration: 30 });
  });

  // Scrolling moves the CFI every few seconds, which would make every
  // location a page too short to count.
  it("counts reading time per screen, not per location", () => {
    const { hook } = setup();
    act(() =>
      hook.result.current.opened({
        cfi: "epubcfi(/6/4!/4/2)",
        percentage: 0.1,
        atEnd: false,
        screen: "1:0",
      }),
    );
    for (let i = 1; i <= 20; i++) {
      act(() => vi.advanceTimersByTime(3_000));
      act(() =>
        hook.result.current.moved({
          cfi: `epubcfi(/6/4!/4/2/${i})`,
          percentage: 0.1,
          atEnd: false,
          screen: i < 10 ? "1:0" : "1:1",
        }),
      );
    }
    hook.unmount();

    const sessions = posts.filter((post) => post.session);
    expect(sessions).toHaveLength(1);
    expect(sessions[0].session).toMatchObject({ duration: 30 + 30 });
  });
});
