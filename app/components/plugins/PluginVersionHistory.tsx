import { useMemo, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  useUpdatePluginVersion,
  type AvailablePlugin,
} from "@/hooks/queries/plugins";
import type { Plugin } from "@/types/generated/models";

import { PluginVersionCard } from "./PluginVersionCard";

const INITIAL_VISIBLE_OLDER = 3;

export interface PluginVersionHistoryProps {
  available?: AvailablePlugin;
  installed?: Plugin;
}

export const PluginVersionHistory = ({
  available,
  installed,
}: PluginVersionHistoryProps) => {
  const versions = useMemo(
    () => available?.versions ?? [],
    [available?.versions],
  );
  const installedVersion = installed?.version;
  const updateTarget = installed?.update_available_version;
  const updateVersion = useUpdatePluginVersion();
  const [expanded, setExpanded] = useState(false);

  const [newerVersions, olderVersions] = useMemo<
    [typeof versions, typeof versions]
  >(() => {
    if (!installedVersion) {
      const compatible = versions.filter((v) => v.compatible !== false);
      return [compatible.slice(0, 1), compatible.slice(1)];
    }
    const installedIdx = versions.findIndex(
      (v) => v.version === installedVersion,
    );
    if (installedIdx === -1) {
      const compatible = versions.filter((v) => v.compatible !== false);
      return [compatible.slice(0, 1), compatible.slice(1)];
    }
    const newer = installedIdx > 0 ? versions.slice(0, installedIdx) : [];
    const rest = versions.slice(installedIdx);
    return [newer, rest];
  }, [versions, installedVersion]);

  const handleUpdate = () => {
    if (!installed) return;
    const targetLabel = updateTarget ?? newerVersions[0]?.version;
    updateVersion.mutate(
      { id: installed.id, scope: installed.scope },
      {
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : "Update failed");
        },
        onSuccess: () => {
          toast.success(
            targetLabel ? `Updated to v${targetLabel}` : "Plugin updated",
          );
        },
      },
    );
  };

  if (versions.length === 0) return null;

  const visibleOlder = expanded
    ? olderVersions
    : olderVersions.slice(0, INITIAL_VISIBLE_OLDER);
  const hiddenCount = olderVersions.length - visibleOlder.length;

  return (
    <section className="rounded-md border border-border p-4 md:p-6">
      <h2 className="mb-3 text-lg font-semibold md:mb-4">Version history</h2>

      <div className="divide-y divide-border">
        {newerVersions.map((v, idx) => {
          const isUpdateTarget =
            installedVersion !== undefined &&
            (updateTarget ? v.version === updateTarget : idx === 0);
          return (
            <PluginVersionCard
              isUpdating={updateVersion.isPending}
              key={v.version}
              onUpdate={isUpdateTarget ? handleUpdate : undefined}
              releaseUrl={v.releaseUrl}
              state={installedVersion ? "available" : "latest"}
              version={v}
            />
          );
        })}

        {visibleOlder.map((v) => (
          <PluginVersionCard
            key={v.version}
            releaseUrl={v.releaseUrl}
            state={v.version === installedVersion ? "installed" : "older"}
            version={v}
          />
        ))}
      </div>

      {hiddenCount > 0 && (
        <Button
          className="mt-2"
          onClick={() => setExpanded(true)}
          size="sm"
          variant="ghost"
        >
          Show {hiddenCount} older version{hiddenCount === 1 ? "" : "s"}
        </Button>
      )}
    </section>
  );
};
