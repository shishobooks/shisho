import { Info } from "lucide-react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Switch } from "@/components/ui/switch";
import {
  useSharingSettings,
  useUpdateSharingSettings,
} from "@/hooks/queries/sharing";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";
import { toastRequestError } from "@/libraries/api";
import type { UpdateSharingSettingsPayload } from "@/types";

interface SettingRowProps {
  id: string;
  label: string;
  description: string;
  checked: boolean;
  canEdit: boolean;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
}

const SettingRow = ({
  id,
  label,
  description,
  checked,
  canEdit,
  disabled,
  onCheckedChange,
}: SettingRowProps) => (
  <div className="flex items-start justify-between gap-4">
    <div>
      <label
        className="text-sm font-medium"
        htmlFor={canEdit ? id : undefined}
        id={`${id}-label`}
      >
        {label}
      </label>
      <p className="text-xs text-muted-foreground mt-0.5">{description}</p>
    </div>
    {canEdit ? (
      <Switch
        checked={checked}
        className="mt-0.5"
        disabled={disabled}
        id={id}
        onCheckedChange={onCheckedChange}
      />
    ) : (
      <span
        aria-labelledby={`${id}-label`}
        className="shrink-0 text-sm text-muted-foreground"
      >
        {checked ? "On" : "Off"}
      </span>
    )}
  </div>
);

const AdminSharing = () => {
  usePageTitle("Sharing");

  const { hasPermission } = useAuth();
  const canEdit = hasPermission("config", "write");
  const canViewUsers = hasPermission("users", "read");
  const settingsQuery = useSharingSettings();
  const updateMutation = useUpdateSharingSettings();

  // A switch saves at once with no button to press, so the toast is the only
  // confirmation. It names the state the switch is now in.
  const save = (payload: UpdateSharingSettingsPayload, saved: string) => {
    updateMutation.mutate(payload, {
      onSuccess: () => toast.success(saved),
      onError: (error) => toastRequestError(error, error.message),
    });
  };

  const pageHeader = (
    <div className="mb-6 md:mb-8">
      <h1 className="text-2xl font-semibold mb-1 md:mb-2">Sharing</h1>
      <p className="text-sm md:text-base text-muted-foreground">
        Control whether users can send a book to someone without a Shisho
        account through a Share Link.
      </p>
    </div>
  );

  if (settingsQuery.isLoading) {
    return (
      <div>
        {pageHeader}
        <LoadingSpinner />
      </div>
    );
  }

  if (settingsQuery.isError || !settingsQuery.data) {
    return (
      <div>
        {pageHeader}
        <p className="text-sm text-muted-foreground">
          Failed to load sharing settings.
        </p>
      </div>
    );
  }

  const settings = settingsQuery.data;

  return (
    <div>
      {pageHeader}
      <div className="grid gap-6">
        <div className="border border-border rounded-md p-4 md:p-6 space-y-6">
          <div className="space-y-3">
            <SettingRow
              canEdit={canEdit}
              checked={settings.enabled}
              description="Let users with the Shares permission create links that open a book without signing in. Turning this off stops every existing link from working without deleting it, and turning it back on restores them."
              disabled={updateMutation.isPending}
              id="sharing-enabled"
              label="Enable Share Links"
              onCheckedChange={(checked) =>
                save(
                  { enabled: checked },
                  checked ? "Share Links turned on" : "Share Links turned off",
                )
              }
            />
            <div className="flex gap-2 rounded-md border border-border bg-muted/50 p-3 text-xs text-muted-foreground">
              <Info className="h-4 w-4 shrink-0" />
              <p>
                This server must be reachable by the people you share links
                with. If Shisho is only available on your local network or
                through a VPN, recipients outside it cannot open the links.
              </p>
            </div>
          </div>

          <SettingRow
            canEdit={canEdit}
            checked={settings.require_expiration}
            description="Every Share Link must have an expiration date. When this is off, users can also create links that never expire."
            disabled={updateMutation.isPending}
            id="sharing-require-expiration"
            label="Require expiration"
            onCheckedChange={(checked) =>
              save(
                { require_expiration: checked },
                checked
                  ? "New links must now expire"
                  : "New links may now last forever",
              )
            }
          />
        </div>

        <div className="border border-border rounded-md p-4 md:p-6">
          <h2 className="text-base md:text-lg font-semibold mb-3 md:mb-4">
            Permissions
          </h2>
          <p className="text-sm text-muted-foreground">
            Creating and managing Share Links requires the{" "}
            <strong className="font-medium text-foreground">Shares</strong>{" "}
            permission, which only the Admin role has by default. To let other
            roles share books, grant it to them in{" "}
            {canViewUsers ? (
              <Link
                className="text-primary hover:underline"
                to="/settings/users"
              >
                Users
              </Link>
            ) : (
              "Users"
            )}
            .
          </p>
        </div>
      </div>
    </div>
  );
};

export default AdminSharing;
