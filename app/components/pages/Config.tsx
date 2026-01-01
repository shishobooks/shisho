import { useEffect, useState } from "react";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useConfig, useUpdateConfig } from "@/hooks/queries/config";

const Config = () => {
  const { data: config, isLoading, error } = useConfig();
  const updateConfigMutation = useUpdateConfig();

  const [syncIntervalMinutes, setSyncIntervalMinutes] = useState<string>(
    config?.sync_interval_minutes?.toString() || "60",
  );

  // Update local state when config loads
  useEffect(() => {
    if (config?.sync_interval_minutes) {
      setSyncIntervalMinutes(config.sync_interval_minutes.toString());
    }
  }, [config?.sync_interval_minutes]);

  const handleSave = () => {
    const intervalValue = parseInt(syncIntervalMinutes, 10);
    if (isNaN(intervalValue) || intervalValue < 1) {
      toast.error("Sync interval must be at least 1 minute");
      return;
    }

    updateConfigMutation.mutate(
      { sync_interval_minutes: intervalValue },
      {
        onSuccess: () => {
          toast.success("Configuration saved successfully");
        },
        onError: (error) => {
          toast.error(`Failed to save configuration: ${error.message}`);
        },
      },
    );
  };

  const handleReset = () => {
    if (config) {
      setSyncIntervalMinutes(config.sync_interval_minutes.toString());
    }
  };

  const hasChanges =
    config &&
    parseInt(syncIntervalMinutes, 10) !== config.sync_interval_minutes;

  if (isLoading) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <LoadingSpinner />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full mx-auto px-6 py-8">
          <div className="text-center">
            <h1 className="text-2xl font-semibold mb-4">
              Error Loading Configuration
            </h1>
            <p className="text-muted-foreground">{error.message}</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        <div className="mb-8">
          <h1 className="text-2xl font-semibold mb-2">Configuration</h1>
          <p className="text-muted-foreground">
            Manage system-wide settings and preferences
          </p>
        </div>

        <div className="max-w-md border border-border rounded-md p-6 space-y-4">
          <div>
            <h2 className="text-lg font-semibold mb-4">Sync Settings</h2>
            <div>
              <Label htmlFor="sync-interval">Sync Interval (minutes)</Label>
              <Input
                id="sync-interval"
                min="1"
                onChange={(e) => setSyncIntervalMinutes(e.target.value)}
                placeholder="60"
                type="number"
                value={syncIntervalMinutes}
              />
              <p className="mt-1 text-xs text-gray-500">
                How often the system should check for new content (minimum 1
                minute)
              </p>
            </div>
          </div>

          <div className="flex gap-2 pt-2">
            <Button
              disabled={!hasChanges || updateConfigMutation.isPending}
              onClick={handleSave}
            >
              {updateConfigMutation.isPending ? "Saving..." : "Save"}
            </Button>
            <Button
              disabled={!hasChanges}
              onClick={handleReset}
              variant="outline"
            >
              Reset
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default Config;
