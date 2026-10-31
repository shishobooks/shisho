import {
  QueryClient,
  QueryClientProvider,
  // Test-only, like the tests the rule exempts: it stands in for a server
  // search, so there is no route permission to gate on.
  // eslint-disable-next-line no-restricted-imports
  useQuery,
} from "@tanstack/react-query";
import { act } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";

import type { OptionSearch } from "@/hooks/queries/entity-search";
import { queryClient } from "@/libraries/query-client";

/** How long a search takes to answer: longer than the comboboxes' debounce. */
export const SEARCH_LATENCY_MS = 500;

/**
 * Advances past a combobox's debounce, then past the search it starts. Two
 * steps, because the search starts in the render the debounce triggers,
 * after a single advance has already stopped.
 */
export async function advancePastSearch() {
  await act(() => vi.advanceTimersByTimeAsync(300));
  await act(() => vi.advanceTimersByTimeAsync(SEARCH_LATENCY_MS + 50));
}

/**
 * Advances past every retry of a failing search, one step per attempt, since
 * each retry is scheduled only after the previous attempt fails.
 */
export async function advancePastFailedSearch() {
  await act(() => vi.advanceTimersByTimeAsync(300));
  for (let attempt = 0; attempt < 4; attempt++) {
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_LATENCY_MS + 50));
    await act(() => vi.advanceTimersByTimeAsync(10_000));
  }
}

/**
 * A search hook backed by real TanStack Query with the app's defaults, so a
 * new query keeps showing the previous results (`keepPreviousData`) until it
 * answers, `SEARCH_LATENCY_MS` later, as a server search does. Render under
 * `wrapper`, and advance the fake timers past the latency for the first
 * results to arrive.
 */
export function createSearchQuery<T>(
  items: T[],
  getLabel: (item: T) => string,
  /** Searches that fail, after the app's retries, instead of answering. */
  fails: (query: string) => boolean = () => false,
) {
  const client = new QueryClient({
    defaultOptions: queryClient.getDefaultOptions(),
  });

  function useSearch(query: string): OptionSearch<T> {
    const q = useQuery({
      queryKey: ["search", query],
      queryFn: () =>
        new Promise<T[]>((resolve, reject) =>
          setTimeout(
            () =>
              fails(query)
                ? reject(new Error("search failed"))
                : resolve(
                    items.filter((i) =>
                      getLabel(i).toLowerCase().includes(query.toLowerCase()),
                    ),
                  ),
            SEARCH_LATENCY_MS,
          ),
        ),
    });
    return {
      data: q.data,
      isLoading: q.isLoading,
      isPlaceholderData: q.isPlaceholderData,
      isError: q.isError,
    };
  }

  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );

  return { useSearch, wrapper };
}
