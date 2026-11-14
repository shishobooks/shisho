import { Check, ChevronLeft, ChevronRight, X } from "lucide-react";
import { useLayoutEffect, useMemo, useRef, type CSSProperties } from "react";
import { Link } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import SessionRow from "@/components/library/SessionRow";
import TruncatedText from "@/components/library/TruncatedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  useReadingStatsCalendar,
  useReadingStatsDay,
} from "@/hooks/queries/reading";
import { useAuth } from "@/hooks/useAuth";
import { useHoverTip } from "@/hooks/useHoverTip";
import { useSessionDelete } from "@/hooks/useSessionDelete";
import { DEMO_MODE_MESSAGE } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import type {
  ReadingStatsBookRef,
  ReadingStatsCalendarDay,
  ReadingStatsDay,
  ReadingStatsDaySession,
} from "@/types";
import {
  formatDayTime,
  formatDuration,
  formatLongDay,
  formatMonthName,
  formatShortDay,
} from "@/utils/format";
import {
  assignBookColors,
  bookColor,
  dayLanes,
  heatLevels,
  heatmapWeeks,
  layoutWeek,
  monthWeeks,
  textOn,
  type DayLane,
} from "@/utils/readingCalendar";

/** A calendar month, 1 to 12. */
export interface YearMonth {
  year: number;
  month: number;
}

const WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

const dayNumber = (day: string) => Number(day.slice(8));

const bookTitle = (ref: ReadingStatsBookRef) => ref.title || "Unknown book";

// The heatmap's five shades, from none to the most reading.
const HEAT_SHADES = [
  "bg-muted",
  "bg-reading/25",
  "bg-reading/50",
  "bg-reading/75",
  "bg-reading",
];

// Heatmap squares grow to fill the card up to this size, so a wide screen
// isn't left with the year bunched at one side.
const HEAT_CELL_MAX = "1.375rem";
const HEAT_CELL_MIN = "0.6875rem";
const HEAT_GAP = "3px";
// Months are labelled at least this many weeks apart, so a short first
// month doesn't run into the next label.
const MONTH_LABEL_GAP = 3;
const WEEKDAY_LABELS: Record<number, string> = { 1: "Mon", 3: "Wed", 5: "Fri" };

/**
 * The last 12 months, one square per day, darker for days with more
 * reading and listening. Selecting a square opens that day in the calendar.
 * It's drawn for sighted readers: the squares are out of the tab order, the
 * calendar below reaches every day, and a sentence under it gives screen
 * readers the year's total and busiest day.
 */
export const ReadingHeatmap = ({
  from,
  to,
  today,
  period,
  days,
  selectedDay,
  onSelectDay,
}: {
  /** The first and last days shown. */
  from: string;
  to: string;
  /** Days after it are still to come: empty, and nothing to open. */
  today: string;
  /** What the days are, for the total: "the last year", "2025". */
  period: string;
  days: ReadingStatsDay[];
  selectedDay: string | null;
  onSelectDay: (day: string) => void;
}) => {
  const hoverTip = useHoverTip();
  const scrollRef = useRef<HTMLDivElement>(null);
  const byDay = useMemo(
    () =>
      new Map(
        days.map((d) => [d.day, d.reading_seconds + d.listening_seconds]),
      ),
    [days],
  );
  const level = useMemo(() => heatLevels([...byDay.values()]), [byDay]);
  const weeks = useMemo(() => heatmapWeeks(from, to), [from, to]);
  const heatmapMaxWidth = `calc(2rem + ${weeks.length} * (${HEAT_CELL_MAX} + ${HEAT_GAP}))`;
  const total = [...byDay.values()].reduce((sum, s) => sum + s, 0);
  const busiest = [...byDay.entries()].sort((a, b) => b[1] - a[1])[0];

  const labels: { col: number; text: string }[] = [];
  weeks.forEach((week, w) => {
    const first = week.find((d) => d?.endsWith("-01"));
    const last = labels[labels.length - 1];
    if (first && (!last || w - last.col >= MONTH_LABEL_GAP)) {
      labels.push({
        col: w,
        text: formatMonthName(
          Number(first.slice(0, 4)),
          Number(first.slice(5, 7)),
        ),
      });
    }
  });

  // Where the year doesn't fit, bring the selected day into view, or
  // today, or the last day shown.
  const lastSoFar = today < to ? today : to;
  const focus =
    selectedDay && selectedDay >= from && selectedDay <= to
      ? selectedDay
      : lastSoFar;
  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el || el.scrollWidth <= el.clientWidth) return;
    const cell = el.querySelector(`[data-day="${focus}"]`);
    if (!cell) return;
    const offset =
      cell.getBoundingClientRect().left - el.getBoundingClientRect().left;
    el.scrollLeft += offset - el.clientWidth / 2;
  }, [focus]);

  // Hovering shows a tip, which re-renders the heatmap; the squares, a year
  // of them, only change with what they show.
  const { show: showTip } = hoverTip;
  const squares = useMemo(
    () =>
      weeks.flatMap((week, w) =>
        week.map((day, d) => {
          if (!day) return null;
          const seconds = byDay.get(day) ?? 0;
          // A day still to come has nothing to open. It stays hoverable, as a
          // disabled button would swallow the pointer and strand the tip.
          const future = day > today;
          return (
            <Button
              aria-disabled={future || undefined}
              aria-label={`Open ${formatLongDay(day)}`}
              className={cn(
                "aspect-square w-full rounded-[2px]",
                HEAT_SHADES[level(seconds)],
                future
                  ? "cursor-default opacity-50"
                  : "hover:ring-1 hover:ring-foreground",
                selectedDay === day && "ring-1 ring-foreground",
              )}
              data-day={day}
              data-level={level(seconds)}
              key={day}
              onClick={() => {
                if (!future) onSelectDay(day);
              }}
              onPointerEnter={(event) =>
                showTip(
                  event.currentTarget,
                  future ? (
                    formatShortDay(day)
                  ) : seconds > 0 ? (
                    <>
                      <span className="font-semibold">
                        {formatDuration(seconds)}
                      </span>{" "}
                      on {formatShortDay(day)}
                    </>
                  ) : (
                    `No reading on ${formatShortDay(day)}`
                  ),
                )
              }
              style={{ gridColumn: w + 2, gridRow: d + 2 }}
              tabIndex={-1}
              variant="unstyled"
            />
          );
        }),
      ),
    [weeks, byDay, level, today, selectedDay, onSelectDay, showTip],
  );

  return (
    <div>
      <p className="mb-3 text-xs text-muted-foreground">
        {total > 0
          ? `${formatDuration(total)} in ${period}`
          : `No reading in ${period}`}
      </p>
      <div className="overflow-x-auto pb-1" ref={scrollRef}>
        <div
          aria-hidden="true"
          // As wide as its squares, so the pinned weekday labels can follow
          // a phone's scroll; on a wide screen, the card's width up to the
          // squares' largest size.
          className="grid w-max"
          onPointerLeave={hoverTip.hide}
          style={{
            gap: HEAT_GAP,
            gridTemplateColumns: `auto repeat(${weeks.length}, minmax(${HEAT_CELL_MIN}, 1fr))`,
            minWidth: `min(100%, ${heatmapMaxWidth})`,
            maxWidth: heatmapMaxWidth,
          }}
        >
          {labels.map(({ col, text }) => (
            <span
              className="whitespace-nowrap text-[10px] leading-4 text-muted-foreground"
              key={`${col}-${text}`}
              style={{ gridColumn: col + 2, gridRow: 1 }}
            >
              {text}
            </span>
          ))}
          {/* One solid column, pinned so the labels stay while a phone
              scrolls the year, its rows the heatmap's own. */}
          <div
            className="sticky left-0 z-10 grid grid-rows-subgrid bg-background pr-1"
            style={{ gridColumn: 1, gridRow: "2 / 9" }}
          >
            {Object.entries(WEEKDAY_LABELS).map(([row, text]) => (
              <span
                className="self-center text-[10px] leading-none text-muted-foreground"
                key={text}
                style={{ gridRow: Number(row) + 1 }}
              >
                {text}
              </span>
            ))}
          </div>
          {squares}
        </div>
      </div>
      {hoverTip.tip}
      <div className="mt-2 flex justify-end text-xs text-muted-foreground">
        <span aria-hidden="true" className="flex items-center gap-1">
          Less
          {HEAT_SHADES.map((shade) => (
            <span
              className={cn("h-2.5 w-2.5 rounded-[2px]", shade)}
              key={shade}
            />
          ))}
          More
        </span>
      </div>
      <p className="sr-only">
        {busiest
          ? `Your most reading in ${period} was ${formatDuration(busiest[1])} on ${formatLongDay(busiest[0])}.`
          : ""}
      </p>
    </div>
  );
};

// Lanes of bars a week shows; more books than that on a day are counted.
const MAX_LANES = 3;

/**
 * A month of reading: a bar on each day for every Book read that day, one
 * continuous bar across consecutive days, in the Book's own colour, and a
 * check on the day a Read-through finished. Choosing a day opens its
 * Sessions under the grid.
 */
export const ReadingCalendar = ({
  month,
  canGoBack,
  canGoForward,
  onMonthChange,
  selectedDay,
  onSelectDay,
  today,
}: {
  month: YearMonth;
  canGoBack: boolean;
  canGoForward: boolean;
  onMonthChange: (by: number) => void;
  selectedDay: string | null;
  onSelectDay: (day: string | null) => void;
  today: string;
}) => {
  const calendarQuery = useReadingStatsCalendar(month);
  const calendar = calendarQuery.data;
  const monthTitle = formatMonthName(month.year, month.month, {
    long: true,
    withYear: true,
  });

  const renderGrid = () => {
    if (calendarQuery.error && !calendar) {
      return (
        <QueryError
          fallback="Failed to load the calendar"
          query={calendarQuery}
        />
      );
    }
    if (!calendar) {
      return <LoadingSpinner />;
    }
    const days = new Map<string, ReadingStatsCalendarDay>(
      calendar.days.map((d) => [d.day, d]),
    );
    const refs = new Map(calendar.books.map((b) => [b.key, b]));
    const colors = assignBookColors(calendar.books.map((b) => b.key));
    // The last month's bars stay, dimmed, until the next one's load.
    const stale =
      calendar.year !== month.year || calendar.month !== month.month;
    return (
      <div
        aria-busy={stale}
        className={cn("transition-opacity", stale && "opacity-60")}
      >
        <div
          aria-hidden="true"
          className="grid grid-cols-7 pb-1 text-center text-[11px] text-muted-foreground"
        >
          {WEEKDAYS.map((name) => (
            <span key={name}>{name}</span>
          ))}
        </div>
        <div className="divide-y divide-border rounded-md border border-border">
          {monthWeeks(month.year, month.month).map((week) => {
            const { items, hidden } = layoutWeek(week, days, MAX_LANES);
            return (
              <div
                className="grid min-h-16 grid-cols-7 gap-y-0.5 sm:min-h-24"
                data-week={week.find(Boolean)}
                key={week.find(Boolean)}
                style={{
                  gridTemplateRows: `1.5rem repeat(${MAX_LANES}, auto) 1fr`,
                }}
              >
                {week.map((day, col) => {
                  if (!day) {
                    return (
                      <span
                        className="bg-muted/30"
                        key={`blank-${col}`}
                        style={{ gridColumn: col + 1, gridRow: "1 / -1" }}
                      />
                    );
                  }
                  const books = days.get(day)?.books ?? [];
                  const label = [
                    formatLongDay(day),
                    ...books.map((b) => {
                      const title = bookTitle(
                        refs.get(b.key) ?? emptyRef(b.key),
                      );
                      return b.finished ? `${title}, finished` : title;
                    }),
                  ].join(". ");
                  return (
                    <Button
                      aria-label={
                        books.length > 0
                          ? label
                          : `${formatLongDay(day)}. No reading`
                      }
                      aria-pressed={selectedDay === day}
                      className={cn(
                        "flex items-start justify-start px-1 py-0.5 text-left text-xs hover:bg-accent/50",
                        col > 0 && "border-l border-border",
                        selectedDay === day && "bg-accent",
                      )}
                      key={day}
                      onClick={() =>
                        onSelectDay(selectedDay === day ? null : day)
                      }
                      style={{ gridColumn: col + 1, gridRow: "1 / -1" }}
                      variant="unstyled"
                    >
                      <span
                        className={cn(
                          "inline-flex h-5 min-w-5 items-center justify-center rounded-full px-1 tabular-nums",
                          day === today
                            ? "bg-primary font-semibold text-primary-foreground"
                            : "text-muted-foreground",
                        )}
                      >
                        {dayNumber(day)}
                      </span>
                    </Button>
                  );
                })}
                {items.map((item) => {
                  const ref = refs.get(item.key) ?? emptyRef(item.key);
                  const color = colors.get(item.key) ?? bookColor(item.key);
                  const row = item.lane + 2;
                  if (item.markerOnly) {
                    return (
                      <FinishMarker
                        color={color}
                        data-book={item.key}
                        key={`${item.key}-${item.start}`}
                        style={{ gridColumn: item.start + 1, gridRow: row }}
                      />
                    );
                  }
                  return [
                    <div
                      aria-hidden="true"
                      className={cn(
                        "pointer-events-none relative z-10 h-1.5 min-w-0 truncate text-[10px] font-medium leading-4 sm:h-4 sm:px-1",
                        item.continuesBefore ? "ml-0" : "ml-1 rounded-l-sm",
                        item.continuesAfter ? "mr-0" : "mr-1 rounded-r-sm",
                      )}
                      data-book={item.key}
                      data-continues={
                        [
                          item.continuesBefore && "before",
                          item.continuesAfter && "after",
                        ]
                          .filter(Boolean)
                          .join(" ") || undefined
                      }
                      data-span={`${item.start}-${item.end}`}
                      key={`${item.key}-${item.start}`}
                      style={{
                        backgroundColor: color,
                        color: textOn(color),
                        gridColumn: `${item.start + 1} / ${item.end + 2}`,
                        gridRow: row,
                      }}
                    >
                      <span className="hidden sm:inline">{bookTitle(ref)}</span>
                    </div>,
                    ...item.finishes.map((col) => (
                      <FinishMarker
                        color={color}
                        data-book={item.key}
                        key={`${item.key}-finish-${col}`}
                        style={{ gridColumn: col + 1, gridRow: row }}
                      />
                    )),
                  ];
                })}
                {hidden.map((count, col) =>
                  count > 0 ? (
                    <span
                      aria-hidden="true"
                      className="pointer-events-none z-10 self-end px-1 pb-0.5 text-[10px] text-muted-foreground"
                      key={`more-${col}`}
                      style={{ gridColumn: col + 1, gridRow: MAX_LANES + 2 }}
                    >
                      +{count}
                    </span>
                  ) : null,
                )}
                {/* The selected day's outline sits over its bars. */}
                {selectedDay && week.includes(selectedDay) && (
                  <span
                    aria-hidden="true"
                    className="pointer-events-none z-30 ring-2 ring-inset ring-primary"
                    data-selected-outline=""
                    style={{
                      gridColumn: week.indexOf(selectedDay) + 1,
                      gridRow: "1 / -1",
                    }}
                  />
                )}
              </div>
            );
          })}
        </div>
        {/* The marker stays with its words when the line wraps. */}
        <p className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <span>Each book keeps its own colour.</span>
          <span className="inline-flex items-center gap-1">
            <FinishMarker color="var(--reading)" />
            marks the day you finished one.
          </span>
        </p>
      </div>
    );
  };

  return (
    <section
      aria-label="Calendar"
      className="rounded-md border border-border p-4"
    >
      <div className="mb-3 flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold">{monthTitle}</h2>
        <div className="flex items-center gap-1">
          <Button
            aria-label="Previous month"
            disabled={!canGoBack}
            onClick={() => onMonthChange(-1)}
            size="icon-sm"
            variant="ghost"
          >
            <ChevronLeft />
          </Button>
          <Button
            aria-label="Next month"
            disabled={!canGoForward}
            onClick={() => onMonthChange(1)}
            size="icon-sm"
            variant="ghost"
          >
            <ChevronRight />
          </Button>
        </div>
      </div>
      {renderGrid()}
      {selectedDay && (
        <DayDetail
          colors={assignBookColors(calendar?.books.map((b) => b.key) ?? [])}
          day={selectedDay}
          onClose={() => onSelectDay(null)}
        />
      )}
    </section>
  );
};

const emptyRef = (key: string): ReadingStatsBookRef => ({
  key,
  title: "",
  removed: true,
  book: null,
});

const FinishMarker = ({
  color,
  className,
  style,
  ...props
}: {
  color: string;
  className?: string;
  style?: CSSProperties;
  "data-book"?: string;
}) => (
  <span
    aria-hidden="true"
    className={cn(
      "pointer-events-none z-20 inline-flex h-3 w-3 items-center justify-center self-center justify-self-end rounded-full ring-2 ring-background sm:mr-1 sm:h-4 sm:w-4",
      className,
    )}
    data-finish=""
    style={{ backgroundColor: color, color: textOn(color), ...style }}
    {...props}
  >
    <Check className="h-2 w-2 sm:h-3 sm:w-3" strokeWidth={3} />
  </span>
);

const BookName = ({
  book,
  color,
}: {
  book: ReadingStatsBookRef;
  color: string;
}) => {
  const title = (
    <TruncatedText className="truncate">{bookTitle(book)}</TruncatedText>
  );
  return (
    <div className="flex min-w-0 items-center gap-2">
      <span
        aria-hidden="true"
        className="h-2.5 w-2.5 shrink-0 rounded-sm"
        style={{ backgroundColor: color }}
      />
      <div className="min-w-0 text-sm font-medium">
        {book.book ? (
          <Link
            className="block min-w-0 hover:underline"
            to={`/libraries/${book.book.library_id}/books/${book.book.id}`}
          >
            {title}
          </Link>
        ) : (
          title
        )}
      </div>
      {book.removed && book.title && (
        <Badge className="shrink-0 text-[11px]" variant="outline">
          Removed from library
        </Badge>
      )}
      {!book.removed && !book.book && (
        <Badge className="shrink-0 text-[11px]" variant="outline">
          No access to this library
        </Badge>
      )}
    </div>
  );
};

// When a session ran on the day shown: "7:00 AM to 8:00 AM", with the date
// on a time that falls on another day.
const sessionWhen = (
  session: ReadingStatsDaySession,
  day: string,
  timeZone: string,
) =>
  `${formatDayTime(session.started_at, day, timeZone)} to ${formatDayTime(session.ended_at, day, timeZone)}`;

// Hours the day's timeline marks, with a label every six.
const TIMELINE_HOURS = [0, 3, 6, 9, 12, 15, 18, 21, 24];
const hourLabel = (hour: number) =>
  new Date(Date.UTC(2000, 0, 1, hour % 24)).toLocaleTimeString(undefined, {
    hour: "numeric",
    timeZone: "UTC",
  });

/**
 * A day as a 24-hour strip, a line per Book with a block for each Session
 * at the time of day it ran. It's drawn for sighted readers; the list under
 * it carries every Session's times.
 */
const DayTimeline = ({
  lanes,
  colors,
  day,
  timeZone,
}: {
  lanes: DayLane[];
  colors: Map<string, string>;
  day: string;
  timeZone: string;
}) => {
  const hoverTip = useHoverTip();
  return (
    <div
      aria-hidden="true"
      className="mb-4"
      data-timeline=""
      onPointerLeave={hoverTip.hide}
    >
      <div className="relative ml-0 h-4 text-[10px] text-muted-foreground">
        {TIMELINE_HOURS.filter((h) => h % 6 === 0).map((hour) => (
          <span
            className={cn(
              "absolute top-0 whitespace-nowrap",
              hour === 0
                ? ""
                : hour === 24
                  ? "-translate-x-full"
                  : "-translate-x-1/2",
            )}
            key={hour}
            style={{ left: `${(hour / 24) * 100}%` }}
          >
            {hourLabel(hour)}
          </span>
        ))}
      </div>
      <div className="relative space-y-1.5 py-1">
        {TIMELINE_HOURS.map((hour) => (
          <span
            className="absolute inset-y-0 border-l border-border"
            key={hour}
            style={{ left: `${(hour / 24) * 100}%` }}
          />
        ))}
        {lanes.map((lane) => {
          const color = colors.get(lane.book.key) ?? bookColor(lane.book.key);
          return (
            <div
              className="relative h-3 rounded-sm bg-muted/40"
              data-lane={lane.book.key}
              key={lane.book.key}
            >
              {lane.sessions.map((session) => (
                <span
                  className="absolute inset-y-0 rounded-sm hover:ring-2 hover:ring-foreground"
                  data-session={session.id}
                  key={session.id}
                  onPointerEnter={(event) =>
                    hoverTip.show(
                      event.currentTarget,
                      <>
                        <div className="font-semibold">
                          {bookTitle(lane.book)}
                        </div>
                        <div className="text-muted-foreground">
                          {formatDayTime(session.started_at, day, timeZone)} to{" "}
                          {formatDayTime(session.ended_at, day, timeZone)} ·{" "}
                          {formatDuration(session.seconds)}
                        </div>
                      </>,
                    )
                  }
                  style={{
                    backgroundColor: color,
                    left: `${(session.from / 1440) * 100}%`,
                    // A short Session still shows as a sliver.
                    width: `max(3px, ${((session.to - session.from) / 1440) * 100}%)`,
                  }}
                />
              ))}
            </div>
          );
        })}
      </div>
      {hoverTip.tip}
    </div>
  );
};

/** One day's Reading Sessions and finishes, under the calendar. */
const DayDetail = ({
  day,
  colors,
  onClose,
}: {
  day: string;
  colors: Map<string, string>;
  onClose: () => void;
}) => {
  const dayQuery = useReadingStatsDay({ day });
  const detail = dayQuery.data?.day === day ? dayQuery.data : undefined;
  const headingId = `reading-day-${day}`;
  const sessionDelete = useSessionDelete();
  const { demoMode } = useAuth();

  const renderBody = () => {
    if (dayQuery.error && !dayQuery.data) {
      return <QueryError fallback="Failed to load this day" query={dayQuery} />;
    }
    if (!detail) {
      return <LoadingSpinner className="py-4" />;
    }
    const lanes = dayLanes(detail.sessions, day, detail.time_zone);
    if (detail.sessions.length === 0 && detail.finished.length === 0) {
      return (
        <p className="text-sm text-muted-foreground">No reading on this day.</p>
      );
    }
    return (
      <div className="space-y-3">
        {detail.finished.length > 0 && (
          <ul aria-label="Finished" className="space-y-1">
            {detail.finished.map((book) => (
              <li className="flex items-center gap-2" key={book.key}>
                <span className="shrink-0 text-xs text-muted-foreground">
                  Finished
                </span>
                <BookName
                  book={book}
                  color={colors.get(book.key) ?? bookColor(book.key)}
                />
              </li>
            ))}
          </ul>
        )}
        {lanes.length > 0 && (
          <>
            <DayTimeline
              colors={colors}
              day={day}
              lanes={lanes}
              timeZone={detail.time_zone}
            />
            <ul aria-label="Reading sessions" className="space-y-3">
              {lanes.map((lane) => (
                <li
                  className="rounded-md border border-border"
                  key={lane.book.key}
                >
                  <div className="flex items-center gap-2 border-b border-border px-3 py-2">
                    <div className="min-w-0 flex-1">
                      <BookName
                        book={lane.book}
                        color={
                          colors.get(lane.book.key) ?? bookColor(lane.book.key)
                        }
                      />
                    </div>
                    <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                      {formatDuration(lane.seconds)}
                    </span>
                  </div>
                  <ul
                    aria-label={`Sessions of ${bookTitle(lane.book)}`}
                    className="divide-y divide-border"
                  >
                    {lane.sessions.map((session) => {
                      const title = bookTitle(session.book);
                      const when = sessionWhen(session, day, detail.time_zone);
                      return (
                        <SessionRow
                          className="pl-[2.125rem]"
                          key={session.id}
                          onDelete={() =>
                            sessionDelete.requestDelete(
                              session.id,
                              `${title}, ${when}`,
                            )
                          }
                          session={session}
                          title={title}
                          when={when}
                        />
                      );
                    })}
                  </ul>
                </li>
              ))}
            </ul>
            {demoMode && (
              <p className="text-xs text-muted-foreground">
                {DEMO_MODE_MESSAGE}
              </p>
            )}
          </>
        )}
      </div>
    );
  };

  return (
    <section
      aria-labelledby={headingId}
      className="mt-4 border-t border-border pt-4"
    >
      <div className="mb-3 flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold" id={headingId}>
          {formatLongDay(day)}
        </h3>
        <Button
          aria-label="Close day"
          onClick={onClose}
          size="icon-sm"
          variant="ghost"
        >
          <X />
        </Button>
      </div>
      {renderBody()}
      {sessionDelete.dialog}
    </section>
  );
};
