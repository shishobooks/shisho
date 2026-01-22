import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  Book,
  File,
  ListBooksQuery,
  UpdateBookPayload,
  UpdateFilePayload,
} from "@/types";

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
      queryClient.setQueryData([QueryKey.RetrieveBook, String(data.id)], data);
    },
  });
};

interface UpdateFileMutationVariables {
  id: number;
  payload: UpdateFilePayload;
}

export const useUpdateFile = () => {
  const queryClient = useQueryClient();

  return useMutation<File, ShishoAPIError, UpdateFileMutationVariables>({
    mutationFn: ({ id, payload }) => {
      return API.request("POST", `/books/files/${id}`, payload, null);
    },
    onSuccess: () => {
      // Invalidate book queries to refresh file data
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
    },
  });
};

interface UploadFileCoverVariables {
  id: number;
  file: globalThis.File;
}

export const useUploadFileCover = () => {
  const queryClient = useQueryClient();

  return useMutation<File, Error, UploadFileCoverVariables>({
    mutationFn: async ({ id, file }) => {
      const formData = new FormData();
      formData.append("cover", file);
      const response = await fetch(`/api/books/files/${id}/cover`, {
        method: "POST",
        body: formData,
      });
      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.error?.message || "Failed to upload cover");
      }
      return response.json();
    },
    onSuccess: () => {
      // Invalidate book queries to refresh file/cover data
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
    },
  });
};

interface SetFileCoverPageVariables {
  id: number;
  page: number;
}

export const useSetFileCoverPage = () => {
  const queryClient = useQueryClient();

  return useMutation<File, ShishoAPIError, SetFileCoverPageVariables>({
    mutationFn: ({ id, page }) => {
      return API.request(
        "PUT",
        `/books/files/${id}/cover-page`,
        { page },
        null,
      );
    },
    onSuccess: () => {
      // Invalidate book queries to refresh file/cover data
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
    },
  });
};

export * from "./resync";
