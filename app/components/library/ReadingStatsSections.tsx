import { Star } from "lucide-react";
import { useLayoutEffect, useRef, useState, type ReactNode } from "react";

import { ChartFrame, ChartTooltip } from "@/components/library/ChartFrame";
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
import { durationTicks, niceTicks, yPercent } from "@/utils/chartTicks";
import {
  formatCount,
  formatDate,
  formatDuration,
  formatRating,
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

const periodKey = (period: ReadingStatsPeriod) =>
  `${period.year}-${period.month ?? ""}`;

// A period with its year, for a tooltip: "March 2026", or "2026".
const periodTitle = (period: ReadingStatsPeriod) =>
  period.month
    ? `${MONTH_NAMES[period.month - 1]} ${period.year}`
    : String(period.year);

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

/**
 * A Rating as its number and a gold star, as the book page and book cards
 * show one, read out as "Rated 4.5 stars".
 */
export const RatingValue = ({
  stars,
  starClassName = "h-3.5 w-3.5",
}: {
  stars: number;
  starClassName?: string;
}) => (
  <span
    aria-label={`Rated ${formatCount(stars, "star")}`}
    className="inline-flex items-center gap-1"
    role="img"
  >
    {formatRating(stars)}
    <Star
      aria-hidden
      className={cn("shrink-0 fill-amber-400 text-amber-400", starClassName)}
    />
  </span>
);

export const StatTile = ({
  label,
  value,
  note,
}: {
  label: string;
  value: ReactNode;
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

// Bars for each period on the app's chart frame, with a tooltip for the
// one under the pointer. The chart is drawn for sighted readers; the table
// beside it carries the same figures for screen readers.
const PeriodBars = ({
  periods,
  caption,
  ticks,
  formatTick,
  renderBar,
  renderTooltip,
  describe,
}: {
  periods: ReadingStatsPeriod[];
  caption: string;
  ticks: number[];
  formatTick: (value: number) => string;
  renderBar: (period: ReadingStatsPeriod) => ReactNode;
  renderTooltip: (period: ReadingStatsPeriod) => ReactNode;
  describe: (period: ReadingStatsPeriod) => string;
}) => {
  const [active, setActive] = useState<number | null>(null);
  const count = periods.length;
  // The tooltip sits against the hovered column's outer edge.
  const tooltipX =
    active === null
      ? 0
      : ((active + 0.5) / count) * 100 > 50
        ? (active / count) * 100
        : ((active + 1) / count) * 100;
  return (
    <>
      <ChartFrame
        formatTick={formatTick}
        plotProps={{ onPointerLeave: () => setActive(null) }}
        ticks={ticks}
        xLabels={
          <div className="mt-1 flex gap-0.5 sm:gap-1">
            {periods.map((period) => (
              <span
                className="min-w-0 flex-1 truncate text-center text-[10px] text-muted-foreground"
                key={periodKey(period)}
              >
                {periodLabel(period)}
              </span>
            ))}
          </div>
        }
      >
        <div className="absolute inset-0 flex items-end gap-0.5 sm:gap-1">
          {periods.map((period, i) => (
            <div
              className={cn(
                "flex h-full min-w-0 flex-1 flex-col justify-end rounded-t",
                active === i && "bg-accent/50",
              )}
              data-period={periodKey(period)}
              key={periodKey(period)}
              onPointerEnter={() => setActive(i)}
            >
              {renderBar(period)}
            </div>
          ))}
        </div>
        {active !== null && (
          <ChartTooltip offset={4} xPercent={tooltipX}>
            {renderTooltip(periods[active])}
          </ChartTooltip>
        )}
      </ChartFrame>
      <table className="sr-only">
        <caption>{caption}</caption>
        <tbody>
          {periods.map((period) => (
            <tr key={periodKey(period)}>
              <th scope="row">{periodName(period)}</th>
              <td>{describe(period)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
};

const TooltipHeading = ({ children }: { children: ReactNode }) => (
  <div className="text-sm font-semibold tabular-nums text-foreground">
    {children}
  </div>
);

export const FinishedChart = ({
  periods,
}: {
  periods: ReadingStatsPeriod[];
}) => {
  const ticks = niceTicks(Math.max(0, ...periods.map((p) => p.finished)));
  return (
    <PeriodBars
      caption="Books finished"
      describe={(p) => formatCount(p.finished, "book")}
      formatTick={(t) => t.toLocaleString()}
      periods={periods}
      renderBar={(p) => (
        <div
          className={cn("w-full rounded-t", SERIES_BAR)}
          style={{ height: `${yPercent(ticks, p.finished)}%` }}
        />
      )}
      renderTooltip={(p) => (
        <>
          <TooltipHeading>{formatCount(p.finished, "book")}</TooltipHeading>
          <div className="text-muted-foreground">{periodTitle(p)}</div>
        </>
      )}
      ticks={ticks}
    />
  );
};

const TooltipSeries = ({
  className,
  label,
  seconds,
}: {
  className: string;
  label: string;
  seconds: number;
}) => (
  <div className="flex items-center gap-1.5 text-muted-foreground">
    <span className={cn("h-2 w-2 rounded-sm", className)} />
    {label}
    <span className="ml-auto pl-3 tabular-nums">{formatDuration(seconds)}</span>
  </div>
);

export const TimeChart = ({ periods }: { periods: ReadingStatsPeriod[] }) => {
  const axis = durationTicks(
    Math.max(0, ...periods.map((p) => p.reading_seconds + p.listening_seconds)),
  );
  return (
    <>
      <Legend />
      <PeriodBars
        caption="Reading and listening time"
        describe={(p) =>
          `${formatDuration(p.reading_seconds)} reading, ${formatDuration(p.listening_seconds)} listening`
        }
        formatTick={axis.format}
        periods={periods}
        renderBar={(p) => (
          // Listening stacks on reading, with a gap of the surface between.
          <div
            className="flex w-full flex-col gap-0.5"
            style={{
              height: `${yPercent(axis.ticks, p.reading_seconds + p.listening_seconds)}%`,
            }}
          >
            {p.listening_seconds > 0 && (
              <div
                className={cn("min-h-px rounded-t", LISTENING_BAR)}
                style={{ flexGrow: p.listening_seconds }}
              />
            )}
            {p.reading_seconds > 0 && (
              <div
                className={cn(
                  "min-h-px",
                  p.listening_seconds === 0 && "rounded-t",
                  READING_BAR,
                )}
                style={{ flexGrow: p.reading_seconds }}
              />
            )}
          </div>
        )}
        renderTooltip={(p) => (
          <>
            <TooltipHeading>
              {formatDuration(p.reading_seconds + p.listening_seconds)}
            </TooltipHeading>
            <div className="mb-1 text-muted-foreground">{periodTitle(p)}</div>
            <TooltipSeries
              className={READING_BAR}
              label="Reading"
              seconds={p.reading_seconds}
            />
            <TooltipSeries
              className={LISTENING_BAR}
              label="Listening"
              seconds={p.listening_seconds}
            />
          </>
        )}
        ticks={axis.ticks}
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

// Rows shown before "Show all".
const VISIBLE_ROWS = 5;
// How much of the last row an expanded list shows: enough to cut its name.
const PEEK = 0.45;

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
  // Expanded, the panel keeps the height it had collapsed, so "Show all"
  // never resizes the card, and the list scrolls inside it.
  const panelRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const toggleRef = useRef<HTMLButtonElement>(null);
  const [lockedHeight, setLockedHeight] = useState<number | null>(null);
  const [listHeight, setListHeight] = useState<number | null>(null);

  const needle = search.trim().toLowerCase();
  const shown = expanded
    ? rows.filter((row) => !needle || row.label.toLowerCase().includes(needle))
    : rows.slice(0, VISIBLE_ROWS);

  // The scrolling list ends partway into a row, through its
  // name, so the cut-off row shows there is more to scroll to. The toggle
  // stays at the bottom, where "Show all" was, and any room left over sits
  // between the two.
  useLayoutEffect(() => {
    const panel = panelRef.current;
    const list = listRef.current;
    const toggle = toggleRef.current;
    if (!expanded || !lockedHeight || !panel || !list || !toggle) {
      setListHeight(null);
      return;
    }
    const items = list.querySelectorAll("li");
    const stride =
      items.length > 1
        ? items[1].getBoundingClientRect().top -
          items[0].getBoundingClientRect().top
        : 0;
    const above =
      list.getBoundingClientRect().top - panel.getBoundingClientRect().top;
    // Expanded, the toggle's spacing is padding, part of its own height.
    const below = toggle.offsetHeight;
    const room = lockedHeight - above - below;
    if (stride <= 0 || list.scrollHeight <= room) {
      setListHeight(null);
      return;
    }
    const whole = Math.max(1, Math.floor(room / stride - PEEK));
    setListHeight((whole + PEEK) * stride);
  }, [expanded, lockedHeight, shown.length]);

  if (rows.length === 0) {
    return <EmptyNote>{emptyMessage}</EmptyNote>;
  }
  const max = Math.max(...rows.map((row) => row.books));
  return (
    <div
      className="flex flex-col"
      ref={panelRef}
      style={expanded && lockedHeight ? { height: lockedHeight } : undefined}
    >
      {/* Also what shows that the expanded list now scrolls. */}
      {expanded && (
        <Input
          aria-label={`Search ${pluralNoun}`}
          className="mb-2 h-8 shrink-0"
          onChange={(event) => setSearch(event.target.value)}
          placeholder={`Search ${pluralNoun}`}
          value={search}
        />
      )}
      <ul
        className={cn(
          "-mx-2 space-y-0.5",
          expanded && "min-h-0 shrink overflow-y-auto",
        )}
        ref={listRef}
        style={listHeight ? { height: listHeight } : undefined}
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
          className={cn("mt-2 self-start text-xs", expanded && "mt-auto pt-2")}
          onClick={() => {
            if (!expanded) {
              setLockedHeight(panelRef.current?.offsetHeight || null);
            }
            setExpanded(!expanded);
            setSearch("");
          }}
          ref={toggleRef}
          variant="link"
        >
          {expanded
            ? "Show fewer"
            : `Show all ${formatCount(rows.length, singularNoun, pluralNoun)}`}
        </Button>
      )}
    </div>
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

// A read's recorded time: nothing when none was, and "<1m" for less than a
// minute, which would otherwise read as none.
const readTime = (seconds: number | null) => {
  if (!seconds) return null;
  return seconds < 60 ? "<1m" : formatDuration(seconds);
};

const FinishedRow = ({ row }: { row: ReadingStatsFinishedBook }) => {
  const rt = row.read_through;
  const finished = rt.end_day ? formatDate(rt.end_day) : null;
  const days =
    row.days_taken !== null ? formatCount(row.days_taken, "day") : null;
  const time = readTime(row.seconds);
  const details = [
    finished ? `Finished ${finished}` : "Finish date unknown",
    days,
    time,
  ].filter((detail): detail is string => detail !== null);
  return (
    <li className="flex items-center gap-3 px-3 py-2">
      <ReadThroughCover book={row.book} />
      <ReadThroughDetails entry={row}>
        <span className="text-muted-foreground md:hidden">
          {details.join(" · ")}
          {row.rating !== null && (
            <>
              {" · "}
              <RatingValue starClassName="h-3 w-3" stars={row.rating} />
            </>
          )}
        </span>
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
        <span>{row.rating !== null && <RatingValue stars={row.rating} />}</span>
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
