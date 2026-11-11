import type { ReactNode } from "react";

import ReadThroughCover from "@/components/library/ReadThroughCover";
import ReadThroughDetails from "@/components/library/ReadThroughDetails";
import TruncatedText from "@/components/library/TruncatedText";
import { cn } from "@/libraries/utils";
import type {
  ReadingStatsBreakdown,
  ReadingStatsFinishedBook,
  ReadingStatsKind,
  ReadingStatsKindCount,
  ReadingStatsPeriod,
  ReadingStatsPlace,
  ReadingStatsRatingCount,
} from "@/types";
import {
  formatCount,
  formatDate,
  formatDuration,
  formatShortDuration,
} from "@/utils/format";

// Reading and listening keep the same colours on every chart.
const READING_BAR = "bg-chart-1";
const LISTENING_BAR = "bg-chart-2";

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
          className="w-full rounded-t-sm bg-primary"
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

// A row with a name, a figure, and a bar sized against the largest row.
const BarRow = ({
  label,
  value,
  title,
  children,
}: {
  label: ReactNode;
  value: string;
  title?: string;
  children: ReactNode;
}) => (
  <li
    className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-2 gap-y-1 text-sm"
    title={title}
  >
    <div className="min-w-0">{label}</div>
    <span className="text-xs tabular-nums text-muted-foreground">{value}</span>
    <div className="col-span-2 flex h-1.5 overflow-hidden rounded-full bg-muted">
      {children}
    </div>
  </li>
);

const EmptyNote = ({ children }: { children: ReactNode }) => (
  <p className="text-sm text-muted-foreground">{children}</p>
);

export const BreakdownList = ({
  breakdown,
  noun,
  pluralNoun,
}: {
  breakdown: ReadingStatsBreakdown;
  noun: string;
  pluralNoun: string;
}) => {
  if (breakdown.items.length === 0) {
    return <EmptyNote>No finished books with {pluralNoun}.</EmptyNote>;
  }
  const max = Math.max(...breakdown.items.map((item) => item.books));
  return (
    <ul className="space-y-2">
      {breakdown.items.map((item) => (
        <BarRow
          key={item.name}
          label={
            <TruncatedText className="truncate">{item.name}</TruncatedText>
          }
          value={formatCount(item.books, "book")}
        >
          <div
            className="h-full rounded-full bg-primary"
            style={{ width: share(item.books, max) }}
          />
        </BarRow>
      ))}
      {breakdown.other && (
        <li className="flex justify-between gap-2 text-xs text-muted-foreground">
          <span>
            {formatCount(
              breakdown.other.names,
              `other ${noun}`,
              `other ${pluralNoun}`,
            )}
          </span>
          <span className="tabular-nums">
            {formatCount(breakdown.other.books, "book")}
          </span>
        </li>
      )}
    </ul>
  );
};

const KIND_LABELS: Record<ReadingStatsKind, string> = {
  ebook: "Ebooks",
  comic: "Comics",
  audiobook: "Audiobooks",
  unknown: "Unknown",
};

export const KindList = ({ kinds }: { kinds: ReadingStatsKindCount[] }) => {
  if (kinds.length === 0) {
    return <EmptyNote>No finished books.</EmptyNote>;
  }
  const max = Math.max(...kinds.map((k) => k.books));
  return (
    <ul className="space-y-2">
      {kinds.map((kind) => (
        <BarRow
          key={kind.kind}
          label={KIND_LABELS[kind.kind]}
          value={formatCount(kind.books, "book")}
        >
          <div
            className="h-full rounded-full bg-primary"
            style={{ width: share(kind.books, max) }}
          />
        </BarRow>
      ))}
    </ul>
  );
};

export const RatingList = ({
  ratings,
}: {
  ratings: ReadingStatsRatingCount[];
}) => {
  if (ratings.length === 0) {
    return <EmptyNote>No finished books are rated.</EmptyNote>;
  }
  const max = Math.max(...ratings.map((r) => r.books));
  // Highest first, as a reader scans a rating breakdown.
  const sorted = [...ratings].sort((a, b) => b.rating - a.rating);
  return (
    <ul className="space-y-2">
      {sorted.map((rating) => (
        <BarRow
          key={rating.rating}
          label={formatCount(rating.rating, "star")}
          value={formatCount(rating.books, "book")}
        >
          <div
            className="h-full rounded-full bg-primary"
            style={{ width: share(rating.books, max) }}
          />
        </BarRow>
      ))}
    </ul>
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
          <BarRow
            key={`${i}-${place.source}-${place.device_name ?? ""}`}
            label={
              <TruncatedText className="truncate">
                {placeLabel(place)}
              </TruncatedText>
            }
            title={placeTime(place)}
            value={formatDuration(
              place.reading_seconds + place.listening_seconds,
            )}
          >
            <div
              className={cn("h-full", READING_BAR)}
              style={{ width: share(place.reading_seconds, max) }}
            />
            <div
              className={cn("h-full", LISTENING_BAR)}
              style={{ width: share(place.listening_seconds, max) }}
            />
            <span className="sr-only">{placeTime(place)}</span>
          </BarRow>
        ))}
      </ul>
    </>
  );
};

const FinishedRow = ({ row }: { row: ReadingStatsFinishedBook }) => {
  const rt = row.read_through;
  const details: string[] = [
    rt.end_day ? `Finished ${formatDate(rt.end_day)}` : "Finish date unknown",
  ];
  if (row.days_taken !== null) {
    details.push(formatCount(row.days_taken, "day"));
  }
  if (row.seconds !== null) {
    details.push(formatDuration(row.seconds));
  }
  return (
    <li className="flex items-center gap-3 px-3 py-2">
      <ReadThroughCover book={row.book} />
      <ReadThroughDetails entry={row}>
        {details.map((detail) => (
          <span className="text-muted-foreground" key={detail}>
            {detail}
          </span>
        ))}
      </ReadThroughDetails>
    </li>
  );
};

export const FinishedList = ({
  rows,
}: {
  rows: ReadingStatsFinishedBook[];
}) => {
  if (rows.length === 0) {
    return <EmptyNote>No books finished.</EmptyNote>;
  }
  return (
    <ul
      aria-label="Books finished"
      className="divide-y divide-border rounded-md border border-border"
    >
      {rows.map((row) => (
        <FinishedRow key={row.read_through.id} row={row} />
      ))}
    </ul>
  );
};
