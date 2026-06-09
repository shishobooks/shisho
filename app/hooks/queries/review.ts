import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  Book,
  File,
  PutReviewCriteriaPayload,
  ReviewCriteriaResponse,
  ReviewOverride,
} from "@/types";

import { QueryKey as BooksQueryKey } from "./books";

export enum QueryKey {
  ReviewCriteria = "ReviewCriteria",
}

export const useReviewCriteria = () =>
  useQuery<ReviewCriteriaResponse, ShishoAPIError>({
    queryKey: [QueryKey.ReviewCriteria],
    queryFn: ({ signal }) =>
      API.request("GET", "/settings/review-criteria", null, null, signal),
  });

export const useUpdateReviewCriteria = () => {
  const queryClient = useQueryClient();
  return useMutation<void, ShishoAPIError, PutReviewCriteriaPayload>({
    mutationFn: (payload) =>
      API.request("PUT", "/settings/review-criteria", payload, null),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ReviewCriteria] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

interface SetFileReviewVariables {
  fileId: number;
  override: ReviewOverride | null;
}

export const useSetFileReview = () => {
  const queryClient = useQueryClient();
  return useMutation<File, ShishoAPIError, SetFileReviewVariables>({
    mutationFn: ({ fileId, override }) =>
      API.request("PATCH", `/books/files/${fileId}/review`, { override }, null),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

interface SetBookReviewVariables {
  bookId: number;
  override: ReviewOverride | null;
}

export const useSetBookReview = () => {
  const queryClient = useQueryClient();
  return useMutation<Book, ShishoAPIError, SetBookReviewVariables>({
    mutationFn: ({ bookId, override }) =>
      API.request("PATCH", `/books/${bookId}/review`, { override }, null),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

interface BulkSetReviewVariables {
  bookIds: number[];
  override: ReviewOverride | null;
}

export const useBulkSetReview = () => {
  const queryClient = useQueryClient();
  return useMutation<void, ShishoAPIError, BulkSetReviewVariables>({
    mutationFn: ({ bookIds, override }) =>
      API.request(
        "POST",
        "/books/bulk/review",
        { book_ids: bookIds, override },
        null,
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};
