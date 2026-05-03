import LoadingSpinner from "@/components/library/LoadingSpinner";
import { useConfig } from "@/hooks/queries/config";
import { usePageTitle } from "@/hooks/usePageTitle";

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

const ConfigRow = ({ description, label, value }: ConfigRowProps) => {
  const displayValue =
    typeof value === "boolean" ? (value ? "Yes" : "No") : String(value);

  return (
    <div className="flex flex-col sm:flex-row sm:justify-between sm:items-start gap-1 sm:gap-4 py-3 border-b border-border last:border-b-0">
      <div className="flex flex-col gap-1 sm:shrink-0">
        <span className="text-sm font-medium text-foreground sm:whitespace-nowrap">
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

  const { data: config, isLoading, error } = useConfig();

  if (isLoading) {
    return <LoadingSpinner />;
  }

  if (error) {
    return (
      <div className="text-center">
        <h1 className="text-2xl font-semibold mb-4">
          Error Loading Configuration
        </h1>
        <p className="text-muted-foreground">{error.message}</p>
      </div>
    );
  }

  if (!config) {
    return null;
  }

  return (
    <div>
      <div className="mb-6 md:mb-8">
        <h1 className="text-2xl font-semibold mb-1 md:mb-2">Server Settings</h1>
        <p className="text-sm md:text-base text-muted-foreground">
          Current system configuration. Settings can be changed via the config
          file or environment variables.
        </p>
      </div>

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
              description="Whether SQL query logging is enabled"
              label="Debug Mode"
              value={config.database_debug}
            />
            <ConfigRow
              description="Number of connection retry attempts on startup"
              label="Connection Retry Count"
              value={config.database_connect_retry_count}
            />
            <ConfigRow
              description="Delay between connection retry attempts"
              label="Connection Retry Delay"
              value={formatDuration(config.database_connect_retry_delay)}
            />
            <ConfigRow
              description="How long to wait for a locked database before retrying"
              label="Busy Timeout"
              value={formatDuration(config.database_busy_timeout)}
            />
            <ConfigRow
              description="Maximum retries for transient database errors"
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
              description="How often libraries are scanned for new content"
              label="Sync Interval"
              value={`${config.sync_interval_minutes} minutes`}
            />
            <ConfigRow
              description="Number of background worker processes"
              label="Worker Processes"
              value={config.worker_processes}
            />
            <ConfigRow
              description="Number of days to retain completed job logs"
              label="Job Retention"
              value={`${config.job_retention_days} days`}
            />
            <ConfigRow
              description="Real-time filesystem monitoring of library paths"
              label="Library Monitor"
              value={config.library_monitor_enabled}
            />
            <ConfigRow
              description="Seconds to wait before processing detected changes"
              label="Monitor Delay"
              value={`${config.library_monitor_delay_seconds}s`}
            />
            <ConfigRow
              description="Application environment mode"
              label="Environment"
              value={config.environment || "production"}
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
              description="Maximum disk space for the download cache"
              label="Download Cache Max Size"
              value={`${config.download_cache_max_size_gb} GB`}
            />
            <ConfigRow
              description="File patterns excluded from supplement discovery"
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
              description="Resolution for rendering PDF pages in the viewer"
              label="PDF Render DPI"
              value={`${config.pdf_render_dpi} DPI`}
            />
            <ConfigRow
              description="JPEG quality for rendered PDF pages"
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
          </div>
        </div>
      </div>
    </div>
  );
};

export default AdminSettings;
