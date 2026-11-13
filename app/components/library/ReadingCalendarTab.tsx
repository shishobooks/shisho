import { useSearchParams } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import {
  ReadingCalendar,
  ReadingHeatmap,
  type YearMonth,
} from "@/components/library/ReadingCalendar";
import { StatsCard } from "@/components/library/ReadingStatsSections";
import { useReadingStatsHeatmap } from "@/hooks/queries/reading";
import { shiftDay } from "@/utils/readingCalendar";

const monthIndex = ({ year, month }: YearMonth) => year * 12 + month - 1;
const fromMonthIndex = (index: number): YearMonth => ({
  year: Math.floor(index / 12),
  month: (index % 12) + 1,
});
const monthOf = (day: string): YearMonth => ({
  year: Number(day.slice(0, 4)),
  month: Number(day.slice(5, 7)),
});
const monthParam = ({ year, month }: YearMonth) =>
  `${year}-${String(month).padStart(2, "0")}`;

/**
 * Reading Stats' Calendar tab: the year up to today as a heatmap, a month
 * calendar that goes back to the first month with any reading, and the day
 * open under it. The month and day are kept in the URL; with no month, the
 * calendar opens on this one.
 */
const ReadingCalendarTab = () => {
  const heatmapQuery = useReadingStatsHeatmap();
  const heatmap = heatmapQuery.data;
  const [searchParams, setSearchParams] = useSearchParams();

  if (heatmapQuery.error && !heatmap) {
    return (
      <QueryError fallback="Failed to load your reading" query={heatmapQuery} />
    );
  }
  if (!heatmap) {
    return <LoadingSpinner />;
  }

  const thisMonth = monthOf(heatmap.today);
  const last = monthIndex(thisMonth);
  const first = Math.min(
    last,
    heatmap.first_day ? monthIndex(monthOf(heatmap.first_day)) : last,
  );

  let month = thisMonth;
  const raw = searchParams.get("month");
  if (raw && /^\d{4}-\d{2}$/.test(raw)) {
    const m = Number(raw.slice(5));
    const index = Number(raw.slice(0, 4)) * 12 + m - 1;
    if (m >= 1 && m <= 12 && index >= first && index <= last) {
      month = fromMonthIndex(index);
    }
  }
  // A day in the month in view, and a real one: not June 31.
  const rawDay = searchParams.get("day");
  const selectedDay =
    rawDay &&
    /^\d{4}-\d{2}-\d{2}$/.test(rawDay) &&
    rawDay.startsWith(`${monthParam(month)}-`) &&
    shiftDay(rawDay, 0) === rawDay
      ? rawDay
      : null;

  const update = (next: YearMonth, day: string | null) => {
    const params = new URLSearchParams(searchParams);
    if (monthIndex(next) === last) params.delete("month");
    else params.set("month", monthParam(next));
    if (day) params.set("day", day);
    else params.delete("day");
    setSearchParams(params, { preventScrollReset: true });
  };

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-4 md:gap-6">
      <StatsCard title="Reading by day">
        <ReadingHeatmap
          days={heatmap.days}
          from={heatmap.from}
          onSelectDay={(day) => update(monthOf(day), day)}
          selectedDay={selectedDay}
          today={heatmap.today}
        />
      </StatsCard>
      <ReadingCalendar
        canGoBack={monthIndex(month) > first}
        canGoForward={monthIndex(month) < last}
        month={month}
        onMonthChange={(by) =>
          update(fromMonthIndex(monthIndex(month) + by), null)
        }
        onSelectDay={(day) => update(month, day)}
        selectedDay={selectedDay}
        today={heatmap.today}
      />
    </div>
  );
};

export default ReadingCalendarTab;
