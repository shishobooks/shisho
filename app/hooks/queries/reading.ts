import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  BookReadingResponse,
  ReorderToReadPayload,
  SetRatingPayload,
  UpdateReadThroughPayload,
} from "@/types";
import { todayDay, type ReadingTransition } from "@/utils/readingStatus";

import { QueryKey as BooksQueryKey } from "./books";
import { useRequires } from "./permissions";

export enum QueryKey {
  BookReading = "BookReading",
}

export const useBookReading = (
  bookId?: number,
  options: Omit<
    UseQueryOptions<BookReadingResponse, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<BookReadingResponse, ShishoAPIError>({
    ...options,
    enabled: useRequires("books:read", options.enabled ?? Boolean(bookId)),
    queryKey: [QueryKey.BookReading, bookId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/reading/books/${bookId}`, null, null, signal);
    },
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
