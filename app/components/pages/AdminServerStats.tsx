import { useId, type ReactNode } from "react";
import { Link } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { Progress } from "@/components/ui/progress";
import { useServerStats } from "@/hooks/queries/serverStats";
import { usePageTitle } from "@/hooks/usePageTitle";
import type {
  ContentCounts,
  GrowthMonth,
  ReadingPeriod,
  StatsResponse,
  Volume,
  VolumePathKind,
} from "@/types/generated/serverstats";
import {
  fileTypeLabel,
  formatCount,
  formatDuration,
  formatFileSize,
  formatMonthYear,
} from "@/utils/format";

interface SectionProps {
  title: string;
  description?: string;
  children: ReactNode;
}

const Section = ({ title, description, children }: SectionProps) => {
  const headingId = useId();
  return (
    <section
      aria-labelledby={headingId}
      className="border border-border rounded-md p-4 md:p-6"
    >
      <h2 className="text-base md:text-lg font-semibold mb-1" id={headingId}>
        {title}
      </h2>
      {description && (
        <p className="text-xs md:text-sm text-muted-foreground">
          {description}
        </p>
      )}
      <div className="mt-3 md:mt-4">{children}</div>
    </section>
  );
};

interface StatProps {
  label: string;
  value: string;
}

const Stat = ({ label, value }: StatProps) => (
  <div className="min-w-0">
    <dt className="text-xs md:text-sm text-muted-foreground">{label}</dt>
    <dd className="text-xl md:text-2xl font-semibold tabular-nums break-words">
      {value}
    </dd>
  </div>
);

const totalFiles = (counts: ContentCounts) =>
  counts.files.reduce((sum, f) => sum + f.count, 0) + counts.supplement_files;

// "EPUB 18,000 · M4B 400 · 7 supplements"
const filesSummary = (counts: ContentCounts): string => {
  const parts = counts.files.map(
    (f) => `${fileTypeLabel(f.file_type)} ${f.count.toLocaleString()}`,
  );
  if (counts.supplement_files > 0) {
    parts.push(formatCount(counts.supplement_files, "supplement"));
  }
  return parts.length > 0 ? parts.join(" · ") : "None";
};

const Overview = ({ stats }: { stats: StatsResponse }) => (
  <Section title="Overview">
    <dl className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4">
      <Stat label="Books" value={stats.totals.books.toLocaleString()} />
      <Stat label="Files" value={totalFiles(stats.totals).toLocaleString()} />
      <Stat
        label="Library size"
        value={formatFileSize(stats.totals.size_bytes)}
      />
      <Stat label="Users" value={stats.users.toLocaleString()} />
      <Stat label="Devices" value={stats.devices.toLocaleString()} />
    </dl>
  </Section>
);

const ReadingGroup = ({
  label,
  period,
}: {
  label: string;
  period: ReadingPeriod;
}) => {
  const labelId = useId();
  return (
    <div aria-labelledby={labelId} role="group">
      <h3 className="text-sm font-medium mb-2" id={labelId}>
        {label}
      </h3>
      <dl className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <Stat label="Reading" value={formatDuration(period.reading_seconds)} />
        <Stat
          label="Listening"
          value={formatDuration(period.listening_seconds)}
        />
        <Stat
          label="Read-throughs finished"
          value={period.read_throughs_finished.toLocaleString()}
        />
      </dl>
    </div>
  );
};

const Reading = ({ stats }: { stats: StatsResponse }) => (
  <Section
    description="Totals across every user. Reading is never broken down by user or by book here."
    title="Reading"
  >
    <div className="grid gap-6">
      <ReadingGroup
        label={String(stats.reading.year)}
        period={stats.reading.this_year}
      />
      <ReadingGroup label="All time" period={stats.reading.all_time} />
      <dl>
        <Stat
          label="Users who read in the last 30 days"
          value={stats.reading.active_users.toLocaleString()}
        />
      </dl>
    </div>
  </Section>
);

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
      <td className="py-2 px-3 text-left text-muted-foreground min-w-48">
        {filesSummary(counts)}
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

const Libraries = ({ stats }: { stats: StatsResponse }) => (
  <Section
    description="Size is the stored size of every file, supplements included."
    title="Libraries"
  >
    {stats.libraries.length === 0 ? (
      <p className="text-sm text-muted-foreground">No libraries yet.</p>
    ) : (
      // The table scrolls on its own on a phone, not the page.
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
            {stats.libraries.length > 1 && (
              <LibraryRow counts={stats.totals} isTotal name="All libraries" />
            )}
          </tbody>
        </table>
      </div>
    )}
  </Section>
);

const pathKindLabel: Record<VolumePathKind, string> = {
  library: "Library",
  data: "Data",
  cache: "Cache",
};

const VolumeItem = ({ volume, index }: { volume: Volume; index: number }) => {
  const headingId = useId();
  const percentUsed =
    volume.total_bytes > 0
      ? Math.round((volume.used_bytes / volume.total_bytes) * 100)
      : 0;
  return (
    <li aria-labelledby={headingId} className="py-3 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <h3 className="text-sm font-medium" id={headingId}>
          Volume {index + 1}
        </h3>
        <span className="text-sm text-muted-foreground">
          {formatFileSize(volume.free_bytes)} free
        </span>
      </div>
      <Progress
        aria-label={`Volume ${index + 1} space used`}
        className="mt-2"
        value={percentUsed}
      />
      <p className="mt-1 text-xs text-muted-foreground">
        {`${formatFileSize(volume.used_bytes)} used of ${formatFileSize(volume.total_bytes)}`}
      </p>
      {volume.paths.length > 0 && (
        <ul className="mt-2 space-y-1">
          {volume.paths.map((p) => (
            <li
              className="flex flex-wrap gap-x-2 text-xs min-w-0"
              key={`${p.kind}:${p.path}`}
            >
              <span className="text-muted-foreground">
                {p.kind === "library"
                  ? `${pathKindLabel[p.kind]}: ${p.name}`
                  : pathKindLabel[p.kind]}
              </span>
              <span className="font-mono break-all min-w-0">{p.path}</span>
            </li>
          ))}
        </ul>
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
            index={index}
            // Volumes come in a fixed order and never reorder in place,
            // and one with only hidden library folders has no paths.
            key={`volume-${index}`}
            volume={volume}
          />
        ))}
      </ul>
    )}
  </Section>
);

const DataRow = ({ label, value }: { label: string; value: ReactNode }) => (
  <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 py-2 border-b border-border last:border-b-0">
    <dt className="text-sm">{label}</dt>
    <dd className="text-sm text-muted-foreground tabular-nums">{value}</dd>
  </div>
);

const ServerData = ({ stats }: { stats: StatsResponse }) => (
  <Section
    description="The database counts its write-ahead log. Caches are regenerated as needed."
    title="Server Data"
  >
    <dl>
      <DataRow label="Database" value={formatFileSize(stats.database_bytes)} />
      {stats.caches.map((cache) => (
        <DataRow
          key={cache.id}
          label={cache.name}
          value={
            <>
              <span>{formatFileSize(cache.size_bytes)}</span>
              <span className="mx-2 text-muted-foreground/50">·</span>
              <span>{formatCount(cache.file_count, "file")}</span>
            </>
          }
        />
      ))}
    </dl>
    <Link
      className="mt-3 inline-block text-sm text-primary hover:underline"
      to="/admin/cache"
    >
      Manage caches
    </Link>
  </Section>
);

const GrowthChart = ({ growth }: { growth: GrowthMonth[] }) => {
  const max = Math.max(1, ...growth.map((m) => m.books));
  const total = growth.reduce((sum, m) => sum + m.books, 0);
  return (
    <>
      {/* The bars are for sighted users; the table below carries the same
          figures for screen readers. */}
      <div aria-hidden="true">
        <div className="flex items-end gap-px h-40 overflow-hidden">
          {growth.map((m) => (
            <div
              className="flex-1 min-w-px bg-primary/80 rounded-t-sm"
              key={m.month}
              style={{
                height:
                  m.books > 0 ? `${Math.max(2, (m.books / max) * 100)}%` : 0,
              }}
              title={`${formatMonthYear(m.month)}: ${formatCount(m.books, "book")}`}
            />
          ))}
        </div>
        <div className="mt-1 flex justify-between text-xs text-muted-foreground">
          <span>{formatMonthYear(growth[0].month)}</span>
          {growth.length > 1 && (
            <span>{formatMonthYear(growth[growth.length - 1].month)}</span>
          )}
        </div>
      </div>
      <p className="mt-2 text-xs text-muted-foreground">
        {formatCount(total, "book")} added over{" "}
        {formatCount(growth.length, "month")}.
      </p>
      <table className="sr-only">
        <caption>Books added by month</caption>
        <thead>
          <tr>
            <th scope="col">Month</th>
            <th scope="col">Books added</th>
          </tr>
        </thead>
        <tbody>
          {growth.map((m) => (
            <tr key={m.month}>
              <th scope="row">{formatMonthYear(m.month)}</th>
              <td>{m.books.toLocaleString()}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
};

const LibraryGrowth = ({ stats }: { stats: StatsResponse }) => (
  <Section
    description="Books by the month they were added, in the server's time zone."
    title="Library Growth"
  >
    {stats.growth.length === 0 ? (
      <p className="text-sm text-muted-foreground">No books yet.</p>
    ) : (
      <GrowthChart growth={stats.growth} />
    )}
  </Section>
);

const AdminServerStats = () => {
  usePageTitle("Server Stats");

  const statsQuery = useServerStats();
  const { data: stats, isLoading } = statsQuery;

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
        <Reading stats={stats} />
        <Libraries stats={stats} />
        <LibraryGrowth stats={stats} />
        <DiskSpace stats={stats} />
        <ServerData stats={stats} />
      </div>
    </div>
  );
};

export default AdminServerStats;
