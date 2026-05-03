import { useQueries } from "@tanstack/react-query";
import { useMemo } from "react";

import { API } from "@/libraries/api";
import type { AuthorInput } from "@/types";

import {
  QueryKey as GenresQueryKey,
  useGenresList,
  type ListGenresData,
} from "./genres";
import { useImprintsList } from "./imprints";
import { usePeopleList, type PersonWithCounts } from "./people";
import { usePublishersList } from "./publishers";
import { useSeriesList } from "./series";
import {
  QueryKey as TagsQueryKey,
  useTagsList,
  type ListTagsData,
} from "./tags";

interface NameOption {
  name: string;
}

export type { NameOption };

export interface PersonOption extends NameOption {
  id: number;
  authored_book_count: number;
  narrated_file_count: number;
}

export interface NameWithBookCount extends NameOption {
  book_count: number;
}

export interface NameWithFileCount extends NameOption {
  file_count: number;
}

export function usePeopleSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: PersonOption[]; isLoading: boolean } {
  const { data, isLoading } = usePeopleList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.people.map((p: PersonWithCounts) => ({
    name: p.name,
    id: p.id,
    authored_book_count: p.authored_book_count,
    narrated_file_count: p.narrated_file_count,
  }));
  return { data: adapted, isLoading };
}

export function useAuthorSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: (AuthorInput & PersonOption)[]; isLoading: boolean } {
  return usePeopleSearch(libraryId, enabled, query);
}

export function useSeriesSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: NameWithBookCount[]; isLoading: boolean } {
  const { data, isLoading } = useSeriesList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.series.map((s) => ({
    name: s.name,
    book_count: s.book_count,
  }));
  return { data: adapted, isLoading };
}

export function usePublisherSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: NameWithFileCount[]; isLoading: boolean } {
  const { data, isLoading } = usePublishersList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.publishers.map((p) => ({
    name: p.name,
    file_count: p.file_count,
  }));
  return { data: adapted, isLoading };
}

export function useImprintSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: NameWithFileCount[]; isLoading: boolean } {
  const { data, isLoading } = useImprintsList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.imprints.map((i) => ({
    name: i.name,
    file_count: i.file_count,
  }));
  return { data: adapted, isLoading };
}

export function useGenreSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: NameWithBookCount[]; isLoading: boolean } {
  const { data, isLoading } = useGenresList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.genres.map((g) => ({
    name: g.name,
    book_count: g.book_count,
  }));
  return { data: adapted, isLoading };
}

export function useTagSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: NameWithBookCount[]; isLoading: boolean } {
  const { data, isLoading } = useTagsList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.tags.map((t) => ({
    name: t.name,
    book_count: t.book_count,
  }));
  return { data: adapted, isLoading };
}

export function useGenreItemCounts(
  libraryId: number | undefined,
  values: string[],
): Map<string, number> {
  const results = useQueries({
    queries: values.map((name) => ({
      queryKey: [
        GenresQueryKey.ListGenres,
        { library_id: libraryId, search: name, limit: 5 },
      ],
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        API.request<ListGenresData>(
          "GET",
          "/genres",
          null,
          { library_id: libraryId, search: name, limit: 5 },
          signal,
        ),
      enabled: !!libraryId,
      staleTime: 5 * 60 * 1000,
    })),
  });

  const dataKey = results.map((r) => r.dataUpdatedAt).join(",");
  return useMemo(() => {
    const map = new Map<string, number>();
    for (let i = 0; i < results.length; i++) {
      const genre = results[i].data?.genres.find(
        (g) => g.name.toLowerCase() === values[i].toLowerCase(),
      );
      if (genre) map.set(values[i], genre.book_count);
    }
    return map;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dataKey, values]);
}

export function useTagItemCounts(
  libraryId: number | undefined,
  values: string[],
): Map<string, number> {
  const results = useQueries({
    queries: values.map((name) => ({
      queryKey: [
        TagsQueryKey.ListTags,
        { library_id: libraryId, search: name, limit: 5 },
      ],
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        API.request<ListTagsData>(
          "GET",
          "/tags",
          null,
          { library_id: libraryId, search: name, limit: 5 },
          signal,
        ),
      enabled: !!libraryId,
      staleTime: 5 * 60 * 1000,
    })),
  });

  const dataKey = results.map((r) => r.dataUpdatedAt).join(",");
  return useMemo(() => {
    const map = new Map<string, number>();
    for (let i = 0; i < results.length; i++) {
      const tag = results[i].data?.tags.find(
        (t) => t.name.toLowerCase() === values[i].toLowerCase(),
      );
      if (tag) map.set(values[i], tag.book_count);
    }
    return map;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dataKey, values]);
}
