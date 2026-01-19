import { QueryKey } from "./books";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { API } from "@/libraries/api";
import { Book, File } from "@/types";

export interface ResyncPayload {
  refresh: boolean;
}

export interface ResyncFileResult {
  file_deleted?: boolean;
  book_deleted?: boolean;
}

export interface ResyncBookResult {
  book_deleted?: boolean;
}

export const useResyncFile = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      fileId,
      payload,
    }: {
      fileId: number;
      payload: ResyncPayload;
    }): Promise<File | ResyncFileResult> => {
      return API.request<File | ResyncFileResult>(
        "POST",
        `/books/files/${fileId}/resync`,
        payload,
      );
    },
    onSuccess: (result) => {
      // If not deleted, invalidate queries to refresh data
      if (!("file_deleted" in result && result.file_deleted)) {
        const file = result as File;
        queryClient.invalidateQueries({
          queryKey: [QueryKey.RetrieveBook, String(file.book_id)],
        });
      }
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListBooks] });
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
    }): Promise<Book | ResyncBookResult> => {
      return API.request<Book | ResyncBookResult>(
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
    },
  });
};
