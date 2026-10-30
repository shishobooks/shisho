import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { useAuth } from "@/hooks/useAuth";
import { API, ShishoAPIError } from "@/libraries/api";
import { writeDemoSettings } from "@/libraries/demoSettings";
import {
  PageFlowPaginated,
  PageFlowScrolled,
  type BookReadingResponse,
  type PageFlow,
  type ReorderToReadPayload,
  type SetPageFlowPayload,
  type SetRatingPayload,
  type UpdateReadThroughPayload,
} from "@/types";
import { todayDay, type ReadingTransition } from "@/utils/readingStatus";

import { QueryKey as BooksQueryKey } from "./books";
import { useRequires } from "./permissions";

export enum QueryKey {
  BookReading = "BookReading",
}

// In Demo Mode, a Book's Page flow is a reader setting like the others: it
// stays in the browser, over whatever the server holds.
export const getDemoPageFlowKey = (bookId: number) =>
  `shisho-demo-page-flow-${bookId}`;

const loadBookReading = async (
  bookId: number,
  demoMode: boolean,
  signal?: AbortSignal,
) => {
  const reading = await API.request<BookReadingResponse>(
    "GET",
    `/reading/books/${bookId}`,
    null,
    null,
    signal,
  );
  if (!demoMode) return reading;
  const saved = readDemoPageFlow(bookId);
  return saved === undefined ? reading : { ...reading, page_flow: saved };
};

// The flow saved in this browser, or undefined when there is none. Only
// choosing a flow writes one, so browsing books leaves nothing behind, and a
// value that isn't a flow counts as none.
const readDemoPageFlow = (bookId: number): PageFlow | null | undefined => {
  try {
    const saved = localStorage.getItem(getDemoPageFlowKey(bookId));
    const parsed: unknown = saved ? JSON.parse(saved) : null;
    if (!parsed || typeof parsed !== "object") return undefined;
    const flow = (parsed as { page_flow?: unknown }).page_flow;
    if (
      flow === null ||
      flow === PageFlowPaginated ||
      flow === PageFlowScrolled
    ) {
      return flow;
    }
  } catch {
    // Blocked storage or corrupt JSON: the server's value stands.
  }
  return undefined;
};

export const useBookReading = (
  bookId?: number,
  options: Omit<
    UseQueryOptions<BookReadingResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  const { demoMode } = useAuth();
  return useQuery<BookReadingResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? Boolean(bookId)),
    queryKey: [QueryKey.BookReading, bookId],
    queryFn: ({ signal }) => loadBookReading(bookId!, demoMode, signal),
  });
};

// Every reading write returns the Book's whole reading record. The status
// also shows on book cards, so book listings (library and List alike, which
// share the ListBooks key) refetch. A Book joining or leaving To Read moves
// every Book after it, so other Books' records refetch for their places.
const onReadingChanged = (
  queryClient: QueryClient,
  data: BookReadingResponse,
) => {
  queryClient.setQueryData([QueryKey.BookReading, data.book_id], data);
  queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
  invalidateOtherBookReadings(queryClient, data.book_id);
};

const invalidateOtherBookReadings = (
  queryClient: QueryClient,
  exceptBookId?: number,
) => {
  queryClient.invalidateQueries({
    queryKey: [QueryKey.BookReading],
    predicate: (query) => query.queryKey[1] !== exceptBookId,
  });
};

// Transitions that date a Read-through send the browser's local day.
const DATED_TRANSITIONS: ReadingTransition[] = ["start", "finish", "abandon"];

interface ReadingTransitionVariables {
  bookId: number;
  transition: ReadingTransition;
}

export const useReadingTransition = () => {
  const queryClient = useQueryClient();
  return useMutation<
    BookReadingResponse,
    ShishoAPIError,
    ReadingTransitionVariables
  >({
    mutationFn: ({ bookId, transition }) => {
      const body = DATED_TRANSITIONS.includes(transition)
        ? { today: todayDay() }
        : null;
      return API.request(
        "POST",
        `/reading/books/${bookId}/${transition}`,
        body,
        null,
      );
    },
    onSuccess: (data) => onReadingChanged(queryClient, data),
  });
};

interface SetRatingVariables {
  bookId: number;
  payload: SetRatingPayload;
}

export const useSetRating = () => {
  const queryClient = useQueryClient();
  return useMutation<BookReadingResponse, ShishoAPIError, SetRatingVariables>({
    mutationFn: ({ bookId, payload }) => {
      return API.request(
        "PUT",
        `/reading/books/${bookId}/rating`,
        payload,
        null,
      );
    },
    onSuccess: (data) => onReadingChanged(queryClient, data),
  });
};

interface SetPageFlowVariables {
  bookId: number;
  payload: SetPageFlowPayload;
}

// The Page flow changes nothing a listing or another Book shows, so only
// this Book's record updates.
export const useSetPageFlow = () => {
  const queryClient = useQueryClient();
  const { demoMode } = useAuth();
  return useMutation<BookReadingResponse, ShishoAPIError, SetPageFlowVariables>(
    {
      mutationFn: async ({ bookId, payload }) => {
        if (!demoMode) {
          return API.request(
            "PUT",
            `/reading/books/${bookId}/page-flow`,
            payload,
            null,
          );
        }
        const current =
          queryClient.getQueryData<BookReadingResponse>([
            QueryKey.BookReading,
            bookId,
          ]) ?? (await loadBookReading(bookId, true));
        const page_flow = payload.page_flow ?? null;
        writeDemoSettings(getDemoPageFlowKey(bookId), { page_flow });
        return { ...current, page_flow };
      },
      onSuccess: (data) => {
        queryClient.setQueryData([QueryKey.BookReading, data.book_id], data);
      },
    },
  );
};

interface UpdateReadThroughVariables {
  id: number;
  payload: UpdateReadThroughPayload;
}

export const useUpdateReadThrough = () => {
  const queryClient = useQueryClient();
  return useMutation<
    BookReadingResponse,
    ShishoAPIError,
    UpdateReadThroughVariables
  >({
    mutationFn: ({ id, payload }) => {
      return API.request(
        "PATCH",
        `/reading/read-throughs/${id}`,
        payload,
        null,
      );
    },
    onSuccess: (data) => onReadingChanged(queryClient, data),
  });
};

export const useDeleteReadThrough = () => {
  const queryClient = useQueryClient();
  return useMutation<BookReadingResponse, ShishoAPIError, number>({
    mutationFn: (id) => {
      return API.request("DELETE", `/reading/read-throughs/${id}`, null, null);
    },
    onSuccess: (data) => onReadingChanged(queryClient, data),
  });
};

interface SetOnToReadVariables {
  bookId: number;
  onToRead: boolean;
}

// Adds a Book to the end of To Read, or takes it off. The To Read page is a
// book listing, so onReadingChanged refetches it.
export const useSetOnToRead = () => {
  const queryClient = useQueryClient();
  return useMutation<BookReadingResponse, ShishoAPIError, SetOnToReadVariables>(
    {
      mutationFn: ({ bookId, onToRead }) => {
        return API.request(
          onToRead ? "POST" : "DELETE",
          `/reading/books/${bookId}/to-read`,
          null,
          null,
        );
      },
      onSuccess: (data) => onReadingChanged(queryClient, data),
    },
  );
};

export const useReorderToRead = () => {
  const queryClient = useQueryClient();
  return useMutation<void, ShishoAPIError, ReorderToReadPayload>({
    mutationFn: (payload) => {
      return API.request("PUT", "/reading/to-read/order", payload, null);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      invalidateOtherBookReadings(queryClient);
    },
  });
};
