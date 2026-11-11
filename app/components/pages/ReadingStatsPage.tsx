import { History } from "lucide-react";
import { useEffect } from "react";
import { Link, useSearchParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import PaginationFooter, {
  type PageChangeHandler,
} from "@/components/library/PaginationFooter";
import QueryError from "@/components/library/QueryError";
import {
  BreakdownList,
  FinishedChart,
  FinishedList,
  KindList,
  PlaceList,
  RatingList,
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
import type { ReadingStatsResponse } from "@/types";
import {
  formatCount,
  formatDuration,
  formatRating,
  formatShowingRange,
} from "@/utils/format";
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

const hasActivity = (stats: ReadingStatsResponse) =>
  stats.totals.finished > 0 ||
  stats.totals.did_not_finish > 0 ||
  stats.totals.reading_seconds > 0 ||
  stats.totals.listening_seconds > 0;

const StatsBody = ({ stats }: { stats: ReadingStatsResponse }) => {
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
            totals.average_rating === null
              ? "None"
              : `${formatRating(totals.average_rating)} ★`
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

      <div className="grid gap-4 md:grid-cols-2 md:gap-6 xl:grid-cols-3">
        <StatsCard title="Genres">
          <BreakdownList
            breakdown={stats.genres}
            noun="genre"
            pluralNoun="genres"
          />
        </StatsCard>
        <StatsCard title="Tags">
          <BreakdownList breakdown={stats.tags} noun="tag" pluralNoun="tags" />
        </StatsCard>
        <StatsCard title="Authors">
          <BreakdownList
            breakdown={stats.authors}
            noun="author"
            pluralNoun="authors"
          />
        </StatsCard>
        <StatsCard title="Kind">
          <KindList kinds={stats.kinds} />
        </StatsCard>
        <StatsCard title="Ratings">
          <RatingList ratings={stats.ratings} />
        </StatsCard>
        <StatsCard title="Where you read">
          <PlaceList places={stats.places} />
        </StatsCard>
      </div>

      <FinishedSection year={stats.year} />
    </div>
  );
};

const FINISHED_PER_PAGE = 50;

// The books finished in the range, a page at a time, paged in the URL.
const FinishedSection = ({ year }: { year: number | null }) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parsePageParam(searchParams.get("page"));
  const offset = (currentPage - 1) * FINISHED_PER_PAGE;
  const finishedQuery = useReadingStatsFinished({
    ...(year === null ? {} : { year }),
    limit: FINISHED_PER_PAGE,
    offset,
  });
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
        <FinishedList rows={finishedQuery.data.items} />
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
      ref={sectionRef}
    >
      <h2 className="mb-3 text-sm font-semibold" id="reading-stats-finished">
        Finished
      </h2>
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
    return <StatsBody stats={stats} />;
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

        <p className="mt-6 text-xs text-muted-foreground">
          Kobo and KOReader don't time your reading the way the Shisho reader
          does, so time from them is estimated.
        </p>
      </ContentWidth>
    </div>
  );
};

export default ReadingStatsPage;
