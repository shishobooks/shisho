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

export function usePeopleSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: (NameOption & { id: number })[]; isLoading: boolean } {
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
  }));
  return { data: adapted, isLoading };
}

export function useAuthorSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: (AuthorInput & { id: number })[]; isLoading: boolean } {
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
  }));
  return { data: adapted, isLoading };
}

export function useSeriesSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: NameOption[]; isLoading: boolean } {
  const { data, isLoading } = useSeriesList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.series.map((s) => ({ name: s.name }));
  return { data: adapted, isLoading };
}

export function usePublisherSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: NameOption[]; isLoading: boolean } {
  const { data, isLoading } = usePublishersList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.publishers.map((p) => ({ name: p.name }));
  return { data: adapted, isLoading };
}

export function useImprintSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: NameOption[]; isLoading: boolean } {
  const { data, isLoading } = useImprintsList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.imprints.map((i) => ({ name: i.name }));
  return { data: adapted, isLoading };
}

export function useGenreSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: string[]; isLoading: boolean } {
  const { data, isLoading } = useGenresList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.genres.map((g) => g.name);
  return { data: adapted, isLoading };
}

export function useTagSearch(
  libraryId: number | undefined,
  enabled: boolean,
  query: string,
): { data?: string[]; isLoading: boolean } {
  const { data, isLoading } = useTagsList(
    {
      library_id: libraryId,
      limit: 50,
      search: query.trim() || undefined,
    },
    { enabled: enabled && !!libraryId },
  );
  const adapted = data?.tags.map((t) => t.name);
  return { data: adapted, isLoading };
}
