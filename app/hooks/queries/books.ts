import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  Book,
  DeleteBookResponse,
  DeleteBooksPayload,
  DeleteBooksResponse,
  DeleteFileResponse,
  File,
  ListBooksQuery,
  MergeBooksPayload,
  MergeBooksResponse,
  MoveFilesPayload,
  MoveFilesResponse,
  UpdateBookPayload,
  UpdateFilePayload,
} from "@/types";

import { QueryKey as GenresQueryKey } from "./genres";
import { QueryKey as ImprintsQueryKey } from "./imprints";
import { QueryKey as LibrariesQueryKey } from "./libraries";
import { QueryKey as PeopleQueryKey } from "./people";
import { QueryKey as PublishersQueryKey } from "./publishers";
import { QueryKey as SearchQueryKey } from "./search";
import { QueryKey as SeriesQueryKey } from "./series";
import { QueryKey as TagsQueryKey } from "./tags";

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
      // Book updates accept entity names; the server creates new persons,
      // series, genres, or tags as needed. Invalidate those caches so the
      // newly-created entities show up in admin pages and combobox results.
      queryClient.invalidateQueries({ queryKey: [PeopleQueryKey.ListPeople] });
      queryClient.invalidateQueries({ queryKey: [SeriesQueryKey.ListSeries] });
      queryClient.invalidateQueries({ queryKey: [GenresQueryKey.ListGenres] });
      queryClient.invalidateQueries({ queryKey: [TagsQueryKey.ListTags] });
      // Title, subtitle, authors, and series_names all feed books_fts —
      // invalidate global search results.
      queryClient.invalidateQueries({
        queryKey: [SearchQueryKey.GlobalSearch],
      });
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
      // Invalidate library languages in case a new language was set
      queryClient.invalidateQueries({
        queryKey: [LibrariesQueryKey.LibraryLanguages],
      });
      // File updates accept entity names; the server creates new narrators
      // (persons), publishers, or imprints as needed. Invalidate those caches
      // so the newly-created entities show up in admin pages and combobox results.
      queryClient.invalidateQueries({ queryKey: [PeopleQueryKey.ListPeople] });
      queryClient.invalidateQueries({
        queryKey: [PublishersQueryKey.ListPublishers],
      });
      queryClient.invalidateQueries({
        queryKey: [ImprintsQueryKey.ListImprints],
      });
      // Filenames and narrators both feed books_fts — invalidate search.
      queryClient.invalidateQueries({
        queryKey: [SearchQueryKey.GlobalSearch],
      });
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

interface MoveFilesMutationVariables {
  bookId: number;
  payload: MoveFilesPayload;
}

export const useMoveFiles = () => {
  const queryClient = useQueryClient();

  return useMutation<
    MoveFilesResponse,
    ShishoAPIError,
    MoveFilesMutationVariables
  >({
    mutationFn: ({ bookId, payload }) => {
      return API.request("POST", `/books/${bookId}/move-files`, payload, null);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
    },
  });
};

interface MergeBooksMutationVariables {
  payload: MergeBooksPayload;
}

export const useMergeBooks = () => {
  const queryClient = useQueryClient();

  return useMutation<
    MergeBooksResponse,
    ShishoAPIError,
    MergeBooksMutationVariables
  >({
    mutationFn: ({ payload }) => {
      return API.request("POST", "/books/merge", payload, null);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
      // Source book disappears, target book absorbs files — books_fts changes.
      queryClient.invalidateQueries({
        queryKey: [SearchQueryKey.GlobalSearch],
      });
    },
  });
};

// Delete book mutation
export const useDeleteBook = () => {
  const queryClient = useQueryClient();

  return useMutation<DeleteBookResponse, ShishoAPIError, number>({
    mutationFn: (bookId) => {
      return API.request("DELETE", `/books/${bookId}`, null, null);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
      queryClient.invalidateQueries({
        queryKey: [SearchQueryKey.GlobalSearch],
      });
    },
  });
};

// Delete file mutation
export const useDeleteFile = () => {
  const queryClient = useQueryClient();

  return useMutation<DeleteFileResponse, ShishoAPIError, number>({
    mutationFn: (fileId) => {
      return API.request("DELETE", `/books/files/${fileId}`, null, null);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
      queryClient.invalidateQueries({
        queryKey: [SearchQueryKey.GlobalSearch],
      });
    },
  });
};

// Bulk delete books mutation
export const useDeleteBooks = () => {
  const queryClient = useQueryClient();

  return useMutation<DeleteBooksResponse, ShishoAPIError, DeleteBooksPayload>({
    mutationFn: (payload) => {
      return API.request("POST", "/books/delete", payload, null);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.RetrieveBook] });
      queryClient.invalidateQueries({
        queryKey: [SearchQueryKey.GlobalSearch],
      });
    },
  });
};

interface SetPrimaryFileMutationVariables {
  bookId: number;
  fileId: number;
}

export const useSetPrimaryFile = () => {
  const queryClient = useQueryClient();

  return useMutation<Book, ShishoAPIError, SetPrimaryFileMutationVariables>({
    mutationFn: ({ bookId, fileId }) => {
      return API.request(
        "PUT",
        `/books/${bookId}/primary-file`,
        { file_id: fileId },
        null,
      );
    },
    onSuccess: (data: Book) => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.setQueryData([QueryKey.RetrieveBook, String(data.id)], data);
    },
  });
};

export * from "./resync";
