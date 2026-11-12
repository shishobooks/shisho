import { Check, ChevronLeft, ChevronRight, Trash2, X } from "lucide-react";
import { useState, type CSSProperties } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import TruncatedText from "@/components/library/TruncatedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  useDeleteReadingSession,
  useReadingStatsCalendar,
  useReadingStatsDay,
} from "@/hooks/queries/reading";
import { toastRequestError } from "@/libraries/api";
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
  bookColor,
  heatLevels,
  heatmapWeeks,
  layoutWeek,
  monthWeeks,
} from "@/utils/readingCalendar";
import { SOURCE_LABELS } from "@/utils/readingStats";

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

/**
 * A year of reading and listening, one cell per day, darker for days with
 * more. It's drawn for sighted readers; a sentence under it gives screen
 * readers the busiest day.
 */
export const ReadingHeatmap = ({
  year,
  days,
}: {
  year: number;
  days: ReadingStatsDay[];
}) => {
  const [active, setActive] = useState<string | null>(null);
  const byDay = new Map(
    days
      .filter((d) => d.day.startsWith(`${year}-`))
      .map((d) => [d.day, d.reading_seconds + d.listening_seconds]),
  );
  const level = heatLevels([...byDay.values()]);
  const weeks = heatmapWeeks(year);
  const busiest = [...byDay.entries()].sort((a, b) => b[1] - a[1])[0];
  const activeSeconds = active ? (byDay.get(active) ?? 0) : 0;

  return (
    <div>
      <div className="overflow-x-auto pb-1">
        <div
          aria-hidden="true"
          className="inline-grid grid-flow-col gap-[3px]"
          onPointerLeave={() => setActive(null)}
          style={{
            gridTemplateRows: "auto repeat(7, 0.6875rem)",
            gridAutoColumns: "0.6875rem",
          }}
        >
          {weeks.map((week, w) => {
            const first = week.find((d) => d !== null)!;
            // A month's label goes over the week its first day falls in.
            const startsMonth = week.some((d) => d?.endsWith("-01"));
            const labelDay = week.find((d) => d?.endsWith("-01"));
            return [
              <span
                className="overflow-visible whitespace-nowrap text-[10px] leading-4 text-muted-foreground"
                key={`label-${first}`}
              >
                {startsMonth && labelDay
                  ? formatMonthName(year, Number(labelDay.slice(5, 7)))
                  : ""}
              </span>,
              ...week.map((day, d) =>
                day ? (
                  <span
                    className={cn(
                      "rounded-[2px]",
                      HEAT_SHADES[level(byDay.get(day) ?? 0)],
                      active === day && "ring-1 ring-foreground",
                    )}
                    data-day={day}
                    data-level={level(byDay.get(day) ?? 0)}
                    key={day}
                    onPointerEnter={() => setActive(day)}
                  />
                ) : (
                  <span key={`blank-${w}-${d}`} />
                ),
              ),
            ];
          })}
        </div>
      </div>
      <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
        <span aria-live="polite" className="tabular-nums">
          {active
            ? `${formatShortDay(active)}: ${activeSeconds > 0 ? formatDuration(activeSeconds) : "no reading"}`
            : " "}
        </span>
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
          ? `Your most reading in ${year} was ${formatDuration(busiest[1])} on ${formatLongDay(busiest[0])}.`
          : `No reading time in ${year}.`}
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
  enabled = true,
  canGoBack,
  canGoForward,
  onMonthChange,
  selectedDay,
  onSelectDay,
  today,
}: {
  month: YearMonth;
  /** False holds the month shown until the range's figures load. */
  enabled?: boolean;
  canGoBack: boolean;
  canGoForward: boolean;
  onMonthChange: (by: number) => void;
  selectedDay: string | null;
  onSelectDay: (day: string | null) => void;
  today: string;
}) => {
  const calendarQuery = useReadingStatsCalendar(month, { enabled });
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
                  const color = bookColor(item.key);
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
                        "pointer-events-none relative z-10 h-1.5 min-w-0 truncate text-[10px] font-medium leading-4 text-white sm:h-4 sm:px-1",
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
              </div>
            );
          })}
        </div>
        <p className="mt-2 flex items-center gap-1 text-xs text-muted-foreground">
          Each book keeps its own colour.
          <FinishMarker className="ml-1" color="var(--muted-foreground)" />
          marks the day you finished one.
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
        <DayDetail day={selectedDay} onClose={() => onSelectDay(null)} />
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
      "pointer-events-none z-20 inline-flex h-3 w-3 items-center justify-center self-center justify-self-end rounded-full text-white ring-2 ring-background sm:mr-1 sm:h-4 sm:w-4",
      className,
    )}
    data-finish=""
    style={{ backgroundColor: color, ...style }}
    {...props}
  >
    <Check className="h-2 w-2 sm:h-3 sm:w-3" strokeWidth={3} />
  </span>
);

const BookName = ({ book }: { book: ReadingStatsBookRef }) => {
  const title = (
    <TruncatedText className="truncate">{bookTitle(book)}</TruncatedText>
  );
  return (
    <div className="flex min-w-0 items-center gap-2">
      <span
        aria-hidden="true"
        className="h-2.5 w-2.5 shrink-0 rounded-sm"
        style={{ backgroundColor: bookColor(book.key) }}
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

const percent = (value: number) => `${Math.round(value * 100)}%`;

const SessionRow = ({
  session,
  day,
  timeZone,
  onDelete,
}: {
  session: ReadingStatsDaySession;
  day: string;
  timeZone: string;
  onDelete: () => void;
}) => {
  const details = [
    `${formatDayTime(session.started_at, day, timeZone)} to ${formatDayTime(session.ended_at, day, timeZone)}`,
    formatDuration(session.seconds),
    `${percent(session.start_percentage)} to ${percent(session.end_percentage)}`,
    session.kind === "listening" ? "Listening" : null,
    SOURCE_LABELS[session.source] + (session.measured ? "" : " (estimated)"),
  ].filter((part): part is string => part !== null);
  return (
    <li className="flex items-start gap-2 px-3 py-2">
      <div className="min-w-0 flex-1">
        <BookName book={session.book} />
        <p className="mt-0.5 pl-[1.125rem] text-xs text-muted-foreground">
          {details.join(" · ")}
        </p>
      </div>
      <Button
        aria-label={`Delete session of ${bookTitle(session.book)}`}
        className="shrink-0 text-muted-foreground hover:text-destructive"
        onClick={onDelete}
        size="icon-sm"
        variant="ghost"
      >
        <Trash2 />
      </Button>
    </li>
  );
};

/** One day's Reading Sessions and finishes, under the calendar. */
const DayDetail = ({ day, onClose }: { day: string; onClose: () => void }) => {
  const dayQuery = useReadingStatsDay({ day });
  const detail = dayQuery.data?.day === day ? dayQuery.data : undefined;
  const headingId = `reading-day-${day}`;
  const deleteMutation = useDeleteReadingSession();
  const [deleting, setDeleting] = useState<ReadingStatsDaySession | null>(null);

  const handleDelete = async () => {
    if (!deleting) return;
    try {
      await deleteMutation.mutateAsync(deleting.id);
      toast.success("Reading session deleted");
      setDeleting(null);
    } catch (error) {
      toastRequestError(error, "Failed to delete reading session");
    }
  };

  const renderBody = () => {
    if (dayQuery.error && !dayQuery.data) {
      return <QueryError fallback="Failed to load this day" query={dayQuery} />;
    }
    if (!detail) {
      return <LoadingSpinner className="py-4" />;
    }
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
                <BookName book={book} />
              </li>
            ))}
          </ul>
        )}
        {detail.sessions.length > 0 && (
          <ul
            aria-label="Reading sessions"
            className="divide-y divide-border rounded-md border border-border"
          >
            {detail.sessions.map((session) => (
              <SessionRow
                day={day}
                key={session.id}
                onDelete={() => setDeleting(session)}
                session={session}
                timeZone={detail.time_zone}
              />
            ))}
          </ul>
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
      <ConfirmDialog
        confirmLabel="Delete"
        description={
          deleting
            ? `${bookTitle(deleting.book)}, ${formatDayTime(deleting.started_at, day, detail?.time_zone ?? "UTC")} to ${formatDayTime(deleting.ended_at, day, detail?.time_zone ?? "UTC")}. Its time stops counting toward your Reading Stats, and it can't be brought back.`
            : ""
        }
        isPending={deleteMutation.isPending}
        onConfirm={handleDelete}
        onOpenChange={(open) => !open && setDeleting(null)}
        open={!!deleting}
        title="Delete this reading session?"
        variant="destructive"
      />
    </section>
  );
};
