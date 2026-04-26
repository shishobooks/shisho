import { useQueries } from "@tanstack/react-query";

import {
  QueryKey as GenresQueryKey,
  type ListGenresData,
} from "@/hooks/queries/genres";
import {
  QueryKey as ImprintsQueryKey,
  type ListImprintsData,
} from "@/hooks/queries/imprints";
import {
  QueryKey as PeopleQueryKey,
  type ListPeopleData,
} from "@/hooks/queries/people";
import {
  QueryKey as PublishersQueryKey,
  type ListPublishersData,
} from "@/hooks/queries/publishers";
import {
  QueryKey as SeriesQueryKey,
  type ListSeriesData,
} from "@/hooks/queries/series";
import {
  QueryKey as TagsQueryKey,
  type ListTagsData,
} from "@/hooks/queries/tags";
import { API } from "@/libraries/api";

export interface AutoMatchInput {
  libraryId: number;
  enabled: boolean;
  authors: string[];
  narrators: string[];
  series: string[];
  publisher: string | undefined;
  imprint: string | undefined;
  genres: string[];
  tags: string[];
}

export interface MatchedEntity {
  name: string;
  existing: { id: number; name: string } | null;
}

export interface AutoMatchResult {
  isLoading: boolean;
  matches: {
    authors: MatchedEntity[];
    narrators: MatchedEntity[];
    series: MatchedEntity[];
    publisher: MatchedEntity | null;
    imprint: MatchedEntity | null;
    genres: MatchedEntity[];
    tags: MatchedEntity[];
  };
}

const eqLower = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();

function dedupe(names: (string | undefined)[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const name of names) {
    if (!name) continue;
    const trimmed = name.trim();
    if (!trimmed) continue;
    const key = trimmed.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(trimmed);
  }
  return out;
}

interface PoolKind<TData, TItem extends { id: number; name: string }> {
  queryKey: string;
  path: string;
  pluck: (data: TData) => TItem[];
}

const POOLS = {
  people: {
    queryKey: PeopleQueryKey.ListPeople,
    path: "/people",
    pluck: (d: ListPeopleData) => d.people,
  } as PoolKind<ListPeopleData, { id: number; name: string }>,
  series: {
    queryKey: SeriesQueryKey.ListSeries,
    path: "/series",
    pluck: (d: ListSeriesData) => d.series,
  } as PoolKind<ListSeriesData, { id: number; name: string }>,
  publishers: {
    queryKey: PublishersQueryKey.ListPublishers,
    path: "/publishers",
    pluck: (d: ListPublishersData) => d.publishers,
  } as PoolKind<ListPublishersData, { id: number; name: string }>,
  imprints: {
    queryKey: ImprintsQueryKey.ListImprints,
    path: "/imprints",
    pluck: (d: ListImprintsData) => d.imprints,
  } as PoolKind<ListImprintsData, { id: number; name: string }>,
  genres: {
    queryKey: GenresQueryKey.ListGenres,
    path: "/genres",
    pluck: (d: ListGenresData) => d.genres,
  } as PoolKind<ListGenresData, { id: number; name: string }>,
  tags: {
    queryKey: TagsQueryKey.ListTags,
    path: "/tags",
    pluck: (d: ListTagsData) => d.tags,
  } as PoolKind<ListTagsData, { id: number; name: string }>,
};

interface NameLookup {
  pool: keyof typeof POOLS;
  name: string;
}

function flattenLookups(input: AutoMatchInput): NameLookup[] {
  const lookups: NameLookup[] = [];
  const pushAll = (pool: keyof typeof POOLS, names: string[]) => {
    for (const name of names) lookups.push({ pool, name });
  };
  // People pool covers both authors and narrators — dedupe across them.
  pushAll("people", dedupe([...input.authors, ...input.narrators]));
  pushAll("series", dedupe(input.series));
  pushAll("publishers", dedupe([input.publisher]));
  pushAll("imprints", dedupe([input.imprint]));
  pushAll("genres", dedupe(input.genres));
  pushAll("tags", dedupe(input.tags));
  return lookups;
}

/**
 * Resolves incoming entity names against the local DB. Used by the identify
 * review form to mark each chip as either matching an existing library entity
 * or pending creation.
 *
 * Each name is looked up via a per-name `?search=<name>` list call rather than
 * a single bulk fetch — list endpoints cap at 50 rows, so a bulk fetch would
 * silently miss matches in large libraries (and the apply path would then
 * create duplicates server-side). React Query's caching dedupes repeat
 * lookups across renders.
 */
export function useAutoMatchEntities(input: AutoMatchInput): AutoMatchResult {
  const enabled = input.enabled && !!input.libraryId;
  const lookups = flattenLookups(input);

  const queries = useQueries({
    queries: lookups.map(({ pool, name }) => {
      const cfg = POOLS[pool];
      const queryParams = {
        library_id: input.libraryId,
        limit: 50,
        search: name,
      };
      return {
        queryKey: [cfg.queryKey, queryParams],
        queryFn: ({ signal }: { signal: AbortSignal }) =>
          API.request("GET", cfg.path, null, queryParams, signal),
        enabled,
        staleTime: 60_000,
      };
    }),
  });

  const isLoading = queries.some((q) => q.isLoading);

  const resolveMatch = (
    pool: keyof typeof POOLS,
    name: string,
  ): MatchedEntity => {
    const cfg = POOLS[pool];
    const idx = lookups.findIndex(
      (l) => l.pool === pool && eqLower(l.name, name),
    );
    if (idx < 0) return { name, existing: null };
    const data = queries[idx]?.data as
      | ListPeopleData
      | ListSeriesData
      | ListPublishersData
      | ListImprintsData
      | ListGenresData
      | ListTagsData
      | undefined;
    if (!data) return { name, existing: null };
    const items = cfg.pluck(data as never);
    const existing = items.find((p) => eqLower(p.name, name));
    return {
      name,
      existing: existing ? { id: existing.id, name: existing.name } : null,
    };
  };

  return {
    isLoading,
    matches: {
      authors: input.authors.map((n) => resolveMatch("people", n)),
      narrators: input.narrators.map((n) => resolveMatch("people", n)),
      series: input.series.map((n) => resolveMatch("series", n)),
      publisher: input.publisher
        ? resolveMatch("publishers", input.publisher)
        : null,
      imprint: input.imprint ? resolveMatch("imprints", input.imprint) : null,
      genres: input.genres.map((n) => resolveMatch("genres", n)),
      tags: input.tags.map((n) => resolveMatch("tags", n)),
    },
  };
}
