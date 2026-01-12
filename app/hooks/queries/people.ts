import { QueryKey as BooksQueryKey } from "./books";
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";

import { API, ShishoAPIError } from "@/libraries/api";
import type { Book, File, Person } from "@/types";
import type { UpdatePersonPayload } from "@/types/generated/people";

export enum QueryKey {
  ListPeople = "ListPeople",
  RetrievePerson = "RetrievePerson",
  PersonAuthoredBooks = "PersonAuthoredBooks",
  PersonNarratedFiles = "PersonNarratedFiles",
}

export interface ListPeopleQuery {
  limit?: number;
  offset?: number;
  library_id?: number;
  search?: string;
}

export interface PersonWithCounts extends Person {
  authored_book_count: number;
  narrated_file_count: number;
}

export interface ListPeopleData {
  people: PersonWithCounts[];
  total: number;
}

export const usePeopleList = (
  query: ListPeopleQuery = {},
  options: Omit<
    UseQueryOptions<ListPeopleData, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<ListPeopleData, ShishoAPIError>({
    ...options,
    queryKey: [QueryKey.ListPeople, query],
    queryFn: ({ signal }) => {
      return API.request("GET", "/people", null, query, signal);
    },
  });
};

export const usePerson = (
  personId?: number,
  options: Omit<
    UseQueryOptions<PersonWithCounts, ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<PersonWithCounts, ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(personId),
    ...options,
    queryKey: [QueryKey.RetrievePerson, personId],
    queryFn: ({ signal }) => {
      return API.request("GET", `/people/${personId}`, null, null, signal);
    },
  });
};

export const usePersonAuthoredBooks = (
  personId?: number,
  options: Omit<
    UseQueryOptions<Book[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<Book[], ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(personId),
    ...options,
    queryKey: [QueryKey.PersonAuthoredBooks, personId],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/people/${personId}/authored-books`,
        null,
        null,
        signal,
      );
    },
  });
};

export const usePersonNarratedFiles = (
  personId?: number,
  options: Omit<
    UseQueryOptions<File[], ShishoAPIError>,
    "queryKey" | "queryFn"
  > = {},
) => {
  return useQuery<File[], ShishoAPIError>({
    enabled:
      options.enabled !== undefined ? options.enabled : Boolean(personId),
    ...options,
    queryKey: [QueryKey.PersonNarratedFiles, personId],
    queryFn: ({ signal }) => {
      return API.request(
        "GET",
        `/people/${personId}/narrated-files`,
        null,
        null,
        signal,
      );
    },
  });
};

export const useUpdatePerson = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      personId,
      payload,
    }: {
      personId: number;
      payload: UpdatePersonPayload;
    }) => {
      return API.request<PersonWithCounts>(
        "PATCH",
        `/people/${personId}`,
        payload,
      );
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrievePerson, variables.personId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListPeople] });
      // Invalidate book queries since they display author/narrator info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useMergePerson = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      targetId,
      sourceId,
    }: {
      targetId: number;
      sourceId: number;
    }) => {
      return API.request<void>("POST", `/people/${targetId}/merge`, {
        source_id: sourceId,
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: [QueryKey.RetrievePerson, variables.targetId],
      });
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListPeople] });
      // Invalidate book queries since they display author/narrator info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};

export const useDeletePerson = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ personId }: { personId: number }) => {
      return API.request<void>("DELETE", `/people/${personId}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QueryKey.ListPeople] });
      // Invalidate book queries since they display author/narrator info
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.ListBooks] });
      queryClient.invalidateQueries({ queryKey: [BooksQueryKey.RetrieveBook] });
    },
  });
};
