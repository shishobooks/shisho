import { useMutation, useQueryClient } from "@tanstack/react-query";

import { API } from "@/libraries/api";
import {
  Book,
  File,
  type ResyncBookResponse,
  type ResyncFileResponse,
  type ResyncPayload,
} from "@/types";

import { QueryKey } from "./books";
import { QueryKey as ChaptersQueryKey } from "./chapters";

export const useResyncFile = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      fileId,
      payload,
    }: {
      fileId: number;
      payload: ResyncPayload;
    }): Promise<File | ResyncFileResponse> => {
      return API.request<File | ResyncFileResponse>(
        "POST",
        `/books/files/${fileId}/resync`,
        payload,
      );
    },
    onSuccess: (result, { fileId }) => {
      // If not deleted, invalidate queries to refresh data
      if (!("file_deleted" in result && result.file_deleted)) {
        const file = result as File;
        queryClient.invalidateQueries({
          queryKey: [QueryKey.RetrieveBook, String(file.book_id)],
        });
      }
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({
        queryKey: [ChaptersQueryKey.FileChapters, fileId],
      });
    },
  });
};

export const useResyncBook = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      bookId,
      payload,
    }: {
      bookId: number;
      payload: ResyncPayload;
    }): Promise<Book | ResyncBookResponse> => {
      return API.request<Book | ResyncBookResponse>(
        "POST",
        `/books/${bookId}/resync`,
        payload,
      );
    },
    onSuccess: (_result, { bookId }) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveBook, String(bookId)],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
      queryClient.invalidateQueries({
        queryKey: [ChaptersQueryKey.FileChapters],
      });
    },
  });
};
