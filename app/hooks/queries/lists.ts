import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type {
  AddBooksPayload,
  CreateListPayload,
  CreateSharePayload,
  List,
  ListBook,
  ListBooksInListQuery,
  ListListsQuery,
  ListShare,
  RemoveBooksPayload,
  ReorderBooksPayload,
  UpdateListPayload,
  UpdateSharePayload,
} from "@/types";

export enum QueryKey {
  ListLists = "ListLists",
  RetrieveList = "RetrieveList",
  ListBooks = "ListBooks",
  ListShares = "ListShares",
  ListTemplates = "ListTemplates",
  BookLists = "BookLists",
}

export interface ListWithCount extends List {
  book_count: number;
  permission: "owner" | "manager" | "editor" | "viewer";
}

export interface ListListsData {
  lists: ListWithCount[];
  total: number;
}

export interface ListBooksData {
  books: ListBook[];
  total: number;
}

export interface RetrieveListData {
  list: List;
  book_count: number;
  permission: "owner" | "manager" | "editor" | "viewer";
}

export interface ListTemplate {
  name: string;
  display_name: string;
  description: string;
  is_ordered: boolean;
  default_sort: string;
}

export type { ListListsQuery, ListBooksInListQuery };

// ============================================================================
// Query Hooks
// ============================================================================

export const useListLists = (
  query: ListListsQuery = {},
  options: Omit<
    UseQueryOptions<ListListsData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListListsData, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListLists, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/lists", null, query, signal);
    },
  });
};

export const useList = (
  listId?: number,
  options: Omit<
    UseQueryOptions<RetrieveListData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<RetrieveListData, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(listId),
    ...options,
    queryKey: [QueryKey.RetrieveList, listId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/lists/${listId}`, null, null, signal);
    },
  });
};

export const useListBooks = (
  listId?: number,
  query: ListBooksInListQuery = {},
  options: Omit<
    UseQueryOptions<ListBooksData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListBooksData, ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(listId),
    ...options,
    queryKey: [QueryKey.ListBooks, listId, query],
    queryFn: ({ signal }) => {
      return API.request("GET", `/lists/${listId}/books`, null, query, signal);
    },
  });
};

export const useListShares = (
  listId?: number,
  options: Omit<
    UseQueryOptions<ListShare[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListShare[], ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(listId),
    ...options,
    queryKey: [QueryKey.ListShares, listId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/lists/${listId}/shares`, null, null, signal);
    },
  });
};

export const useListTemplates = (
  options: Omit<
    UseQueryOptions<ListTemplate[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListTemplate[], ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListTemplates],
    queryFn: ({ signal }) => {
      return API.request("GET", "/lists/templates", null, null, signal);
    },
  });
};

export const useBookLists = (
  bookId?: number,
  options: Omit<
    UseQueryOptions<List[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<List[], ShishoAPIError>({
    enabled: options.enabled !== undefined ? options.enabled : Boolean(bookId),
    ...options,
    queryKey: [QueryKey.BookLists, bookId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/books/${bookId}/lists`, null, null, signal);
    },
  });
};

// ============================================================================
// Mutation Hooks
// ============================================================================

export const useCreateList = () => {
  const queryClient = useQueryClient();

  return useMutation<List, ShishoAPIError, CreateListPayload>({
    mutationFn: (payload) => {
      return API.request("POST", "/lists", payload);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
    },
  });
};

export const useUpdateList = () => {
  const queryClient = useQueryClient();

  return useMutation<
    List,
    ShishoAPIError,
    { listId: number; payload: UpdateListPayload }
  >({
    mutationFn: ({ listId, payload }) => {
      return API.request("PATCH", `/lists/${listId}`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveList, variables.listId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
    },
  });
};

export const useDeleteList = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, { listId: number }>({
    mutationFn: ({ listId }) => {
      return API.request("DELETE", `/lists/${listId}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.BookLists] });
    },
  });
};

export const useAddBooksToList = () => {
  const queryClient = useQueryClient();

  return useMutation<
    void,
    ShishoAPIError,
    { listId: number; payload: AddBooksPayload }
  >({
    mutationFn: ({ listId, payload }) => {
      return API.request("POST", `/lists/${listId}/books`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveList, variables.listId],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.ListBooks, variables.listId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.BookLists] });
    },
  });
};

export const useRemoveBooksFromList = () => {
  const queryClient = useQueryClient();

  return useMutation<
    void,
    ShishoAPIError,
    { listId: number; payload: RemoveBooksPayload }
  >({
    mutationFn: ({ listId, payload }) => {
      return API.request("DELETE", `/lists/${listId}/books`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrieveList, variables.listId],
      });
      queryClient.invalidateQueries({
        queryKey: [QueryKey.ListBooks, variables.listId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
      queryClient.invalidateQueries({ queryKey: [QueryKey.BookLists] });
    },
  });
};

export const useReorderListBooks = () => {
  const queryClient = useQueryClient();

  return useMutation<
    void,
    ShishoAPIError,
    { listId: number; payload: ReorderBooksPayload }
  >({
    mutationFn: ({ listId, payload }) => {
      return API.request("PATCH", `/lists/${listId}/books/reorder`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.ListBooks, variables.listId],
      });
    },
  });
};

export const useCreateShare = () => {
  const queryClient = useQueryClient();

  return useMutation<
    ListShare,
    ShishoAPIError,
    { listId: number; payload: CreateSharePayload }
  >({
    mutationFn: ({ listId, payload }) => {
      return API.request("POST", `/lists/${listId}/shares`, payload);
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.ListShares, variables.listId],
      });
    },
  });
};

export const useUpdateShare = () => {
  const queryClient = useQueryClient();

  return useMutation<
    ListShare,
    ShishoAPIError,
    { listId: number; shareId: number; payload: UpdateSharePayload }
  >({
    mutationFn: ({ listId, shareId, payload }) => {
      return API.request(
        "PATCH",
        `/lists/${listId}/shares/${shareId}`,
        payload,
      );
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.ListShares, variables.listId],
      });
    },
  });
};

export const useDeleteShare = () => {
  const queryClient = useQueryClient();

  return useMutation<void, ShishoAPIError, { listId: number; shareId: number }>(
    {
      mutationFn: ({ listId, shareId }) => {
        return API.request("DELETE", `/lists/${listId}/shares/${shareId}`);
      },
      onSuccess: (_data, variables) => {
        queryClient.invalidateQueries({
          queryKey: [QueryKey.ListShares, variables.listId],
        });
        queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
      },
    },
  );
};

export const useCreateListFromTemplate = () => {
  const queryClient = useQueryClient();

  return useMutation<List, ShishoAPIError, { templateName: string }>({
    mutationFn: ({ templateName }) => {
      return API.request("POST", `/lists/templates/${templateName}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListLists] });
    },
  });
};
