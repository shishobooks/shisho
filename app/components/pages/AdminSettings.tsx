import LoadingSpinner from "@/components/library/LoadingSpinner";
import { useConfig } from "@/hooks/queries/config";

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
    <div className="flex flex-col py-3 border-b border-border last:border-b-0">
      <div className="flex justify-between items-center">
        <span className="text-sm font-medium text-foreground">{label}</span>
        <span className="text-sm text-muted-foreground font-mono">
          {displayValue}
        </span>
      </div>
      {description && (
        <p className="text-xs text-muted-foreground mt-1">{description}</p>
      )}
    </div>
  );
};

const AdminSettings = () => {
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
      <div className="mb-8">
        <h1 className="text-2xl font-semibold mb-2">Server Settings</h1>
        <p className="text-muted-foreground">
          Current system configuration. Settings can be changed via the config
          file or environment variables.
        </p>
      </div>

      <div className="grid gap-6 max-w-2xl">
        {/* Database Settings */}
        <div className="border border-border rounded-md p-6">
          <h2 className="text-lg font-semibold mb-4">Database</h2>
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
        <div className="border border-border rounded-md p-6">
          <h2 className="text-lg font-semibold mb-4">Server</h2>
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
        <div className="border border-border rounded-md p-6">
          <h2 className="text-lg font-semibold mb-4">Application</h2>
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
              description="Application environment mode"
              label="Environment"
              value={config.environment || "production"}
            />
          </div>
        </div>

        {/* Storage Settings */}
        <div className="border border-border rounded-md p-6">
          <h2 className="text-lg font-semibold mb-4">Storage</h2>
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
              description="Directory where installed plugins are stored"
              label="Plugin Directory"
              value={config.plugin_dir}
            />
            <ConfigRow
              description="File patterns excluded from supplement discovery"
              label="Supplement Exclude Patterns"
              value={config.supplement_exclude_patterns.join(", ")}
            />
          </div>
        </div>
      </div>
    </div>
  );
};

export default AdminSettings;
