import { Plus, TabletSmartphone } from "lucide-react";
import { Link, useSearchParams } from "react-router-dom";

import ConnectDeviceFlow from "@/components/devices/ConnectDeviceFlow";
import EmptyState from "@/components/library/EmptyState";
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
import { formatCount, formatDateTime } from "@/utils/format";

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
          {device.unlinked_books > 0 && (
            <Badge>{formatCount(device.unlinked_books, "book")} to link</Badge>
          )}
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

// The search param that opens the Connect a device flow, so Getting Started
// can link straight to it. Once the flow creates a Device, it holds the
// Device's id, so a reload resumes that Device's setup.
const CONNECT_PARAM = "connect";

const Devices = () => {
  usePageTitle("Devices");
  const devicesQuery = useDevices();
  const { data: devices, isLoading } = devicesQuery;
  const [searchParams, setSearchParams] = useSearchParams();
  const connectParam = searchParams.get(CONNECT_PARAM);
  const connectOpen = connectParam !== null;
  const setConnectParam = (value: string | null, replace: boolean) =>
    setSearchParams(
      (params) => {
        if (value === null) params.delete(CONNECT_PARAM);
        else params.set(CONNECT_PARAM, value);
        return params;
      },
      { preventScrollReset: true, replace },
    );
  const setConnectOpen = (open: boolean) =>
    setConnectParam(open ? "" : null, !open);

  return (
    <div>
      <TopNav />
      <div className="w-full px-4 py-4 md:px-6 md:py-8">
        <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between md:mb-8">
          <div>
            <h1 className="mb-1 text-2xl font-semibold md:mb-2">Devices</h1>
            <p className="text-sm text-muted-foreground md:text-base">
              Each e-reader or reading app you read on, and how it gets your
              books from Shisho.
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button
              aria-label="Connect Device"
              onClick={() => setConnectOpen(true)}
              size="sm"
            >
              <Plus className="h-4 w-4 sm:mr-2" />
              <span className="hidden sm:inline">Connect Device</span>
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
            <EmptyState
              action={
                <Button onClick={() => setConnectOpen(true)} size="sm">
                  <Plus aria-hidden="true" className="h-4 w-4" />
                  Connect Device
                </Button>
              }
              description="Connect each e-reader or reading app you use: a Kobo, KOReader on any device, an e-reader's web browser, or a reading app on your phone. Your books then sync to it or download right on it."
              icon={TabletSmartphone}
              title="No devices yet"
            />
          ) : (
            <div className="divide-y divide-border rounded-md border border-border">
              {devices.map((device) => (
                <DeviceRow device={device} key={device.id} />
              ))}
            </div>
          )}
        </div>
      </div>

      <ConnectDeviceFlow
        deviceId={connectParam || undefined}
        onDeviceIdChange={(id) => setConnectParam(id ?? "", true)}
        onOpenChange={setConnectOpen}
        open={connectOpen}
      />
    </div>
  );
};

export default Devices;
