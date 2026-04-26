import { useGenresList } from "@/hooks/queries/genres";
import { useImprintsList } from "@/hooks/queries/imprints";
import { usePeopleList } from "@/hooks/queries/people";
import { usePublishersList } from "@/hooks/queries/publishers";
import { useSeriesList } from "@/hooks/queries/series";
import { useTagsList } from "@/hooks/queries/tags";

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

function matchByName<T extends { id: number; name: string }>(
  names: string[],
  pool: T[],
): MatchedEntity[] {
  return names.map((name) => {
    const existing = pool.find((p) => eqLower(p.name, name));
    return {
      name,
      existing: existing ? { id: existing.id, name: existing.name } : null,
    };
  });
}

export function useAutoMatchEntities(input: AutoMatchInput): AutoMatchResult {
  const peopleNeeded = input.authors.length > 0 || input.narrators.length > 0;
  const seriesNeeded = input.series.length > 0;
  const publisherNeeded = !!input.publisher;
  const imprintNeeded = !!input.imprint;
  const genresNeeded = input.genres.length > 0;
  const tagsNeeded = input.tags.length > 0;

  const enabled = input.enabled && !!input.libraryId;

  const peopleQuery = usePeopleList(
    { library_id: input.libraryId, limit: 50 },
    { enabled: enabled && peopleNeeded },
  );
  const seriesQuery = useSeriesList(
    { library_id: input.libraryId, limit: 50 },
    { enabled: enabled && seriesNeeded },
  );
  const publishersQuery = usePublishersList(
    { library_id: input.libraryId, limit: 50 },
    { enabled: enabled && publisherNeeded },
  );
  const imprintsQuery = useImprintsList(
    { library_id: input.libraryId, limit: 50 },
    { enabled: enabled && imprintNeeded },
  );
  const genresQuery = useGenresList(
    { library_id: input.libraryId, limit: 50 },
    { enabled: enabled && genresNeeded },
  );
  const tagsQuery = useTagsList(
    { library_id: input.libraryId, limit: 50 },
    { enabled: enabled && tagsNeeded },
  );

  const isLoading =
    (peopleNeeded && peopleQuery.isLoading) ||
    (seriesNeeded && seriesQuery.isLoading) ||
    (publisherNeeded && publishersQuery.isLoading) ||
    (imprintNeeded && imprintsQuery.isLoading) ||
    (genresNeeded && genresQuery.isLoading) ||
    (tagsNeeded && tagsQuery.isLoading);

  const peoplePool = peopleQuery.data?.people ?? [];
  const seriesPool = seriesQuery.data?.series ?? [];
  const publishersPool = publishersQuery.data?.publishers ?? [];
  const imprintsPool = imprintsQuery.data?.imprints ?? [];
  const genresPool = genresQuery.data?.genres ?? [];
  const tagsPool = tagsQuery.data?.tags ?? [];

  return {
    isLoading,
    matches: {
      authors: matchByName(input.authors, peoplePool),
      narrators: matchByName(input.narrators, peoplePool),
      series: matchByName(input.series, seriesPool),
      publisher: input.publisher
        ? matchByName([input.publisher], publishersPool)[0]
        : null,
      imprint: input.imprint
        ? matchByName([input.imprint], imprintsPool)[0]
        : null,
      genres: matchByName(input.genres, genresPool),
      tags: matchByName(input.tags, tagsPool),
    },
  };
}
