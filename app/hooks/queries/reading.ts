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
  type FilePositionResponse,
  type ListReadingHistoryQuery,
  type ListReadingHistoryResponse,
  type ListReadingStatsFinishedQuery,
  type ListReadingStatsFinishedResponse,
  type PageFlow,
  type ReadingStatsCalendarQuery,
  type ReadingStatsCalendarResponse,
  type ReadingStatsDayQuery,
  type ReadingStatsDayResponse,
  type ReadingStatsHeatmapQuery,
  type ReadingStatsHeatmapResponse,
  type ReadingStatsQuery,
  type ReadingStatsResponse,
  type ReadThroughChangeResponse,
  type ReadThroughSessionsResponse,
  type ReorderToReadPayload,
  type SetPageFlowPayload,
  type SetRatingPayload,
  type UpdatePositionPayload,
  type UpdatePositionResponse,
  type UpdateReadThroughPayload,
  type UserSettingsResponse,
} from "@/types";
import { todayDay, type ReadingTransition } from "@/utils/readingStatus";

import { QueryKey as BooksQueryKey } from "./books";
import { useRequires } from "./permissions";
import { QueryKey as ServerStatsQueryKey } from "./serverStats";
import { QueryKey as SettingsQueryKey } from "./settings";

export enum QueryKey {
  BookReading = "BookReading",
  FilePosition = "FilePosition",
  ReadingHistory = "ReadingHistory",
  ReadingStats = "ReadingStats",
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
// Starting a Read-through or deleting the one underway resets the Book's
// Reading Positions, so positions refetch too.
const onReadingChanged = (
  queryClient: QueryClient,
  data: BookReadingResponse,
) => {
  queryClient.setQueryData([QueryKey.BookReading, data.book_id], data);
  queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
  queryClient.invalidateQueries({ queryKey: [QueryKey.FilePosition] });
  invalidateReadingRecords(queryClient);
  invalidateOtherBookReadings(queryClient, data.book_id);
};

// A change to a Read-through carries its Book's reading record, unless the
// Book was deleted or is in a library the user can't reach.
const onReadThroughChanged = (
  queryClient: QueryClient,
  data: ReadThroughChangeResponse,
) => {
  if (data.reading) {
    onReadingChanged(queryClient, data.reading);
    return;
  }
  invalidateReadingRecords(queryClient);
};

// Reading History and Reading Stats summarize every Read-through and Session
// the user has, so any change to one refetches both.
export const invalidateReadingRecords = (queryClient: QueryClient) => {
  queryClient.invalidateQueries({ queryKey: [QueryKey.ReadingHistory] });
  queryClient.invalidateQueries({ queryKey: [QueryKey.ReadingStats] });
};

// The signed-in user's Reading Stats for one year, or all time without one.
export const useReadingStats = (
  query: ReadingStatsQuery,
  options: Omit<
    UseQueryOptions<ReadingStatsResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ReadingStatsResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? true),
    queryKey: [QueryKey.ReadingStats, query],
    queryFn: ({ signal }) =>
      API.request("GET", "/reading/stats", null, query, signal),
  });
};

// A page of the books the signed-in user finished in a year, or all time,
// for Reading Stats. It sits under the Reading Stats key, so whatever
// refetches Reading Stats refetches it too.
export const useReadingStatsFinished = (
  query: ListReadingStatsFinishedQuery,
  options: Omit<
    UseQueryOptions<ListReadingStatsFinishedResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListReadingStatsFinishedResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? true),
    queryKey: [QueryKey.ReadingStats, "finished", query],
    queryFn: ({ signal }) =>
      API.request("GET", "/reading/stats/finished", null, query, signal),
  });
};

// The signed-in user's reading a day at a time for a year, or the last 12
// months, for the heatmap on Reading Stats' Calendar tab, under the Reading
// Stats key.
export const useReadingStatsHeatmap = (
  query: ReadingStatsHeatmapQuery,
  options: Omit<
    UseQueryOptions<ReadingStatsHeatmapResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ReadingStatsHeatmapResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? true),
    queryKey: [QueryKey.ReadingStats, "heatmap", query],
    queryFn: ({ signal }) =>
      API.request("GET", "/reading/stats/heatmap", null, query, signal),
  });
};

// One month of the signed-in user's Reading Stats calendar. Like the
// finished list, it sits under the Reading Stats key.
export const useReadingStatsCalendar = (
  query: ReadingStatsCalendarQuery,
  options: Omit<
    UseQueryOptions<ReadingStatsCalendarResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ReadingStatsCalendarResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? true),
    queryKey: [QueryKey.ReadingStats, "calendar", query],
    queryFn: ({ signal }) =>
      API.request("GET", "/reading/stats/calendar", null, query, signal),
  });
};

// One day's Reading Sessions for the Reading Stats calendar, under the
// Reading Stats key.
export const useReadingStatsDay = (
  query: ReadingStatsDayQuery,
  options: Omit<
    UseQueryOptions<ReadingStatsDayResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ReadingStatsDayResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? true),
    queryKey: [QueryKey.ReadingStats, "day", query],
    queryFn: ({ signal }) =>
      API.request("GET", "/reading/stats/day", null, query, signal),
  });
};

// The Reading Sessions of one of the signed-in user's Read-throughs. It
// sits under the Reading History key, so deleting a Session refetches it.
export const useReadThroughSessions = (
  readThroughId: number,
  options: Omit<
    UseQueryOptions<ReadThroughSessionsResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ReadThroughSessionsResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? true),
    queryKey: [QueryKey.ReadingHistory, "sessions", readThroughId],
    queryFn: ({ signal }) =>
      API.request(
        "GET",
        `/reading/read-throughs/${readThroughId}/sessions`,
        null,
        null,
        signal,
      ),
  });
};

// The signed-in user's Read-throughs for all time, a page at a time.
export const useReadingHistory = (
  query: ListReadingHistoryQuery,
  options: Omit<
    UseQueryOptions<ListReadingHistoryResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListReadingHistoryResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? true),
    queryKey: [QueryKey.ReadingHistory, query],
    queryFn: ({ signal }) =>
      API.request("GET", "/reading/history", null, query, signal),
  });
};

interface EndReadThroughVariables {
  id: number;
  outcome: "finish" | "abandon";
}

// Ends a Read-through underway from Reading History, where its Book may be
// gone, dated like ending it from the Book.
export const useEndReadThrough = () => {
  const queryClient = useQueryClient();
  return useMutation<
    ReadThroughChangeResponse,
    ShishoAPIError,
    EndReadThroughVariables
  >({
    mutationFn: ({ id, outcome }) => {
      const timeZone = queryClient.getQueryData<UserSettingsResponse>([
        SettingsQueryKey.UserSettings,
      ])?.time_zone;
      return API.request(
        "POST",
        `/reading/read-throughs/${id}/${outcome}`,
        { today: todayDay(timeZone) },
        null,
      );
    },
    onSuccess: (data) => onReadThroughChanged(queryClient, data),
  });
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

// Transitions that date a Read-through send today in the user's time zone,
// from the settings the app loads at start.
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
      const timeZone = queryClient.getQueryData<UserSettingsResponse>([
        SettingsQueryKey.UserSettings,
      ])?.time_zone;
      const body = DATED_TRANSITIONS.includes(transition)
        ? { today: todayDay(timeZone) }
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
    ReadThroughChangeResponse,
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
    onSuccess: (data) => onReadThroughChanged(queryClient, data),
  });
};

export const useDeleteReadThrough = () => {
  const queryClient = useQueryClient();
  return useMutation<ReadThroughChangeResponse, ShishoAPIError, number>({
    mutationFn: (id) => {
      return API.request("DELETE", `/reading/read-throughs/${id}`, null, null);
    },
    onSuccess: (data) => {
      onReadThroughChanged(queryClient, data);
      // Its Sessions go with it, and Server Stats totals everyone's time.
      queryClient.invalidateQueries({
        queryKey: [ServerStatsQueryKey.ServerStats],
      });
    },
  });
};

// Deletes one of the signed-in user's Reading Sessions for good. Its time
// leaves Reading Stats and Server Stats, and its Read-through's session
// count drops on Reading History and the Book's page.
export const useDeleteReadingSession = () => {
  const queryClient = useQueryClient();
  return useMutation<void, ShishoAPIError, number>({
    mutationFn: (id) => {
      return API.request("DELETE", `/reading/sessions/${id}`, null, null);
    },
    onSuccess: () => {
      invalidateReadingRecords(queryClient);
      queryClient.invalidateQueries({ queryKey: [QueryKey.BookReading] });
      // Server Stats totals everyone's reading time.
      queryClient.invalidateQueries({
        queryKey: [ServerStatsQueryKey.ServerStats],
      });
    },
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

// The signed-in user's Reading Position in a File, which a reader opens at.
export const useFilePosition = (
  fileId?: number,
  options: Omit<
    UseQueryOptions<FilePositionResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<FilePositionResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? Boolean(fileId)),
    queryKey: [QueryKey.FilePosition, fileId],
    queryFn: ({ signal }) =>
      API.request(
        "GET",
        `/reading/files/${fileId}/position`,
        null,
        null,
        signal,
      ),
  });
};

// Saves where the reader is in a File. Readers send it as they read, so it
// isn't a mutation hook; keepalive is for a save as the page closes.
export const saveFilePosition = (
  fileId: number,
  payload: UpdatePositionPayload,
  keepalive = false,
) =>
  API.request<UpdatePositionResponse>(
    "POST",
    `/reading/files/${fileId}/position`,
    payload,
    null,
    undefined,
    { keepalive },
  );

// A position update can start, resume, finish, or reopen a Read-through,
// which shows on the book page and on cards, and can take the Book off To
// Read, which moves every Book after it. Starting one resets the Book's other
// Files' positions; the reader keeps its own File's cache current.
export const invalidateAfterPositionUpdate = (
  queryClient: QueryClient,
  bookId: number,
  fileId: number,
) => {
  queryClient.invalidateQueries({ queryKey: [QueryKey.BookReading, bookId] });
  queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
  invalidateReadingRecords(queryClient);
  queryClient.invalidateQueries({
    queryKey: [QueryKey.FilePosition],
    predicate: (query) => query.queryKey[1] !== fileId,
  });
  invalidateOtherBookReadings(queryClient, bookId);
};

// A save changed the progress the book page and book cards show. An open reader
// has no use for that, so they just refetch the next time something shows them,
// or now if the reader has closed and something may already be showing them.
export const markProgressStale = (
  queryClient: QueryClient,
  bookId: number,
  readerClosed: boolean,
) => {
  const refetchType = readerClosed ? "active" : "none";
  queryClient.invalidateQueries({
    queryKey: [QueryKey.BookReading, bookId],
    refetchType,
  });
  queryClient.invalidateQueries({
    queryKey: [BooksQueryKey.ListBooks],
    refetchType,
  });
};

// Undoes the finish a reader's position update made, for Undo on the finish
// toast.
export const useUndoAutoFinish = () => {
  const queryClient = useQueryClient();
  return useMutation<BookReadingResponse, ShishoAPIError, number>({
    mutationFn: (readThroughId) => {
      return API.request(
        "POST",
        `/reading/read-throughs/${readThroughId}/undo-finish`,
        null,
        null,
      );
    },
    onSuccess: (data) => onReadingChanged(queryClient, data),
  });
};
