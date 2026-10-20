import { useState } from "react";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { useAllDevices, useRevokeDevice } from "@/hooks/queries/devices";
import { useCan } from "@/hooks/useCan";
import { usePageTitle } from "@/hooks/usePageTitle";
import { isDemoModeError, toastRequestError } from "@/libraries/api";
import type { AdminDeviceResponse } from "@/types/generated/devices";
import { CONNECTION_LABELS, DEVICE_TYPE_LABELS } from "@/utils/devices";
import { formatDateTime } from "@/utils/format";

const AdminDevices = () => {
  usePageTitle("Devices");

  const devicesQuery = useAllDevices();
  const { data: devices, isLoading } = devicesQuery;
  const canRevoke = useCan("users:write");
  const revokeDevice = useRevokeDevice();
  const [revoking, setRevoking] = useState<AdminDeviceResponse | null>(null);

  const handleRevoke = async () => {
    if (!revoking) return;
    try {
      await revokeDevice.mutateAsync(revoking.id);
      toast.success("Device revoked");
      setRevoking(null);
    } catch (error) {
      if (isDemoModeError(error)) setRevoking(null);
      toastRequestError(error, "Failed to revoke device");
    }
  };

  return (
    <div>
      <div className="mb-6 md:mb-8">
        <h1 className="mb-1 text-2xl font-semibold md:mb-2">Devices</h1>
        <p className="text-sm text-muted-foreground md:text-base">
          Every user's Devices.
          {canRevoke && " Revoke a lost or stolen one to cut it off."}
        </p>
      </div>

      {isLoading ? (
        <LoadingSpinner />
      ) : devicesQuery.error && !devices ? (
        <QueryError fallback="Failed to load devices" query={devicesQuery} />
      ) : !devices?.length ? (
        <div className="rounded-md border border-border p-8 text-center">
          <p className="text-muted-foreground">No user has added a device.</p>
        </div>
      ) : (
        <div className="divide-y divide-border rounded-md border border-border">
          {devices.map((device) => (
            <div
              className="flex items-center justify-between gap-3 px-4 py-3 md:px-6 md:py-4"
              key={device.id}
            >
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2 md:gap-3">
                  <span
                    className="truncate text-sm font-medium text-foreground md:text-base"
                    title={device.name}
                  >
                    {device.name}
                  </span>
                  <Badge variant="secondary">
                    {DEVICE_TYPE_LABELS[device.device_type]}
                  </Badge>
                </div>
                <p className="mt-1 text-xs text-muted-foreground md:text-sm">
                  {device.owner.username}
                  {device.connections.length > 0 &&
                    ` · ${device.connections.map((c) => CONNECTION_LABELS[c]).join(", ")}`}
                  {device.last_accessed_at &&
                    ` · Last used ${formatDateTime(device.last_accessed_at)}`}
                </p>
              </div>
              {canRevoke && (
                <Button
                  aria-label={`Revoke ${device.name}`}
                  onClick={() => setRevoking(device)}
                  size="sm"
                  variant="outline"
                >
                  Revoke
                </Button>
              )}
            </div>
          ))}
        </div>
      )}

      {canRevoke && (
        <ConfirmDialog
          confirmLabel="Revoke"
          description={`"${revoking?.name ?? ""}" (${revoking?.owner.username ?? ""}) stops reaching the library right away and is deleted. To use that device again, its owner must add a new Device and set it up again.`}
          isPending={revokeDevice.isPending}
          onConfirm={handleRevoke}
          onOpenChange={(open) => !open && setRevoking(null)}
          open={revoking !== null}
          title="Revoke Device"
        />
      )}
    </div>
  );
};

export default AdminDevices;
