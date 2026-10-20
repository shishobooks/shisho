import { Plus } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import CreateDeviceDialog from "@/components/devices/CreateDeviceDialog";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useDevices } from "@/hooks/queries/devices";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { DeviceResponse } from "@/types/generated/devices";
import {
  CONNECTION_LABELS,
  DEVICE_TYPE_LABELS,
  scopeLabel,
} from "@/utils/devices";
import { formatDateTime } from "@/utils/format";

const DeviceRow = ({ device }: { device: DeviceResponse }) => {
  const connections = device.connections
    .map((connection) => CONNECTION_LABELS[connection])
    .join(" · ");
  return (
    <Link
      className="flex items-center justify-between gap-3 px-4 py-3 transition-colors hover:bg-muted/50 md:px-6 md:py-4"
      to={`/user/devices/${device.id}`}
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
          {scopeLabel(device)} · {connections}
          {device.last_accessed_at &&
            ` · Last used ${formatDateTime(device.last_accessed_at)}`}
        </p>
      </div>
    </Link>
  );
};

const Devices = () => {
  usePageTitle("Devices");
  const navigate = useNavigate();
  const devicesQuery = useDevices();
  const { data: devices, isLoading } = devicesQuery;
  const [createOpen, setCreateOpen] = useState(false);

  return (
    <div>
      <TopNav />
      <div className="w-full px-4 py-4 md:px-6 md:py-8">
        <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between md:mb-8">
          <div>
            <h1 className="mb-1 text-2xl font-semibold md:mb-2">Devices</h1>
            <p className="text-sm text-muted-foreground md:text-base">
              Add a Device for each reading app that connects to Shisho. Kobo
              sync and the eReader browser sign in with the Device's credential.
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button
              aria-label="Add Device"
              onClick={() => setCreateOpen(true)}
              size="sm"
            >
              <Plus className="h-4 w-4 sm:mr-2" />
              <span className="hidden sm:inline">Add Device</span>
            </Button>
          </div>
        </div>

        <div>
          {isLoading ? (
            <LoadingSpinner />
          ) : devicesQuery.error && !devices ? (
            <QueryError
              fallback="Failed to load devices"
              query={devicesQuery}
            />
          ) : !devices?.length ? (
            <div className="rounded-md border border-border p-8 text-center">
              <p className="text-muted-foreground">
                No devices yet. Add one to sync a Kobo or browse your library
                from an e-reader.
              </p>
            </div>
          ) : (
            <div className="divide-y divide-border rounded-md border border-border">
              {devices.map((device) => (
                <DeviceRow device={device} key={device.id} />
              ))}
            </div>
          )}
        </div>
      </div>

      <CreateDeviceDialog
        onCreated={(device) => navigate(`/user/devices/${device.id}`)}
        onOpenChange={setCreateOpen}
        open={createOpen}
      />
    </div>
  );
};

export default Devices;
