import { History } from "lucide-react";
import { useEffect } from "react";
import { Link, useSearchParams } from "react-router-dom";

import {
  FilterChip,
  type FilterChipKind,
} from "@/components/library/FilterChip";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter, {
  type PageChangeHandler,
} from "@/components/library/PaginationFooter";
import QueryError from "@/components/library/QueryError";
import {
  CountList,
  FinishedChart,
  FinishedList,
  PlaceList,
  RatingValue,
  StatsCard,
  StatTile,
  TimeChart,
} from "@/components/library/ReadingStatsSections";
import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ContentWidth } from "@/contexts/ContentWidth";
import {
  useReadingStats,
  useReadingStatsFinished,
} from "@/hooks/queries/reading";
import { useUserSettings } from "@/hooks/queries/settings";
import { usePageTitle } from "@/hooks/usePageTitle";
import {
  SCROLL_CLEAR_OF_NAV,
  useSectionPaging,
} from "@/hooks/useSectionPaging";
import { parsePageParam, resetPaging } from "@/libraries/pagination";
import { cn } from "@/libraries/utils";
import type { ReadingStatsKind, ReadingStatsResponse } from "@/types";
import {
  formatCount,
  formatDuration,
  formatShowingRange,
} from "@/utils/format";
import {
  countRows,
  KIND_LABELS,
  kindRows,
  ratingLabel,
  ratingRows,
} from "@/utils/readingStats";
import { todayDay } from "@/utils/readingStatus";

const ALL_TIME = "all";

// The range in the URL: a four-digit year, or "all" (null) for all time.
// Undefined when it's absent or anything else, which shows the current
// year. A year with no reading in it is still a year, and shows as empty.
const parseRange = (value: string | null): number | null | undefined => {
  if (value === ALL_TIME) return null;
  if (!value || !/^\d{4}$/.test(value)) return undefined;
  const year = Number(value);
  return year >= 1000 ? year : undefined;
};

// The breakdowns the finished books can be filtered by, each kept in its
// own URL parameter, and how each shows as a chip.
const FILTERS = [
  { param: "genre", chip: "genre", label: (value: string) => value },
  { param: "tag", chip: "tag", label: (value: string) => value },
  { param: "author", chip: "author", label: (value: string) => value },
  {
    param: "kind",
    chip: "fileType",
    label: (value: string) => KIND_LABELS[value as ReadingStatsKind],
  },
  {
    param: "rating",
    chip: "rating",
    label: (value: string) => ratingLabel(Number(value)),
  },
] as const satisfies readonly {
  param: string;
  chip: FilterChipKind;
  label: (value: string) => string;
}[];

type FilterParam = (typeof FILTERS)[number]["param"];
type FinishedFilters = Partial<Record<FilterParam, string>>;

const isRating = (value: string) => {
  const stars = Number(value);
  return Number.isInteger(stars * 4) && stars >= 0.25 && stars <= 5;
};

// The filters in the URL, leaving out a kind or rating the server would
// refuse.
const readFilters = (params: URLSearchParams): FinishedFilters => {
  const filters: FinishedFilters = {};
  for (const { param } of FILTERS) {
    const value = params.get(param);
    if (!value) continue;
    if (param === "kind" && !(value in KIND_LABELS)) continue;
    if (param === "rating" && !isRating(value)) continue;
    filters[param] = value;
  }
  return filters;
};

const hasActivity = (stats: ReadingStatsResponse) =>
  stats.totals.finished > 0 ||
  stats.totals.did_not_finish > 0 ||
  stats.totals.reading_seconds > 0 ||
  stats.totals.listening_seconds > 0;

const StatsBody = ({
  stats,
  filters,
  onToggleFilter,
  onClearFilter,
}: {
  stats: ReadingStatsResponse;
  filters: FinishedFilters;
  onToggleFilter: (param: FilterParam, value: string) => void;
  onClearFilter: (param: FilterParam) => void;
}) => {
  const { totals } = stats;
  const byYear = stats.year === null;
  return (
    <div className="space-y-4 md:space-y-6">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <StatTile
          label="Books finished"
          note={
            totals.rereads > 0
              ? formatCount(totals.rereads, "reread")
              : undefined
          }
          value={totals.finished.toLocaleString()}
        />
        <StatTile
          label="Did Not Finish"
          value={totals.did_not_finish.toLocaleString()}
        />
        <StatTile
          label="Reading time"
          value={formatDuration(totals.reading_seconds)}
        />
        <StatTile
          label="Listening time"
          value={formatDuration(totals.listening_seconds)}
        />
        <StatTile
          label="Median days to finish"
          value={
            totals.median_days_to_finish === null
              ? "None"
              : totals.median_days_to_finish.toLocaleString(undefined, {
                  maximumFractionDigits: 1,
                })
          }
        />
        <StatTile
          label="Average Rating"
          value={
            totals.average_rating === null ? (
              "None"
            ) : (
              <RatingValue
                starClassName="h-5 w-5"
                stars={totals.average_rating}
              />
            )
          }
        />
      </div>

      {stats.periods.length > 0 && (
        <div className="grid gap-4 md:grid-cols-2 md:gap-6">
          <StatsCard
            title={
              byYear ? "Books finished by year" : "Books finished by month"
            }
          >
            <FinishedChart periods={stats.periods} />
          </StatsCard>
          <StatsCard
            title={
              byYear
                ? "Reading and listening by year"
                : "Reading and listening by month"
            }
          >
            <TimeChart periods={stats.periods} />
          </StatsCard>
        </div>
      )}

      <div>
        <p className="mb-2 text-xs text-muted-foreground">
          Select a genre, tag, author, kind, or rating to list those books under
          Finished.
        </p>
        <div className="grid gap-4 md:grid-cols-2 md:gap-6 xl:grid-cols-3">
          <StatsCard title="Genres">
            <CountList
              emptyMessage="No finished books with genres."
              onSelect={(value) => onToggleFilter("genre", value)}
              pluralNoun="genres"
              rows={countRows(stats.genres)}
              selected={filters.genre ?? null}
              singularNoun="genre"
            />
          </StatsCard>
          <StatsCard title="Tags">
            <CountList
              emptyMessage="No finished books with tags."
              onSelect={(value) => onToggleFilter("tag", value)}
              pluralNoun="tags"
              rows={countRows(stats.tags)}
              selected={filters.tag ?? null}
              singularNoun="tag"
            />
          </StatsCard>
          <StatsCard title="Authors">
            <CountList
              emptyMessage="No finished books with authors."
              onSelect={(value) => onToggleFilter("author", value)}
              pluralNoun="authors"
              rows={countRows(stats.authors)}
              selected={filters.author ?? null}
              singularNoun="author"
            />
          </StatsCard>
          <StatsCard title="Kind">
            <CountList
              emptyMessage="No finished books."
              onSelect={(value) => onToggleFilter("kind", value)}
              pluralNoun="kinds"
              rows={kindRows(stats.kinds)}
              selected={filters.kind ?? null}
              singularNoun="kind"
            />
          </StatsCard>
          <StatsCard title="Ratings">
            <CountList
              emptyMessage="No finished books are rated."
              onSelect={(value) => onToggleFilter("rating", value)}
              pluralNoun="ratings"
              rows={ratingRows(stats.ratings)}
              selected={filters.rating ?? null}
              singularNoun="rating"
            />
          </StatsCard>
          <StatsCard title="Where you read">
            <PlaceList places={stats.places} />
          </StatsCard>
        </div>
      </div>

      <FinishedSection
        filters={filters}
        onClearFilter={onClearFilter}
        year={stats.year}
      />
    </div>
  );
};

const FINISHED_PER_PAGE = 50;
const FINISHED_SECTION_ID = "finished";

// The books finished in the range, a page at a time, paged in the URL.
const FinishedSection = ({
  year,
  filters,
  onClearFilter,
}: {
  year: number | null;
  filters: FinishedFilters;
  onClearFilter: (param: FilterParam) => void;
}) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parsePageParam(searchParams.get("page"));
  const offset = (currentPage - 1) * FINISHED_PER_PAGE;
  const finishedQuery = useReadingStatsFinished({
    ...(year === null ? {} : { year }),
    ...(filters.genre ? { genre: filters.genre } : {}),
    ...(filters.tag ? { tag: filters.tag } : {}),
    ...(filters.author ? { author: filters.author } : {}),
    ...(filters.kind ? { kind: filters.kind as ReadingStatsKind } : {}),
    ...(filters.rating ? { rating: Number(filters.rating) } : {}),
    limit: FINISHED_PER_PAGE,
    offset,
  });
  const activeFilters = FILTERS.filter((filter) => filters[filter.param]);
  const total = finishedQuery.data?.total ?? 0;

  const handlePageChange: PageChangeHandler = (page, options) => {
    const params = new URLSearchParams(searchParams);
    if (page === 1) params.delete("page");
    else params.set("page", page.toString());
    setSearchParams(params, options);
  };
  const { sectionRef, sectionPageChange } =
    useSectionPaging<HTMLElement>(handlePageChange);

  const renderList = () => {
    if (finishedQuery.error && !finishedQuery.data) {
      return (
        <QueryError
          fallback="Failed to load books finished"
          query={finishedQuery}
        />
      );
    }
    if (!finishedQuery.data) {
      return <LoadingSpinner />;
    }
    const showing = formatShowingRange(
      offset,
      FINISHED_PER_PAGE,
      total,
      "book",
      "books",
    );
    return (
      <>
        {showing && (
          <div className="mb-3 text-sm text-muted-foreground">{showing}</div>
        )}
        <FinishedList
          emptyMessage={
            activeFilters.length > 0
              ? "No finished books match these filters."
              : "No books finished."
          }
          rows={finishedQuery.data.items}
        />
        <PaginationFooter
          className="mt-6"
          currentPage={currentPage}
          onPageChange={sectionPageChange}
          totalIsCurrent={!finishedQuery.isPlaceholderData}
          totalPages={Math.ceil(total / FINISHED_PER_PAGE)}
        />
      </>
    );
  };

  return (
    <section
      aria-labelledby="reading-stats-finished"
      className={SCROLL_CLEAR_OF_NAV}
      id={FINISHED_SECTION_ID}
      ref={sectionRef}
    >
      <h2 className="mb-3 text-sm font-semibold" id="reading-stats-finished">
        Finished
      </h2>
      {activeFilters.length > 0 && (
        <div
          aria-label="Filters"
          className="mb-3 flex flex-wrap gap-2"
          role="group"
        >
          {activeFilters.map((filter) => (
            <FilterChip
              key={filter.param}
              kind={filter.chip}
              label={filter.label(filters[filter.param]!)}
              onRemove={() => onClearFilter(filter.param)}
            />
          ))}
        </div>
      )}
      {renderList()}
    </section>
  );
};

// The signed-in user's own reading, summarized for one year or all time.
// It takes no user, so it never shows anyone else's.
const ReadingStatsPage = () => {
  usePageTitle("Reading Stats");
  const settingsQuery = useUserSettings();
  const currentYear = Number(
    todayDay(settingsQuery.data?.time_zone).slice(0, 4),
  );
  const [searchParams, setSearchParams] = useSearchParams();
  const yearParam = searchParams.get("year");
  const parsed = parseRange(yearParam);
  const year = parsed === undefined ? currentYear : parsed;
  const statsQuery = useReadingStats(year === null ? {} : { year });
  const stats = statsQuery.data;

  // A "year" in the URL that isn't one goes at once, so the URL always
  // names what the page shows.
  const strayYear = yearParam !== null && parsed === undefined;
  useEffect(() => {
    if (!strayYear) return;
    setSearchParams(
      (prev) => {
        const params = new URLSearchParams(prev);
        params.delete("year");
        resetPaging(params);
        return params;
      },
      { replace: true, preventScrollReset: true },
    );
  }, [strayYear, setSearchParams]);

  // The current year is always offered, with or without reading in it, as
  // is the year in view.
  const years = [
    ...new Set([currentYear, ...(year ? [year] : []), ...(stats?.years ?? [])]),
  ].sort((a, b) => b - a);

  const setRange = (value: string) => {
    const params = new URLSearchParams(searchParams);
    if (value === String(currentYear)) params.delete("year");
    else params.set("year", value);
    resetPaging(params);
    setSearchParams(params, { preventScrollReset: true });
  };

  const filters = readFilters(searchParams);

  // Choosing a breakdown row lists its books under Finished, so bring that
  // into view; choosing it again, or removing its chip, clears it.
  const toggleFilter = (param: FilterParam, value: string) => {
    const params = new URLSearchParams(searchParams);
    const clearing = params.get(param) === value;
    if (clearing) params.delete(param);
    else params.set(param, value);
    resetPaging(params);
    setSearchParams(params, { preventScrollReset: true });
    if (!clearing) {
      requestAnimationFrame(() =>
        document
          .getElementById(FINISHED_SECTION_ID)
          ?.scrollIntoView({ block: "start", behavior: "smooth" }),
      );
    }
  };
  const clearFilter = (param: FilterParam) => {
    const params = new URLSearchParams(searchParams);
    params.delete(param);
    resetPaging(params);
    setSearchParams(params, { preventScrollReset: true });
  };

  const rangeLabel = year === null ? "all time" : String(year);

  const renderBody = () => {
    if (statsQuery.error && !stats) {
      return (
        <QueryError
          fallback="Failed to load Reading Stats"
          query={statsQuery}
        />
      );
    }
    if (!stats) {
      return <LoadingSpinner />;
    }
    if (!hasActivity(stats)) {
      return (
        <div className="rounded-md border border-border p-8 text-center text-muted-foreground">
          {year === null
            ? "Nothing here yet. Books you finish and time you spend reading show up here."
            : `No reading in ${year} yet. Books you finish and time you spend reading show up here.`}
        </div>
      );
    }
    return (
      // Keyed by range, so expanded panels and a hovered bar start over
      // with the new range's figures. The last range's figures stay, dimmed,
      // while the next one loads.
      <div
        aria-busy={statsQuery.isPlaceholderData}
        className={cn(
          "transition-opacity",
          statsQuery.isPlaceholderData && "opacity-60",
        )}
      >
        <StatsBody
          filters={filters}
          key={stats.year ?? "all"}
          onClearFilter={clearFilter}
          onToggleFilter={toggleFilter}
          stats={stats}
        />
      </div>
    );
  };

  return (
    <div>
      <TopNav />
      <ContentWidth className="px-4 py-4 md:px-6 md:py-8">
        <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <h1 className="mb-2 text-2xl font-semibold">Reading Stats</h1>
            <p className="text-sm text-muted-foreground md:text-base">
              Your reading for {rangeLabel}. Only you can see this.
            </p>
          </div>
          <div className="flex shrink-0 flex-wrap items-center gap-2">
            <Select
              onValueChange={setRange}
              value={year === null ? ALL_TIME : String(year)}
            >
              <SelectTrigger aria-label="Range" className="w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {years.map((y) => (
                  <SelectItem key={y} value={String(y)}>
                    {y}
                  </SelectItem>
                ))}
                <SelectItem value={ALL_TIME}>All time</SelectItem>
              </SelectContent>
            </Select>
            <Button asChild size="sm" variant="outline">
              <Link aria-label="Manage reading history" to="/reading-history">
                <History className="h-4 w-4" />
                <span className="hidden sm:inline">Manage reading history</span>
              </Link>
            </Button>
          </div>
        </div>

        {renderBody()}

        {stats?.places.some((place) => place.source !== "web_reader") && (
          <p className="mt-6 text-xs text-muted-foreground">
            Kobo and KOReader don't time your reading the way the Shisho reader
            does, so time from them is estimated.
          </p>
        )}
      </ContentWidth>
    </div>
  );
};

export default ReadingStatsPage;
