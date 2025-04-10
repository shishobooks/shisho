import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Book, ListBooksQuery, UpdateBookPayload } from "@/types";

export enum QueryKey {
  RetrieveBook = "RetrieveBook",
  ListBooks = "ListBooks",
}

export const useBook = (
  id?: string,
  options: Omit<
    UseQueryOptions<Book, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Book, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(id),
    ...options,
    queryKey: [QueryKey.RetrieveBook, id],
    queryFn: ({ signal }) => {
      return API.request("GET", `/books/${id}`, null, null, signal);
    },
  });
};

export interface ListBooksData {
  books: Book[];
  total: number;
}

export const useBooks = (
  query: ListBooksQuery = {},
  options: Omit<
    UseQueryOptions<ListBooksData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListBooksData, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListBooks, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/books", null, query, signal);
    },
  });
};

interface UpdateBookMutationVariables {
  id: string;
  payload: UpdateBookPayload;
}

export const useUpdateBook = () => {
  const queryClient = useQueryClient();

  return useMutation<Book, ShishoAPIError, UpdateBookMutationVariables>({
    mutationFn: ({ id, payload }) => {
      return API.request("POST", `/books/${id}`, payload, null);
    },
    onSuccess: (data: Book) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.setQueryData([QueryKey.RetrieveBook, data.id], data);
    },
  });
};
