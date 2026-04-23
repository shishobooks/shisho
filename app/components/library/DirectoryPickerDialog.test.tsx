import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { BrowseQuery, BrowseResponse, Entry } from "@/types";

import DirectoryPickerDialog from "./DirectoryPickerDialog";

// --- Mock state plumbing ---
//
// We model React Query's behavior by tracking a "current key" (path+offset).
// When the component requests a different key than `currentKey`, the mock
// returns the previous key's data as placeholder with `isPlaceholderData=true`
// and `isFetching=true` — matching how `placeholderData: keepPreviousData`
// behaves in production. The test advances `currentKey` to simulate the
// fetch completing.

type BrowseState = {
  data: BrowseResponse | undefined;
  dataUpdatedAt: number;
  isPlaceholderData: boolean;
  isFetching: boolean;
  isLoading: boolean;
  isError: boolean;
  error: { message: string } | null;
};

let currentKey = "";
let lastSettledData: BrowseResponse | undefined;
let lastDataUpdatedAt = 0;
let dataUpdatedAtTick = 0;
let keyResponses: Record<string, BrowseResponse> = {};

// Key on every BrowseQuery field the component varies (path, offset, search,
// show_hidden) so the mock models placeholder transitions for all of them.
const keyOf = (q: BrowseQuery) =>
  `${q.path}::${q.offset ?? 0}::${q.search ?? ""}::${q.show_hidden ? 1 : 0}`;

vi.mock("@/hooks/queries/filesystem", () => ({
  useFilesystemBrowse: (q: BrowseQuery): BrowseState => {
    const requestedKey = keyOf(q);
    if (requestedKey === currentKey) {
      return {
        data: lastSettledData,
        dataUpdatedAt: lastDataUpdatedAt,
        isPlaceholderData: false,
        isFetching: false,
        isLoading: lastSettledData === undefined,
        isError: false,
        error: null,
      };
    }
    return {
      data: lastSettledData,
      dataUpdatedAt: 0,
      isPlaceholderData: lastSettledData !== undefined,
      isFetching: true,
      isLoading: lastSettledData === undefined,
      isError: false,
      error: null,
    };
  },
  QueryKey: { FilesystemBrowse: "FilesystemBrowse" },
}));

const settle = (key: string) => {
  currentKey = key;
  lastSettledData = keyResponses[key];
  // Use a monotonic counter so re-settles always change `dataUpdatedAt` and
  // re-trigger the accumulator effect, regardless of fake-timer state.
  lastDataUpdatedAt = ++dataUpdatedAtTick;
};

const makeEntries = (start: number, count: number): Entry[] =>
  Array.from({ length: count }, (_, i) => ({
    name: `entry-${start + i}`,
    path: `/root/entry-${start + i}`,
    is_dir: true,
  }));

const wrap = (ui: React.ReactNode) => (
  <QueryClientProvider
    client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
  >
    {ui}
  </QueryClientProvider>
);

const renderDialog = () =>
  render(
    wrap(
      <DirectoryPickerDialog
        initialPath="/root"
        onOpenChange={() => {}}
        onSelect={() => {}}
        open
      />,
    ),
  );

describe("DirectoryPickerDialog", () => {
  beforeEach(() => {
    currentKey = "";
    lastSettledData = undefined;
    lastDataUpdatedAt = 0;
    dataUpdatedAtTick = 0;
    keyResponses = {};
  });

  it("does not duplicate entries when Load More transitions through placeholder data", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    keyResponses["/root::0::::0"] = {
      current_path: "/root",
      entries: makeEntries(1, 50),
      total: 100,
      has_more: true,
    };
    keyResponses["/root::50::::0"] = {
      current_path: "/root",
      entries: makeEntries(51, 50),
      total: 100,
      has_more: false,
    };
    settle("/root::0::::0");

    const { rerender } = renderDialog();

    expect(screen.getByText("entry-1")).toBeInTheDocument();
    expect(screen.getByText("entry-50")).toBeInTheDocument();

    // Click Load More — component will request offset:50, which the mock
    // treats as a key mismatch (returns placeholder = entries 1-50).
    await user.click(screen.getByRole("button", { name: /load more/i }));

    // Simulate the offset:50 fetch settling.
    settle("/root::50::::0");
    rerender(
      wrap(
        <DirectoryPickerDialog
          initialPath="/root"
          onOpenChange={() => {}}
          onSelect={() => {}}
          open
        />,
      ),
    );

    expect(screen.getAllByText("entry-1")).toHaveLength(1);
    expect(screen.getAllByText("entry-50")).toHaveLength(1);
    expect(screen.getByText("entry-51")).toBeInTheDocument();
    expect(screen.getByText("entry-100")).toBeInTheDocument();
  });

  it("does not retain previous directory's entries after navigating into a subdirectory", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    keyResponses["/root::0::::0"] = {
      current_path: "/root",
      entries: makeEntries(1, 50),
      total: 50,
      has_more: false,
    };
    keyResponses["/root/entry-1::0::::0"] = {
      current_path: "/root/entry-1",
      entries: [
        { name: "child-a", path: "/root/entry-1/child-a", is_dir: true },
      ],
      total: 1,
      has_more: false,
    };
    settle("/root::0::::0");

    const { rerender } = renderDialog();

    expect(screen.getByText("entry-1")).toBeInTheDocument();

    // Click into the subdirectory. The component navigates to
    // /root/entry-1, the mock returns the placeholder (old entries) with
    // isFetching=true while the real fetch is "in flight".
    await user.click(screen.getByText("entry-1"));

    // Mid-flight: previous directory's entries must NOT be visible. The
    // accumulator must skip placeholder data, and the spinner condition
    // must trigger when entries are empty + a fetch is in progress.
    expect(screen.queryByText("entry-1")).not.toBeInTheDocument();
    expect(screen.queryByText("entry-50")).not.toBeInTheDocument();

    // Settle the new directory's fetch.
    settle("/root/entry-1::0::::0");
    rerender(
      wrap(
        <DirectoryPickerDialog
          initialPath="/root"
          onOpenChange={() => {}}
          onSelect={() => {}}
          open
        />,
      ),
    );

    expect(screen.getByText("child-a")).toBeInTheDocument();
    expect(screen.queryByText("entry-50")).not.toBeInTheDocument();
  });

  it("does not retain previous results during a debounced search transition", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });

    keyResponses["/root::0::::0"] = {
      current_path: "/root",
      entries: makeEntries(1, 50),
      total: 50,
      has_more: false,
    };
    keyResponses["/root::0::book::0"] = {
      current_path: "/root",
      entries: [{ name: "book-1", path: "/root/book-1", is_dir: true }],
      total: 1,
      has_more: false,
    };
    settle("/root::0::::0");

    const { rerender } = renderDialog();

    expect(screen.getByText("entry-1")).toBeInTheDocument();

    // Type a search term and let the 300ms debounce fire.
    await user.type(screen.getByPlaceholderText(/search/i), "book");
    await vi.advanceTimersByTimeAsync(400);

    // Mid-flight: previous (unfiltered) entries must NOT be visible.
    expect(screen.queryByText("entry-1")).not.toBeInTheDocument();
    expect(screen.queryByText("entry-50")).not.toBeInTheDocument();

    // Settle the search query.
    settle("/root::0::book::0");
    rerender(
      wrap(
        <DirectoryPickerDialog
          initialPath="/root"
          onOpenChange={() => {}}
          onSelect={() => {}}
          open
        />,
      ),
    );

    expect(screen.getByText("book-1")).toBeInTheDocument();
    expect(screen.queryByText("entry-1")).not.toBeInTheDocument();
  });
});
