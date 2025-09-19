import { useEffect, useState } from "react";
import { toast } from "sonner";

import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
        <div className="max-w-7xl w-full p-8 m-auto">
          <div className="text-center">Loading configuration...</div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div>
        <TopNav />
        <div className="max-w-7xl w-full p-8 m-auto">
          <div className="text-center text-red-600">
            Error loading configuration: {error.message}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full p-8 m-auto">
        <h1 className="mb-8 text-3xl font-bold">Configuration</h1>

        <Card className="max-w-md">
          <CardHeader>
            <CardTitle>Sync Settings</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
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

            <div className="flex gap-2">
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
          </CardContent>
        </Card>
      </div>
    </div>
  );
};

export default Config;
