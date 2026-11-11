import { useState, type ReactNode } from "react";

import ReadThroughCover from "@/components/library/ReadThroughCover";
import ReadThroughDetails from "@/components/library/ReadThroughDetails";
import TruncatedText from "@/components/library/TruncatedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/libraries/utils";
import type {
  ReadingStatsFinishedBook,
  ReadingStatsPeriod,
  ReadingStatsPlace,
} from "@/types";
import {
  formatCount,
  formatDate,
  formatDuration,
  formatRating,
  formatShortDuration,
} from "@/utils/format";
import type { CountRow } from "@/utils/readingStats";

// Reading and listening keep the same colours on every chart.
const READING_BAR = "bg-reading";
const LISTENING_BAR = "bg-listening";
// A chart with one series uses the reading purple: the brand hue stepped
// for marks on each theme's surface, where the brand token itself turns pale
// in dark mode.
const SERIES_BAR = READING_BAR;

const MONTHS = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
];

const MONTH_NAMES = [
  "January",
  "February",
  "March",
  "April",
  "May",
  "June",
  "July",
  "August",
  "September",
  "October",
  "November",
  "December",
];

const periodLabel = (period: ReadingStatsPeriod) =>
  period.month ? MONTHS[period.month - 1] : String(period.year);

const periodName = (period: ReadingStatsPeriod) =>
  period.month ? MONTH_NAMES[period.month - 1] : String(period.year);

const share = (value: number, max: number) =>
  max > 0 ? `${(value / max) * 100}%` : "0%";

export const StatsCard = ({
  title,
  children,
  className,
}: {
  title: string;
  children: ReactNode;
  className?: string;
}) => (
  <section
    aria-label={title}
    className={cn("rounded-md border border-border p-4", className)}
  >
    <h2 className="mb-3 text-sm font-semibold">{title}</h2>
    {children}
  </section>
);

export const StatTile = ({
  label,
  value,
  note,
}: {
  label: string;
  value: string;
  note?: string;
}) => (
  <div
    aria-label={label}
    className="rounded-md border border-border p-3 md:p-4"
    role="group"
  >
    <div className="text-xs text-muted-foreground">{label}</div>
    <div className="mt-1 text-xl font-semibold tabular-nums md:text-2xl">
      {value}
    </div>
    {note && <div className="mt-0.5 text-xs text-muted-foreground">{note}</div>}
  </div>
);

const Legend = () => (
  <div className="mb-2 flex flex-wrap gap-3 text-xs text-muted-foreground">
    <span className="flex items-center gap-1">
      <span className={cn("h-2 w-2 rounded-sm", READING_BAR)} />
      Reading
    </span>
    <span className="flex items-center gap-1">
      <span className={cn("h-2 w-2 rounded-sm", LISTENING_BAR)} />
      Listening
    </span>
  </div>
);

// Columns for each period. The chart is drawn for sighted readers; the
// table beside it carries the same figures for screen readers.
const PeriodColumns = ({
  periods,
  caption,
  renderBar,
  renderValue,
  describe,
}: {
  periods: ReadingStatsPeriod[];
  caption: string;
  renderBar: (period: ReadingStatsPeriod) => ReactNode;
  renderValue: (period: ReadingStatsPeriod) => string;
  describe: (period: ReadingStatsPeriod) => string;
}) => (
  <>
    <div aria-hidden className="flex h-40 items-end gap-0.5 sm:gap-1">
      {periods.map((period) => (
        <div
          className="flex h-full min-w-0 flex-1 flex-col items-center"
          key={`${period.year}-${period.month ?? ""}`}
          title={`${periodName(period)}: ${describe(period)}`}
        >
          <div className="w-full truncate text-center text-[10px] tabular-nums text-muted-foreground">
            {renderValue(period)}
          </div>
          <div className="flex w-full flex-1 flex-col justify-end">
            {renderBar(period)}
          </div>
          <div className="mt-1 w-full truncate text-center text-[10px] text-muted-foreground">
            {periodLabel(period)}
          </div>
        </div>
      ))}
    </div>
    <table className="sr-only">
      <caption>{caption}</caption>
      <tbody>
        {periods.map((period) => (
          <tr key={`${period.year}-${period.month ?? ""}`}>
            <th scope="row">{periodName(period)}</th>
            <td>{describe(period)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  </>
);

export const FinishedChart = ({
  periods,
}: {
  periods: ReadingStatsPeriod[];
}) => {
  const max = Math.max(0, ...periods.map((p) => p.finished));
  return (
    <PeriodColumns
      caption="Books finished"
      describe={(p) => formatCount(p.finished, "book")}
      periods={periods}
      renderBar={(p) => (
        <div
          className={cn("w-full rounded-t-sm", SERIES_BAR)}
          style={{ height: share(p.finished, max) }}
        />
      )}
      renderValue={(p) => (p.finished > 0 ? p.finished.toLocaleString() : "")}
    />
  );
};

export const TimeChart = ({ periods }: { periods: ReadingStatsPeriod[] }) => {
  const max = Math.max(
    0,
    ...periods.map((p) => p.reading_seconds + p.listening_seconds),
  );
  return (
    <>
      <Legend />
      <PeriodColumns
        caption="Reading and listening time"
        describe={(p) =>
          `${formatDuration(p.reading_seconds)} reading, ${formatDuration(p.listening_seconds)} listening`
        }
        periods={periods}
        renderBar={(p) => (
          <div
            className="flex w-full flex-col overflow-hidden rounded-t-sm"
            style={{
              height: share(p.reading_seconds + p.listening_seconds, max),
            }}
          >
            <div
              className={LISTENING_BAR}
              style={{
                flexGrow: p.listening_seconds,
              }}
            />
            <div
              className={READING_BAR}
              style={{
                flexGrow: p.reading_seconds,
              }}
            />
          </div>
        )}
        renderValue={(p) => {
          const total = p.reading_seconds + p.listening_seconds;
          return total > 0 ? formatShortDuration(total) : "";
        }}
      />
    </>
  );
};

const EmptyNote = ({ children }: { children: ReactNode }) => (
  <p className="text-sm text-muted-foreground">{children}</p>
);

// A track under a figure, filled to its share of the largest.
const ShareBar = ({ children }: { children: ReactNode }) => (
  <span className="col-span-2 flex h-1.5 overflow-hidden rounded-full bg-muted">
    {children}
  </span>
);

// Rows shown before "Show all", and how many it takes to offer a search.
const VISIBLE_ROWS = 8;
const SEARCH_FROM = 20;

/**
 * A breakdown of finished books, most first. It shows the first few and
 * expands to all of them, with a search once there are many, so a library
 * with hundreds of tags stays usable. Selecting a row filters the finished
 * books to it; selecting it again clears that.
 */
export const CountList = ({
  rows,
  singularNoun,
  pluralNoun,
  emptyMessage,
  selected,
  onSelect,
}: {
  rows: CountRow[];
  singularNoun: string;
  pluralNoun: string;
  emptyMessage: string;
  selected: string | null;
  onSelect: (value: string) => void;
}) => {
  const [expanded, setExpanded] = useState(false);
  const [search, setSearch] = useState("");
  if (rows.length === 0) {
    return <EmptyNote>{emptyMessage}</EmptyNote>;
  }
  const max = Math.max(...rows.map((row) => row.books));
  const searchable = expanded && rows.length >= SEARCH_FROM;
  const needle = search.trim().toLowerCase();
  const shown = expanded
    ? rows.filter((row) => !needle || row.label.toLowerCase().includes(needle))
    : rows.slice(0, VISIBLE_ROWS);
  return (
    <>
      {searchable && (
        <Input
          aria-label={`Search ${pluralNoun}`}
          className="mb-2 h-8"
          onChange={(event) => setSearch(event.target.value)}
          placeholder={`Search ${pluralNoun}`}
          value={search}
        />
      )}
      <ul
        className={cn(
          "-mx-2 space-y-0.5",
          expanded && "max-h-80 overflow-y-auto",
        )}
      >
        {shown.map((row) => {
          const active = selected === row.value;
          return (
            <li key={row.value}>
              <Button
                aria-pressed={active}
                className={cn(
                  "grid w-full cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-x-2 gap-y-1 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent/60",
                  active && "bg-accent",
                )}
                onClick={() => onSelect(row.value)}
                variant="unstyled"
              >
                <span className="min-w-0 truncate" title={row.label}>
                  {row.label}
                </span>
                <span className="text-xs tabular-nums text-muted-foreground">
                  {formatCount(row.books, "book")}
                </span>
                <ShareBar>
                  <span
                    className={cn("h-full rounded-full", SERIES_BAR)}
                    style={{ width: share(row.books, max) }}
                  />
                </ShareBar>
              </Button>
            </li>
          );
        })}
        {shown.length === 0 && (
          <li className="px-2 text-sm text-muted-foreground">
            No {pluralNoun} match.
          </li>
        )}
      </ul>
      {rows.length > VISIBLE_ROWS && (
        <Button
          className="mt-2 text-xs"
          onClick={() => {
            setExpanded(!expanded);
            setSearch("");
          }}
          variant="link"
        >
          {expanded
            ? "Show fewer"
            : `Show all ${formatCount(rows.length, singularNoun, pluralNoun)}`}
        </Button>
      )}
    </>
  );
};

const SOURCE_LABELS: Record<ReadingStatsPlace["source"], string> = {
  web_reader: "Shisho reader",
  kobo: "Kobo",
  koreader: "KOReader",
};

const placeLabel = (place: ReadingStatsPlace) =>
  place.device_name ?? SOURCE_LABELS[place.source];

const placeTime = (place: ReadingStatsPlace) => {
  const parts: string[] = [];
  if (place.reading_seconds > 0)
    parts.push(`${formatDuration(place.reading_seconds)} reading`);
  if (place.listening_seconds > 0)
    parts.push(`${formatDuration(place.listening_seconds)} listening`);
  return parts.join(", ") || formatDuration(0);
};

export const PlaceList = ({ places }: { places: ReadingStatsPlace[] }) => {
  if (places.length === 0) {
    return <EmptyNote>No reading time recorded.</EmptyNote>;
  }
  const max = Math.max(
    ...places.map((p) => p.reading_seconds + p.listening_seconds),
  );
  return (
    <>
      <Legend />
      <ul className="space-y-2">
        {places.map((place, i) => (
          // Two Devices can share a name, so the place in the list keys it.
          <li
            className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-2 gap-y-1 text-sm"
            key={`${i}-${place.source}-${place.device_name ?? ""}`}
            title={placeTime(place)}
          >
            <TruncatedText className="min-w-0 truncate">
              {placeLabel(place)}
            </TruncatedText>
            <span className="text-xs tabular-nums text-muted-foreground">
              {formatDuration(place.reading_seconds + place.listening_seconds)}
            </span>
            <ShareBar>
              <span
                className={cn("h-full", READING_BAR)}
                style={{ width: share(place.reading_seconds, max) }}
              />
              <span
                className={cn("h-full", LISTENING_BAR)}
                style={{ width: share(place.listening_seconds, max) }}
              />
              <span className="sr-only">{placeTime(place)}</span>
            </ShareBar>
          </li>
        ))}
      </ul>
    </>
  );
};

// Wide screens put a finished book's figures in columns at the end of its
// row, under one header; narrow ones keep them on a line under its title.
const FINISHED_COLUMNS =
  "md:grid md:grid-cols-[7rem_4.5rem_5rem_4.5rem] md:gap-4 md:text-right";

const FinishedRow = ({ row }: { row: ReadingStatsFinishedBook }) => {
  const rt = row.read_through;
  const finished = rt.end_day ? formatDate(rt.end_day) : null;
  const days =
    row.days_taken !== null ? formatCount(row.days_taken, "day") : null;
  const time = row.seconds !== null ? formatDuration(row.seconds) : null;
  const rating = row.rating !== null ? `${formatRating(row.rating)} ★` : null;
  const details = [
    finished ? `Finished ${finished}` : "Finish date unknown",
    days,
    time,
    rating,
  ].filter((detail): detail is string => detail !== null);
  return (
    <li className="flex items-center gap-3 px-3 py-2">
      <ReadThroughCover book={row.book} />
      <ReadThroughDetails entry={row}>
        {details.map((detail) => (
          <span className="text-muted-foreground md:hidden" key={detail}>
            {detail}
          </span>
        ))}
      </ReadThroughDetails>
      <div
        className={cn(
          "hidden shrink-0 text-sm tabular-nums text-muted-foreground",
          FINISHED_COLUMNS,
        )}
      >
        <span>
          <span className="sr-only">Finished </span>
          {finished ?? "Unknown"}
        </span>
        <span>{days}</span>
        <span>{time}</span>
        <span>
          {row.rating !== null && (
            <>
              <span className="sr-only">Rated </span>
              {formatRating(row.rating)}{" "}
              <span aria-hidden className="text-foreground">
                ★
              </span>
            </>
          )}
        </span>
      </div>
    </li>
  );
};

export const FinishedList = ({
  rows,
  emptyMessage,
}: {
  rows: ReadingStatsFinishedBook[];
  emptyMessage: string;
}) => {
  if (rows.length === 0) {
    return <EmptyNote>{emptyMessage}</EmptyNote>;
  }
  return (
    <>
      <div
        aria-hidden
        className="hidden justify-end px-3 pb-1 text-xs text-muted-foreground md:flex"
      >
        <div className={FINISHED_COLUMNS}>
          <span>Finished</span>
          <span>Days</span>
          <span>Time</span>
          <span>Rating</span>
        </div>
      </div>
      <ul
        aria-label="Books finished"
        className="divide-y divide-border rounded-md border border-border"
      >
        {rows.map((row) => (
          <FinishedRow key={row.read_through.id} row={row} />
        ))}
      </ul>
    </>
  );
};
