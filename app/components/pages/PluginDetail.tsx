import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { CapabilitiesWarning } from "@/components/plugins/CapabilitiesWarning";
import { PluginCapabilitiesSection } from "@/components/plugins/PluginCapabilitiesSection";
import { PluginConfigForm } from "@/components/plugins/PluginConfigForm";
import { PluginDangerZone } from "@/components/plugins/PluginDangerZone";
import { PluginDetailHero } from "@/components/plugins/PluginDetailHero";
import { PluginHookOrderSection } from "@/components/plugins/PluginHookOrderSection";
import { PluginVersionHistory } from "@/components/plugins/PluginVersionHistory";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";
import {
  useInstallPlugin,
  usePluginRepositories,
  usePluginsAvailable,
  usePluginsInstalled,
  useUpdatePlugin,
  useUpdatePluginVersion,
} from "@/hooks/queries/plugins";
import { useCan } from "@/hooks/useCan";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import { requestErrorMessage, toastRequestError } from "@/libraries/api";

export const PluginDetail = () => {
  const { scope, id } = useParams<{ scope: string; id: string }>();
  const canWrite = useCan("config:write");
  const installedQuery = usePluginsInstalled();
  const availableQuery = usePluginsAvailable();
  const { data: repos = [] } = usePluginRepositories();
  const updatePlugin = useUpdatePlugin();
  const updateVersion = useUpdatePluginVersion();
  const installPlugin = useInstallPlugin();

  const [configDirty, setConfigDirty] = useState(false);
  const [installDialogOpen, setInstallDialogOpen] = useState(false);
  const { cancelNavigation, proceedNavigation, showBlockerDialog } =
    useUnsavedChanges(configDirty);

  const installed = installedQuery.data?.find(
    (p) => p.scope === scope && p.id === id,
  );
  const available = availableQuery.data?.find(
    (p) => p.scope === scope && p.id === id,
  );

  const displayName = installed?.name ?? available?.name ?? id;
  const repoScope = installed?.scope ?? available?.scope ?? scope;
  const repoMatch = repos.find((r) => r.scope === repoScope);
  const repoName = repoMatch?.name || undefined;
  usePageTitle(displayName);

  const isLoading = installedQuery.isLoading || availableQuery.isLoading;
  const hasError = installedQuery.isError || availableQuery.isError;
  const notFound = !isLoading && !hasError && !installed && !available;

  const handleToggleEnabled = async (enabled: boolean) => {
    if (!installed) return;
    try {
      await updatePlugin.mutateAsync({
        id: installed.id,
        payload: { enabled },
        scope: installed.scope,
      });
      toast.success(
        enabled ? `${installed.name} enabled` : `${installed.name} disabled`,
      );
    } catch (err) {
      toastRequestError(err, "Failed to update plugin status");
    }
  };

  const handleInstallConfirm = () => {
    if (!available) return;
    const compatibleVersion = available.versions.find((v) => v.compatible);
    if (!compatibleVersion) return;
    installPlugin.mutate(
      {
        id: available.id,
        name: available.name,
        scope: available.scope,
        version: compatibleVersion.version,
      },
      {
        onError: (err) => {
          toastRequestError(err, "Failed to install plugin");
        },
        onSuccess: () => {
          setInstallDialogOpen(false);
          toast.success(`${available.name} installed`);
        },
      },
    );
  };

  const handleUpdate = async () => {
    if (!installed) return;
    const targetLabel = installed.update_available_version;
    try {
      await updateVersion.mutateAsync({
        id: installed.id,
        scope: installed.scope,
      });
      toast.success(
        targetLabel ? `Updated to v${targetLabel}` : "Plugin updated",
      );
    } catch (err) {
      toastRequestError(err, "Failed to update plugin");
    }
  };

  return (
    <div className="flex flex-col gap-6 p-6">
      <nav className="text-xs sm:text-sm text-muted-foreground overflow-hidden">
        <ol className="flex items-center gap-1 sm:gap-2 flex-wrap">
          <li className="shrink-0">
            <Link
              className="hover:text-foreground hover:underline"
              to="/settings/plugins"
            >
              Plugins
            </Link>
          </li>
          <li aria-hidden="true" className="shrink-0">
            ›
          </li>
          <li className="text-foreground truncate">{displayName}</li>
        </ol>
      </nav>

      {isLoading && <LoadingSpinner />}

      {!isLoading && hasError && (
        <div className="rounded-md border border-destructive/40 p-8 text-center text-destructive">
          <p className="text-lg">Failed to load plugin</p>
          <p className="mt-1 text-sm text-muted-foreground">
            {requestErrorMessage(
              installedQuery.error ?? availableQuery.error,
              "An unexpected error occurred.",
            )}
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
          canWrite={canWrite}
          id={id}
          installed={installed}
          isInstalling={installPlugin.isPending}
          isTogglingEnabled={updatePlugin.isPending}
          isUpdating={updateVersion.isPending}
          onInstall={() => setInstallDialogOpen(true)}
          onToggleEnabled={handleToggleEnabled}
          onUpdate={handleUpdate}
          repoName={repoName}
          scope={scope}
        />
      )}

      {!isLoading && !hasError && !notFound && (
        <PluginVersionHistory available={available} installed={installed} />
      )}

      {!isLoading && !hasError && !notFound && (
        <PluginCapabilitiesSection
          available={available}
          installed={installed}
        />
      )}

      {installed && <PluginHookOrderSection installed={installed} />}

      {installed && scope && id && (
        <PluginConfigForm
          canWrite={canWrite}
          id={id}
          onDirtyChange={setConfigDirty}
          scope={scope}
        />
      )}

      {installed && <PluginDangerZone canWrite={canWrite} plugin={installed} />}

      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />

      <CapabilitiesWarning
        isPending={installPlugin.isPending}
        onConfirm={handleInstallConfirm}
        onOpenChange={setInstallDialogOpen}
        open={installDialogOpen}
        plugin={available ?? null}
      />
    </div>
  );
};
