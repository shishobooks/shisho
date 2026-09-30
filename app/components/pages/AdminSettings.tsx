import { useId } from "react";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { useConfig } from "@/hooks/queries/config";
import { usePageTitle } from "@/hooks/usePageTitle";
import {
  MinJWTSecretLength,
  MinLibraryMonitorDelaySeconds,
} from "@/types/generated/config";

const formatDuration = (nanoseconds: number): string => {
  const seconds = nanoseconds / 1_000_000_000;
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = seconds / 60;
  if (minutes < 60) {
    return `${minutes}m`;
  }
  const hours = minutes / 60;
  return `${hours}h`;
};

interface ConfigRowProps {
  description?: string;
  label: string;
  value: string | number | boolean;
}

/** Shows a setting where 0 turns the feature off. */
const orDisabled = (value: number, unit: string): string =>
  value === 0 ? "Disabled" : `${value} ${unit}`;

const ConfigRow = ({ description, label, value }: ConfigRowProps) => {
  const labelId = useId();
  const displayValue =
    typeof value === "boolean" ? (value ? "Yes" : "No") : String(value);

  return (
    <div
      aria-labelledby={labelId}
      className="flex flex-col sm:flex-row sm:justify-between sm:items-start gap-1 sm:gap-4 py-3 border-b border-border last:border-b-0"
      role="group"
    >
      <div className="flex flex-col gap-1 sm:shrink-0">
        <span
          className="text-sm font-medium text-foreground sm:whitespace-nowrap"
          id={labelId}
        >
          {label}
        </span>
        {description && (
          <p className="text-xs text-muted-foreground">{description}</p>
        )}
      </div>
      <span className="text-xs sm:text-sm text-muted-foreground font-mono break-words sm:text-right min-w-0">
        {displayValue}
      </span>
    </div>
  );
};

const AdminSettings = () => {
  usePageTitle("Server Settings");

  const configQuery = useConfig();
  const { data: config, isLoading } = configQuery;

  if (isLoading) {
    return <LoadingSpinner />;
  }

  const pageHeader = (
    <div className="mb-6 md:mb-8">
      <h1 className="text-2xl font-semibold mb-1 md:mb-2">Server Settings</h1>
      <p className="text-sm md:text-base text-muted-foreground">
        Current system configuration. Settings can be changed via the config
        file or environment variables.
      </p>
    </div>
  );

  if (!config) {
    return (
      <div>
        {pageHeader}
        {configQuery.error && (
          <QueryError
            fallback="Failed to load configuration"
            query={configQuery}
          />
        )}
      </div>
    );
  }

  return (
    <div>
      {pageHeader}

      <div className="grid gap-6">
        {/* Database Settings */}
        <div className="border border-border rounded-md p-4 md:p-6">
          <h2 className="text-base md:text-lg font-semibold mb-3 md:mb-4">
            Database
          </h2>
          <div className="space-y-0">
            <ConfigRow
              description="Path to the SQLite database file"
              label="Database Path"
              value={config.database_file_path}
            />
            <ConfigRow
              description="Logs every SQL statement, with its values, at debug level. This exposes stored values such as password hashes and share tokens in Settings > Logs, so enable it only briefly. Queries slower than 250 ms are always logged as warnings without their values"
              label="Debug Mode"
              value={config.database_debug}
            />
            <ConfigRow
              description="Total connection attempts on startup. 0 skips the check"
              label="Connection Retry Count"
              value={config.database_connect_retry_count}
            />
            <ConfigRow
              description="Delay between connection attempts. Minimum 1ms"
              label="Connection Retry Delay"
              value={formatDuration(config.database_connect_retry_delay)}
            />
            <ConfigRow
              description="How long to wait for a locked database before retrying. Minimum 1ms"
              label="Busy Timeout"
              value={formatDuration(config.database_busy_timeout)}
            />
            <ConfigRow
              description="Maximum retries for busy or locked database errors. 0 means one attempt with no retries"
              label="Max Retries"
              value={config.database_max_retries}
            />
          </div>
        </div>

        {/* Server Settings */}
        <div className="border border-border rounded-md p-4 md:p-6">
          <h2 className="text-base md:text-lg font-semibold mb-3 md:mb-4">
            Server
          </h2>
          <div className="space-y-0">
            <ConfigRow
              description="Address the server is bound to"
              label="Host"
              value={config.server_host}
            />
            <ConfigRow
              description="Port the server is listening on"
              label="Port"
              value={config.server_port}
            />
          </div>
        </div>

        {/* Application Settings */}
        <div className="border border-border rounded-md p-4 md:p-6">
          <h2 className="text-base md:text-lg font-semibold mb-3 md:mb-4">
            Application
          </h2>
          <div className="space-y-0">
            <ConfigRow
              description="Read-only API with background work and integrations disabled"
              label="Demo Mode"
              value={config.demo_mode}
            />
            <ConfigRow
              description="How often libraries are scanned for new content. The first scan runs one interval after startup"
              label="Sync Interval"
              value={orDisabled(config.sync_interval_minutes, "minutes")}
            />
            <ConfigRow
              description="Number of background workers that run jobs at the same time. Minimum 1"
              label="Worker Processes"
              value={config.worker_processes}
            />
            <ConfigRow
              description="Days to keep completed and failed jobs, with their logs, before the hourly cleanup deletes them"
              label="Job Retention"
              value={orDisabled(config.job_retention_days, "days")}
            />
            <ConfigRow
              description="Real-time filesystem monitoring of library paths"
              label="Library Monitor"
              value={config.library_monitor_enabled}
            />
            <ConfigRow
              description={
                config.library_monitor_effective_delay_seconds ===
                config.library_monitor_delay_seconds
                  ? `Seconds to wait before processing detected changes. Minimum ${MinLibraryMonitorDelaySeconds}s`
                  : `Seconds to wait before processing detected changes. Configured ${config.library_monitor_delay_seconds}s, raised to the ${MinLibraryMonitorDelaySeconds}s minimum`
              }
              label="Monitor Delay"
              value={`${config.library_monitor_effective_delay_seconds}s`}
            />
            <ConfigRow
              description="Test-only API routes for the end-to-end test suite"
              label="Test Mode"
              value={config.test_mode}
            />
          </div>
        </div>

        {/* Storage Settings */}
        <div className="border border-border rounded-md p-4 md:p-6">
          <h2 className="text-base md:text-lg font-semibold mb-3 md:mb-4">
            Storage
          </h2>
          <div className="space-y-0">
            <ConfigRow
              description="Directory for cached downloads and generated files"
              label="Cache Directory"
              value={config.cache_dir}
            />
            <ConfigRow
              description="Maximum disk space for generated downloads. When exceeded, the least recently used are removed until the cache is at 80% of this size"
              label="Download Cache Max Size"
              value={`${config.download_cache_max_size_gb} GiB`}
            />
            <ConfigRow
              description="File patterns excluded from supplement discovery. They never cause files to be deleted"
              label="Supplement Exclude Patterns"
              value={config.supplement_exclude_patterns.join(", ")}
            />
          </div>
        </div>

        {/* PDF Settings */}
        <div className="border border-border rounded-md p-4 md:p-6">
          <h2 className="text-base md:text-lg font-semibold mb-3 md:mb-4">
            PDF
          </h2>
          <div className="space-y-0">
            <ConfigRow
              description="Resolution for rendering PDF pages in the viewer. Pages are rendered again after a change"
              label="PDF Render DPI"
              value={`${config.pdf_render_dpi} DPI`}
            />
            <ConfigRow
              description="JPEG quality for rendered PDF pages. Pages are rendered again after a change"
              label="PDF Render Quality"
              value={`${config.pdf_render_quality}`}
            />
            <ConfigRow
              description="PDF basenames auto-classified as supplements when a sibling main file exists in the same directory"
              label="PDF Supplement Filenames"
              value={config.pdf_supplement_filenames.join(", ")}
            />
          </div>
        </div>

        {/* Plugin Settings */}
        <div className="border border-border rounded-md p-4 md:p-6">
          <h2 className="text-base md:text-lg font-semibold mb-3 md:mb-4">
            Plugins
          </h2>
          <div className="space-y-0">
            <ConfigRow
              description="Directory where installed plugins are stored"
              label="Plugin Directory"
              value={config.plugin_dir}
            />
            <ConfigRow
              description="Directory where plugin persistent data is stored"
              label="Plugin Data Directory"
              value={config.plugin_data_dir}
            />
            <ConfigRow
              description="Confidence threshold for automatic metadata enrichment during scans. Results below this score are skipped. Per-plugin thresholds override this value."
              label="Enrichment Confidence Threshold"
              value={`${Math.round(config.enrichment_confidence_threshold * 100)}%`}
            />
          </div>
        </div>

        {/* Authentication Settings */}
        <div className="border border-border rounded-md p-4 md:p-6">
          <h2 className="text-base md:text-lg font-semibold mb-3 md:mb-4">
            Authentication
          </h2>
          <div className="space-y-0">
            <ConfigRow
              description="How long login sessions remain valid"
              label="Session Duration"
              value={`${config.session_duration_days} days`}
            />
            {config.jwt_secret_too_short && (
              <ConfigRow
                description={`Shorter than ${MinJWTSecretLength} characters, so login sessions are easier to forge. Replace it with the output of openssl rand -hex 32 and restart; everyone is signed out once`}
                label="JWT Secret"
                value="Too short"
              />
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

export default AdminSettings;
