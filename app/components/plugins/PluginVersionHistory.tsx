import { PluginVersionCard } from "./PluginVersionCard";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  useUpdatePluginVersion,
  type AvailablePlugin,
} from "@/hooks/queries/plugins";
import type { Plugin } from "@/types/generated/models";

const INITIAL_VISIBLE_OLDER = 3;

export interface PluginVersionHistoryProps {
  available?: AvailablePlugin;
  installed?: Plugin;
}

const buildGitHubDiffUrl = (
  homepage: string | undefined | null,
  version: string,
): string | undefined => {
  if (!homepage || !homepage.includes("github.com")) return undefined;
  return `${homepage.replace(/\/$/, "")}/releases/tag/v${version}`;
};

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
      return [[], versions];
    }
    const newer = installedIdx > 0 ? versions.slice(0, installedIdx) : [];
    const rest = versions.slice(installedIdx);
    return [newer, rest];
  }, [versions, installedVersion]);

  const handleUpdate = () => {
    if (!installed) return;
    const targetLabel = updateTarget ?? newerVersions[0]?.version ?? "latest";
    updateVersion.mutate(
      { id: installed.id, scope: installed.scope },
      {
        onError: (err) => {
          toast.error(err instanceof Error ? err.message : "Update failed");
        },
        onSuccess: () => {
          toast.success(`Updated to v${targetLabel}`);
        },
      },
    );
  };

  if (versions.length === 0) return null;

  const visibleOlder = expanded
    ? olderVersions
    : olderVersions.slice(0, INITIAL_VISIBLE_OLDER);
  const hiddenCount = olderVersions.length - visibleOlder.length;
  const homepage = installed?.homepage ?? available?.homepage;

  return (
    <section className="space-y-4">
      <h2 className="text-lg font-semibold">Version history</h2>

      {newerVersions.map((v, idx) => {
        const isUpdateTarget =
          installedVersion !== undefined &&
          (updateTarget ? v.version === updateTarget : idx === 0);
        return (
          <PluginVersionCard
            gitHubDiffUrl={buildGitHubDiffUrl(homepage, v.version)}
            isUpdating={updateVersion.isPending}
            key={v.version}
            onUpdate={isUpdateTarget ? handleUpdate : undefined}
            state={installedVersion ? "available" : "latest"}
            version={v}
          />
        );
      })}

      {visibleOlder.map((v) => (
        <PluginVersionCard
          gitHubDiffUrl={buildGitHubDiffUrl(homepage, v.version)}
          key={v.version}
          state={v.version === installedVersion ? "installed" : "older"}
          version={v}
        />
      ))}

      {hiddenCount > 0 && (
        <Button onClick={() => setExpanded(true)} size="sm" variant="ghost">
          Show {hiddenCount} older version{hiddenCount === 1 ? "" : "s"}
        </Button>
      )}
    </section>
  );
};
