import type { AuthorInput } from "@/types";

import { useGenresList } from "./genres";
import { useImprintsList } from "./imprints";
import { usePeopleList, type PersonWithCounts } from "./people";
import { usePublishersList } from "./publishers";
import { useSeriesList } from "./series";
import { useTagsList } from "./tags";

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
