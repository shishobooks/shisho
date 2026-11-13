import { Loader2 } from "lucide-react";
import { Fragment, useEffect, useId, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router-dom";

import LibraryGrowthChart from "@/components/library/LibraryGrowthChart";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import TruncatedText from "@/components/library/TruncatedText";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  useServerStats,
  useServerStatsReading,
} from "@/hooks/queries/serverStats";
import { useIsPhone } from "@/hooks/useIsPhone";
import { usePageTitle } from "@/hooks/usePageTitle";
import { cn } from "@/libraries/utils";
import type {
  CacheSize,
  ContentCounts,
  StatsResponse,
  Volume,
  VolumePath,
  VolumePathKind,
} from "@/types/generated/serverstats";
import {
  fileTypeLabel,
  formatCount,
  formatDuration,
  formatFileSize,
} from "@/utils/format";

interface SectionProps {
  title: string;
  description?: string;
  /** A control for the whole section, beside its title. */
  action?: ReactNode;
  children: ReactNode;
}

const Section = ({ title, description, action, children }: SectionProps) => {
  const headingId = useId();
  return (
    <section
      aria-labelledby={headingId}
      // min-w-0 lets a grid item shrink below its content, so the Libraries
      // table scrolls inside its card instead of widening the page.
      className="min-w-0 border border-border rounded-md p-4 md:p-6"
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h2
            className="text-base md:text-lg font-semibold mb-1"
            id={headingId}
          >
            {title}
          </h2>
          {description && (
            <p className="text-xs md:text-sm text-muted-foreground">
              {description}
            </p>
          )}
        </div>
        {action && <div className="shrink-0">{action}</div>}
      </div>
      <div className="mt-3 md:mt-4">{children}</div>
    </section>
  );
};

interface StatProps {
  label: string;
  value: string;
  /** Qualifies the figure, under it ("Last 30 days"). */
  note?: string;
}

const Stat = ({ label, value, note }: StatProps) => (
  <div aria-label={label} className="min-w-0" role="group">
    <div className="text-xs md:text-sm text-muted-foreground">{label}</div>
    <div className="text-xl md:text-2xl font-semibold tabular-nums break-words">
      {value}
    </div>
    {note && <div className="text-xs text-muted-foreground">{note}</div>}
  </div>
);

const totalFiles = (counts: ContentCounts) =>
  counts.files.reduce((sum, f) => sum + f.count, 0) + counts.supplement_files;

// Each format as its own chip, most common first, then supplements in a
// dashed chip since they aren't a format.
const FilesBreakdown = ({
  name,
  counts,
}: {
  name: string;
  counts: ContentCounts;
}) => {
  const files = [...counts.files].sort(
    (a, b) =>
      b.count - a.count ||
      fileTypeLabel(a.file_type).localeCompare(fileTypeLabel(b.file_type)),
  );
  if (files.length === 0 && counts.supplement_files === 0) {
    return <span className="text-muted-foreground">None</span>;
  }
  const chip =
    "inline-flex items-baseline gap-1 rounded-md px-1.5 py-0.5 text-xs";
  return (
    <ul aria-label={`Files in ${name}`} className="flex flex-wrap gap-1">
      {files.map((f) => (
        <li className={cn(chip, "bg-secondary")} key={f.file_type}>
          <span className="text-muted-foreground">
            {fileTypeLabel(f.file_type)}
          </span>
          <span className="tabular-nums">{f.count.toLocaleString()}</span>
        </li>
      ))}
      {counts.supplement_files > 0 && (
        <li className={cn(chip, "border border-dashed border-border")}>
          <span className="text-muted-foreground">Supplements</span>
          <span className="tabular-nums">
            {counts.supplement_files.toLocaleString()}
          </span>
        </li>
      )}
    </ul>
  );
};

const Overview = ({ stats }: { stats: StatsResponse }) => (
  <Section title="Overview">
    <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-4">
      <Stat label="Books" value={stats.totals.books.toLocaleString()} />
      <Stat label="Files" value={totalFiles(stats.totals).toLocaleString()} />
      <Stat
        label="Library size"
        value={formatFileSize(stats.totals.size_bytes)}
      />
      <Stat label="Users" value={stats.users.toLocaleString()} />
      <Stat
        label="Active readers"
        note="Last 30 days"
        value={stats.active_readers.toLocaleString()}
      />
      <Stat label="Devices" value={stats.devices.toLocaleString()} />
    </div>
  </Section>
);

const ALL_TIME = "all";

// The range in the URL: a four-digit year, or "all" (null) for all time.
// Undefined when it's absent or anything else, which shows the server's
// current year.
const parseRange = (value: string | null): number | null | undefined => {
  if (value === ALL_TIME) return null;
  if (!value || !/^\d{4}$/.test(value)) return undefined;
  const year = Number(value);
  return year >= 1000 ? year : undefined;
};

interface ReadingProps {
  range: number | null;
  currentYear: number;
  query: ReturnType<typeof useServerStatsReading>;
  onRangeChange: (value: string) => void;
}

const Reading = ({
  range,
  currentYear,
  query,
  onRangeChange,
}: ReadingProps) => {
  const reading = query.data;
  // The current year and the year in view are always offered, with or
  // without reading in them.
  const years = [
    ...new Set([
      currentYear,
      ...(range ? [range] : []),
      ...(reading?.years ?? []),
    ]),
  ].sort((a, b) => b - a);
  const renderBody = () => {
    if (query.error && !reading) {
      return <QueryError fallback="Failed to load reading" query={query} />;
    }
    if (!reading) return <LoadingSpinner />;
    // The previous range's figures stay while another loads, dimmed and
    // marked busy so they aren't read as the new range's.
    return (
      <div
        aria-busy={query.isPlaceholderData}
        className={cn(
          "grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4 transition-opacity",
          query.isPlaceholderData && "opacity-50",
        )}
      >
        <Stat
          label="Reading time"
          value={formatDuration(reading.reading_seconds)}
        />
        <Stat
          label="Listening time"
          value={formatDuration(reading.listening_seconds)}
        />
        <Stat
          label="Read-throughs finished"
          value={reading.read_throughs_finished.toLocaleString()}
        />
        <Stat
          label="Did Not Finish"
          value={reading.did_not_finish.toLocaleString()}
        />
        <Stat label="Readers" value={reading.readers.toLocaleString()} />
      </div>
    );
  };
  return (
    <Section
      action={
        <div className="flex items-center gap-2">
          {query.isPlaceholderData && (
            <Loader2
              aria-label="Loading reading"
              className="h-4 w-4 animate-spin text-muted-foreground"
              role="status"
            />
          )}
          <Select
            onValueChange={onRangeChange}
            value={range === null ? ALL_TIME : String(range)}
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
        </div>
      }
      description={`Totals across every user for ${range === null ? "all time" : range}.`}
      title="Reading"
    >
      {renderBody()}
    </Section>
  );
};

const LibraryRow = ({
  name,
  counts,
  isTotal = false,
}: {
  name: string;
  counts: ContentCounts;
  isTotal?: boolean;
}) => {
  const cell = "py-2 px-3 text-right tabular-nums whitespace-nowrap";
  return (
    <tr
      className={
        isTotal
          ? "border-t-2 border-border font-semibold"
          : "border-t border-border"
      }
    >
      <th className="py-2 pr-3 text-left font-medium" scope="row">
        {name}
      </th>
      <td className={cell}>{counts.books.toLocaleString()}</td>
      <td className="py-2 px-3 text-left min-w-48">
        <FilesBreakdown counts={counts} name={name} />
      </td>
      <td className={cell}>{counts.series.toLocaleString()}</td>
      <td className={cell}>{counts.persons.toLocaleString()}</td>
      <td className={cell}>{counts.genres.toLocaleString()}</td>
      <td className={cell}>{counts.tags.toLocaleString()}</td>
      <td className={cell}>{counts.publishers.toLocaleString()}</td>
      <td className={cell}>{formatFileSize(counts.size_bytes)}</td>
    </tr>
  );
};

// The server's totals get their own row when there's more than one row to
// sum, or when they include libraries the viewer can't see.
const showsAllLibraries = (stats: StatsResponse) =>
  stats.libraries.length > 1 ||
  stats.libraries.reduce((sum, lib) => sum + lib.books, 0) !==
    stats.totals.books ||
  stats.libraries.reduce((sum, lib) => sum + lib.size_bytes, 0) !==
    stats.totals.size_bytes;

// A library's counts as a card, for a phone, where the table's nine
// columns would leave most of them scrolled out of sight.
const LibraryCard = ({
  name,
  counts,
  isTotal = false,
}: {
  name: string;
  counts: ContentCounts;
  isTotal?: boolean;
}) => {
  const figures: [string, string][] = [
    ["Books", counts.books.toLocaleString()],
    ["Series", counts.series.toLocaleString()],
    ["People", counts.persons.toLocaleString()],
    ["Genres", counts.genres.toLocaleString()],
    ["Tags", counts.tags.toLocaleString()],
    ["Publishers", counts.publishers.toLocaleString()],
    ["Size", formatFileSize(counts.size_bytes)],
  ];
  return (
    <li
      aria-label={name}
      className={cn(
        "space-y-2 py-3",
        isTotal ? "border-t-2 border-border" : "border-t border-border",
      )}
    >
      <h3 className="break-words text-sm font-semibold">{name}</h3>
      <FilesBreakdown counts={counts} name={name} />
      <dl className="grid grid-cols-3 gap-x-4 gap-y-2">
        {figures.map(([label, value]) => (
          <div className="min-w-0" key={label}>
            <dt className="text-xs text-muted-foreground">{label}</dt>
            <dd
              className={cn(
                "text-sm tabular-nums break-words",
                isTotal && "font-semibold",
              )}
            >
              {value}
            </dd>
          </div>
        ))}
      </dl>
    </li>
  );
};

const Libraries = ({ stats }: { stats: StatsResponse }) => {
  const isPhone = useIsPhone();
  return (
    <Section
      description="Size is the stored size of every file, supplements included."
      title="Libraries"
    >
      {stats.libraries.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {stats.totals.books > 0 || stats.totals.size_bytes > 0
            ? "No libraries you can access."
            : "No libraries yet."}
        </p>
      ) : isPhone ? (
        <ul aria-label="Libraries">
          {stats.libraries.map((library) => (
            <LibraryCard
              counts={library}
              key={library.id}
              name={library.name}
            />
          ))}
          {showsAllLibraries(stats) && (
            <LibraryCard counts={stats.totals} isTotal name="All libraries" />
          )}
        </ul>
      ) : (
        // Between a phone and a wide screen, the table scrolls on its own,
        // not the page.
        <div className="overflow-x-auto -mx-4 px-4 md:mx-0 md:px-0">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-xs text-muted-foreground">
                <th className="py-2 pr-3 text-left font-medium" scope="col">
                  Library
                </th>
                <th className="py-2 px-3 text-right font-medium" scope="col">
                  Books
                </th>
                <th className="py-2 px-3 text-left font-medium" scope="col">
                  Files
                </th>
                <th className="py-2 px-3 text-right font-medium" scope="col">
                  Series
                </th>
                <th className="py-2 px-3 text-right font-medium" scope="col">
                  People
                </th>
                <th className="py-2 px-3 text-right font-medium" scope="col">
                  Genres
                </th>
                <th className="py-2 px-3 text-right font-medium" scope="col">
                  Tags
                </th>
                <th className="py-2 px-3 text-right font-medium" scope="col">
                  Publishers
                </th>
                <th className="py-2 px-3 text-right font-medium" scope="col">
                  Size
                </th>
              </tr>
            </thead>
            <tbody>
              {stats.libraries.map((library) => (
                <LibraryRow
                  counts={library}
                  key={library.id}
                  name={library.name}
                />
              ))}
              {showsAllLibraries(stats) && (
                <LibraryRow
                  counts={stats.totals}
                  isTotal
                  name="All libraries"
                />
              )}
            </tbody>
          </table>
        </div>
      )}
    </Section>
  );
};

const pathKindLabel: Record<VolumePathKind, string> = {
  library: "Library",
  data: "Database",
  cache: "Cache",
};

interface DiskSegment {
  label: string;
  bytes: number;
  /** Share of the volume's total the bar draws, 0 to 100. */
  percent: number;
  swatch: string;
}

// What a volume's used space is made of. Shisho's kinds come from stored
// file sizes and cache sizes, which can add up to more than the volume uses
// on a compressed filesystem; the bar then scales them to fit the used
// space, and the legend still states their sizes.
const diskSegments = (volume: Volume): DiskSegment[] => {
  const kinds = [
    {
      label: "Libraries",
      bytes: volume.library_bytes,
      swatch: "bg-disk-library",
    },
    {
      label: "Database",
      bytes: volume.database_bytes,
      swatch: "bg-disk-database",
    },
    { label: "Cache", bytes: volume.cache_bytes, swatch: "bg-disk-cache" },
  ].filter((k) => k.bytes > 0);
  const total = volume.total_bytes;
  const shisho = kinds.reduce((sum, k) => sum + k.bytes, 0);
  const scale = shisho > volume.used_bytes ? volume.used_bytes / shisho : 1;
  const percent = (bytes: number) => (total > 0 ? (bytes / total) * 100 : 0);
  const segments = kinds.map((k) => ({
    ...k,
    percent: percent(k.bytes * scale),
  }));
  const other = volume.used_bytes - Math.min(shisho, volume.used_bytes);
  if (other > 0) {
    segments.push({
      label: "Other",
      bytes: other,
      percent: percent(other),
      swatch: "bg-muted-foreground/40",
    });
  }
  return segments;
};

// "20%", or "<1%" for a sliver that would round to nothing.
const formatShare = (percent: number) =>
  percent > 0 && percent < 1 ? "<1%" : `${Math.round(percent)}%`;

const pathCell = "py-1.5 align-top";

// Each cache's size, then a link to manage them. Indented under the cache
// path's row, or on their own when that path's volume couldn't be read.
const CacheRows = ({
  caches,
  indent = false,
}: {
  caches: CacheSize[];
  indent?: boolean;
}) => (
  <>
    {caches.map((c) => (
      <tr
        className={cn(
          "text-muted-foreground",
          !indent && "border-t border-border",
        )}
        key={c.id}
      >
        <th
          className={cn(
            pathCell,
            "pr-3 text-left font-normal",
            indent && "pl-3",
          )}
          scope="row"
        >
          {c.name}
        </th>
        <td className={cn(pathCell, "pr-3")}>
          {formatCount(c.file_count, "file")}
        </td>
        <td className={cn(pathCell, "text-right tabular-nums")}>
          {formatFileSize(c.size_bytes)}
        </td>
      </tr>
    ))}
    <tr>
      <td className={cn(pathCell, indent && "pl-3")} colSpan={3}>
        <Link className="text-primary hover:underline" to="/admin/cache">
          Manage caches
        </Link>
      </td>
    </tr>
  </>
);

const VolumePaths = ({
  name,
  paths,
  caches,
}: {
  name: string;
  paths: VolumePath[];
  caches: CacheSize[];
}) => {
  const cell = pathCell;
  return (
    <table
      aria-label={`${name} paths`}
      className="mt-3 w-full table-fixed text-xs"
    >
      <thead>
        <tr className="text-muted-foreground">
          <th
            className={cn(cell, "w-1/3 pr-3 text-left font-medium sm:w-1/4")}
            scope="col"
          >
            Location
          </th>
          <th className={cn(cell, "pr-3 text-left font-medium")} scope="col">
            Path
          </th>
          <th className={cn(cell, "w-20 text-right font-medium")} scope="col">
            Size
          </th>
        </tr>
      </thead>
      <tbody>
        {paths.map((p) => (
          <Fragment key={`${p.kind}:${p.path}`}>
            <tr className="border-t border-border">
              <th
                className={cn(cell, "pr-3 text-left font-normal break-words")}
                scope="row"
              >
                {pathLocation(p)}
              </th>
              <td className={cn(cell, "pr-3 min-w-0")}>
                {/* Long paths keep their end, the folder's own name, and
                    lose the start; the full path is on hover. */}
                <TruncatedText
                  className="truncate font-mono [direction:rtl] text-left"
                  tooltip={p.path}
                >
                  <bdi>{p.path}</bdi>
                </TruncatedText>
              </td>
              <td className={cn(cell, "text-right tabular-nums")}>
                {formatFileSize(p.size_bytes)}
              </td>
            </tr>
            {p.kind === "cache" && <CacheRows caches={caches} indent />}
          </Fragment>
        ))}
      </tbody>
    </table>
  );
};

const pathLocation = (p: VolumePath) =>
  p.kind === "library" ? p.name : pathKindLabel[p.kind];

const VolumeItem = ({
  volume,
  index,
  caches,
}: {
  volume: Volume;
  index: number;
  caches: CacheSize[];
}) => {
  const headingId = useId();
  const name = `Volume ${index + 1}`;
  const segments = diskSegments(volume);
  return (
    <li aria-labelledby={headingId} className="py-3 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <h3 className="text-sm font-medium" id={headingId}>
          {name}
        </h3>
        <span className="text-sm text-muted-foreground">
          {formatFileSize(volume.free_bytes)} free
        </span>
      </div>
      {/* The legend below carries every figure, so the bar is decoration
          for screen readers; hovering a segment names it. A 2px gap in the
          surface color separates the segments; a tiny one still shows as a
          sliver. */}
      <div
        aria-hidden="true"
        className="mt-2 flex h-3 gap-0.5 overflow-hidden rounded-full"
      >
        {[
          ...segments,
          {
            label: "Free",
            bytes: volume.free_bytes,
            percent:
              volume.total_bytes > 0
                ? (volume.free_bytes / volume.total_bytes) * 100
                : 0,
            swatch: "flex-1 bg-muted",
          },
        ].map((seg) => (
          <Tooltip key={seg.label}>
            <TooltipTrigger asChild>
              <div
                className={cn("min-w-[3px] shrink-0", seg.swatch)}
                data-segment={seg.label}
                style={seg.label === "Free" ? {} : { width: `${seg.percent}%` }}
              />
            </TooltipTrigger>
            <TooltipContent
              arrowClassName="bg-popover fill-popover"
              className="border border-border bg-popover text-popover-foreground shadow-md"
              side="top"
            >
              <div className="font-semibold">{seg.label}</div>
              <div className="tabular-nums text-muted-foreground">
                {/* The share the bar draws, which a compressed volume's
                    stored sizes can exceed. */}
                {formatFileSize(seg.bytes)} · {formatShare(seg.percent)} of the
                volume
              </div>
            </TooltipContent>
          </Tooltip>
        ))}
      </div>
      <p className="mt-1 text-xs text-muted-foreground">
        {`${formatFileSize(volume.used_bytes)} used of ${formatFileSize(volume.total_bytes)}`}
      </p>
      <ul
        aria-label={`${name} space`}
        className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs"
      >
        {[
          ...segments,
          {
            label: "Free",
            bytes: volume.free_bytes,
            percent: 0,
            swatch: "bg-muted border border-border",
          },
        ].map((seg) => (
          <li className="flex items-center gap-1.5" key={seg.label}>
            <span
              aria-hidden="true"
              className={cn("size-2.5 shrink-0 rounded-sm", seg.swatch)}
            />
            <span className="text-muted-foreground">{seg.label}</span>
            <span className="tabular-nums">{formatFileSize(seg.bytes)}</span>
          </li>
        ))}
      </ul>
      {volume.paths.length === 0 && (
        <p className="mt-2 text-xs text-muted-foreground">
          Holds only library folders you can't see.
        </p>
      )}
      {volume.paths.length > 0 && (
        <VolumePaths caches={caches} name={name} paths={volume.paths} />
      )}
    </li>
  );
};

const DiskSpace = ({ stats }: { stats: StatsResponse }) => (
  <Section
    description="Free and used space on each volume holding a library, the data directory, or the cache. A volume shared by several paths is listed once."
    title="Disk Space"
  >
    {stats.volumes.length === 0 ? (
      <p className="text-sm text-muted-foreground">
        No volume space could be read.
      </p>
    ) : (
      <ul className="divide-y divide-border">
        {stats.volumes.map((volume, index) => (
          <VolumeItem
            caches={stats.caches}
            index={index}
            // Volumes come in a fixed order and never reorder in place,
            // and one with only hidden library folders has no paths.
            key={`volume-${index}`}
            volume={volume}
          />
        ))}
      </ul>
    )}
    {/* The caches are listed under the cache path's volume; when that
        volume couldn't be read, they're listed here instead. */}
    {stats.caches.length > 0 &&
      !stats.volumes.some((v) => v.paths.some((p) => p.kind === "cache")) && (
        <table aria-label="Caches" className="mt-4 w-full table-fixed text-xs">
          <thead>
            <tr className="text-muted-foreground">
              <th
                className={cn(
                  pathCell,
                  "w-1/3 pr-3 text-left font-medium sm:w-1/4",
                )}
                scope="col"
              >
                Cache
              </th>
              <th
                className={cn(pathCell, "pr-3 text-left font-medium")}
                scope="col"
              >
                Files
              </th>
              <th
                className={cn(pathCell, "w-20 text-right font-medium")}
                scope="col"
              >
                Size
              </th>
            </tr>
          </thead>
          <tbody>
            <CacheRows caches={stats.caches} />
          </tbody>
        </table>
      )}
  </Section>
);

const LibraryGrowth = ({ stats }: { stats: StatsResponse }) => (
  <Section
    description="Books in the library over time, by when they were added, in the server's time zone."
    title="Library Growth"
  >
    {stats.growth.length === 0 ? (
      <p className="text-sm text-muted-foreground">No books yet.</p>
    ) : (
      <LibraryGrowthChart growth={stats.growth} />
    )}
  </Section>
);

const AdminServerStats = () => {
  usePageTitle("Server Stats");

  const statsQuery = useServerStats();
  const { data: stats, isLoading } = statsQuery;

  // Reading covers the year in the URL, or the server's current year, which
  // the page learns from the stats; it waits for them unless the URL already
  // says.
  const [searchParams, setSearchParams] = useSearchParams();
  const parsed = parseRange(searchParams.get("year"));
  const range = parsed === undefined ? stats?.current_year : parsed;
  const readingQuery = useServerStatsReading(
    range == null ? {} : { year: range },
    { enabled: range !== undefined },
  );
  // A "year" in the URL that isn't one goes at once, so the URL always
  // says what the page shows.
  const strayYear = searchParams.get("year") !== null && parsed === undefined;
  useEffect(() => {
    if (!strayYear) return;
    setSearchParams(
      (prev) => {
        const params = new URLSearchParams(prev);
        params.delete("year");
        return params;
      },
      { replace: true, preventScrollReset: true },
    );
  }, [strayYear, setSearchParams]);
  const setRange = (value: string) => {
    const params = new URLSearchParams(searchParams);
    if (value === String(stats?.current_year)) params.delete("year");
    else params.set("year", value);
    setSearchParams(params, { preventScrollReset: true });
  };

  if (isLoading) {
    return <LoadingSpinner />;
  }

  const pageHeader = (
    <div className="mb-6 md:mb-8">
      <h1 className="text-2xl font-semibold mb-1 md:mb-2">Server Stats</h1>
      <p className="text-sm md:text-base text-muted-foreground">
        What the server holds, the disk it uses, and how much it is used.
        Figures are counted when this page opens.
      </p>
    </div>
  );

  if (!stats) {
    return (
      <div>
        {pageHeader}
        {statsQuery.error && (
          <QueryError
            fallback="Failed to load Server Stats"
            query={statsQuery}
          />
        )}
      </div>
    );
  }

  return (
    <div>
      {pageHeader}
      <div className="grid gap-6">
        <Overview stats={stats} />
        <Reading
          currentYear={stats.current_year}
          onRangeChange={setRange}
          query={readingQuery}
          range={range ?? null}
        />
        <Libraries stats={stats} />
        <LibraryGrowth stats={stats} />
        <DiskSpace stats={stats} />
      </div>
    </div>
  );
};

export default AdminServerStats;
