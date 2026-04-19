import { ChevronLeft } from "lucide-react";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import { PluginConfigForm } from "@/components/plugins/PluginConfigForm";
import { PluginDetailHero } from "@/components/plugins/PluginDetailHero";
import { PluginPermissions } from "@/components/plugins/PluginPermissions";
import { PluginVersionHistory } from "@/components/plugins/PluginVersionHistory";
import { Button } from "@/components/ui/button";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";
import {
  usePluginsAvailable,
  usePluginsInstalled,
} from "@/hooks/queries/plugins";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";

export const PluginDetail = () => {
  const { scope, id } = useParams<{ scope: string; id: string }>();
  const navigate = useNavigate();
  const installedQuery = usePluginsInstalled();
  const availableQuery = usePluginsAvailable();

  const [configDirty, setConfigDirty] = useState(false);
  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(configDirty);

  const installed = installedQuery.data?.find(
    (p) => p.scope === scope && p.id === id,
  );
  const available = availableQuery.data?.find(
    (p) => p.scope === scope && p.id === id,
  );

  const displayName = installed?.name ?? available?.name ?? id;
  usePageTitle(displayName);

  const isLoading = installedQuery.isLoading || availableQuery.isLoading;
  const hasError = installedQuery.isError || availableQuery.isError;
  const notFound = !isLoading && !hasError && !installed && !available;

  return (
    <div className="flex flex-col gap-6 p-6">
      <div>
        <Button
          onClick={() => navigate("/settings/plugins")}
          size="sm"
          variant="ghost"
        >
          <ChevronLeft className="mr-1 h-4 w-4" />
          Plugins
        </Button>
      </div>

      {isLoading && <PluginDetailSkeleton />}

      {!isLoading && hasError && (
        <div className="rounded-md border border-destructive/40 p-8 text-center text-destructive">
          <p className="text-lg">Failed to load plugin</p>
          <p className="mt-1 text-sm text-muted-foreground">
            {installedQuery.error instanceof Error
              ? installedQuery.error.message
              : availableQuery.error instanceof Error
                ? availableQuery.error.message
                : "An unexpected error occurred."}
          </p>
        </div>
      )}

      {notFound && (
        <div className="rounded-md border border-border p-8 text-center text-muted-foreground">
          <p className="text-lg">Plugin not found</p>
          <p className="mt-1 text-sm">
            No installed or available plugin matches{" "}
            <code>
              {scope}/{id}
            </code>
            .
          </p>
        </div>
      )}

      {!isLoading && !hasError && !notFound && scope && id && (
        <PluginDetailHero
          available={available}
          id={id}
          installed={installed}
          scope={scope}
        />
      )}

      {!isLoading && !hasError && !notFound && (
        <PluginVersionHistory available={available} installed={installed} />
      )}

      {!isLoading && !hasError && !notFound && (
        <PluginPermissions available={available} installed={installed} />
      )}

      {installed && scope && id && (
        <section className="space-y-3 rounded-md border border-border p-6">
          <PluginConfigForm
            id={id}
            onDirtyChange={setConfigDirty}
            scope={scope}
          />
        </section>
      )}

      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />
    </div>
  );
};

const PluginDetailSkeleton = () => (
  <div className="rounded-md border border-border p-6">
    <div className="flex gap-4">
      <div className="h-16 w-16 animate-pulse rounded-xl bg-muted" />
      <div className="flex-1 space-y-2">
        <div className="h-5 w-1/3 animate-pulse rounded bg-muted" />
        <div className="h-4 w-1/2 animate-pulse rounded bg-muted" />
        <div className="h-4 w-3/4 animate-pulse rounded bg-muted" />
      </div>
    </div>
  </div>
);
