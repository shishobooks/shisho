import {
  ArrowLeft,
  CircleCheck,
  CircleDashed,
  Pencil,
  RefreshCw,
  Tablet,
  Trash2,
} from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import {
  EReaderBrowserSetup,
  KoboSyncSetup,
  KOReaderSyncSetup,
  LoopbackWarning,
  NotWorkingLink,
  OPDSSetup,
} from "@/components/devices/ConnectionSetups";
import DeviceRulesSection from "@/components/devices/DeviceRulesSection";
import DeviceSection from "@/components/devices/DeviceSection";
import DeviceTypeRadioGroup from "@/components/devices/DeviceTypeRadioGroup";
import Disclosure from "@/components/devices/Disclosure";
import KOReaderPluginSetup from "@/components/devices/KOReaderPluginSetup";
import KOReaderUnrecognizedBooks from "@/components/devices/KOReaderUnrecognizedBooks";
import CopyField from "@/components/library/CopyField";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import TopNav from "@/components/library/TopNav";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";
import {
  useDeleteDevice,
  useDevice,
  useRegenerateDeviceCredential,
  useUpdateDevice,
} from "@/hooks/queries/devices";
import { useUserLibraries } from "@/hooks/queries/libraries";
import { useListLists } from "@/hooks/queries/lists";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import {
  isDemoModeError,
  isLoadFailure,
  toastRequestError,
} from "@/libraries/api";
import { cn } from "@/libraries/utils";
import {
  ConnectionEReaderBrowser,
  ConnectionKoboSync,
  ConnectionKOReaderPlugin,
  ConnectionKOReaderSync,
  ConnectionOPDS,
  ScopeAll,
  ScopeLibrary,
  ScopeList,
  type Connection,
  type DeviceResponse,
  type DeviceType,
  type ScopeType,
} from "@/types/generated/devices";
import {
  connectChoiceFor,
  CONNECTION_LABELS,
  DEVICE_TYPE_LABELS,
} from "@/utils/devices";
import { formatDateTime } from "@/utils/format";

const CredentialSection = ({ device }: { device: DeviceResponse }) => {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const regenerate = useRegenerateDeviceCredential();

  const handleRegenerate = async () => {
    try {
      await regenerate.mutateAsync(device.id);
      toast.success("Password regenerated");
      setConfirmOpen(false);
    } catch (error) {
      // Demo Mode already toasted the rejection; nothing will change, so
      // there is nothing left to confirm.
      if (isDemoModeError(error)) setConfirmOpen(false);
      toastRequestError(error, "Failed to regenerate password");
    }
  };

  return (
    <DeviceSection
      description="However this Device connects, it signs in with this password, which is also part of its URLs. Type it exactly as shown."
      id="credential-heading"
      title="Device password"
    >
      <CopyField label="Device password" value={device.credential} />
      <Button onClick={() => setConfirmOpen(true)} size="sm" variant="outline">
        <RefreshCw className="mr-2 h-4 w-4" />
        Regenerate
      </Button>
      <ConfirmDialog
        confirmLabel="Regenerate"
        description="Every URL with the current password stops working, including the Kobo sync URL and eReader browser bookmarks. You'll need to set this Device up again with the new one."
        isPending={regenerate.isPending}
        onConfirm={handleRegenerate}
        onOpenChange={setConfirmOpen}
        open={confirmOpen}
        title="Regenerate Password"
      />
    </DeviceSection>
  );
};

const SCOPE_OPTIONS: { type: ScopeType; label: string }[] = [
  { type: ScopeAll, label: "All Books" },
  { type: ScopeLibrary, label: "Library" },
  { type: ScopeList, label: "List" },
];

// ScopeSection saves each choice as it is made. Picking Library or List
// saves once a library or list is chosen.
const ScopeSection = ({ device }: { device: DeviceResponse }) => {
  const updateDevice = useUpdateDevice();
  const [scopeType, setScopeType] = useState<ScopeType>(device.scope_type);
  const { data: libraries } = useUserLibraries({
    enabled: scopeType === ScopeLibrary,
  });
  const { data: listsData } = useListLists(
    {},
    { enabled: scopeType === ScopeList },
  );

  useEffect(() => {
    setScopeType(device.scope_type);
  }, [device.scope_type, device.scope_library_id, device.scope_list_id]);

  const save = async (payload: {
    type: ScopeType;
    library_id?: number;
    list_id?: number;
  }) => {
    try {
      await updateDevice.mutateAsync({
        id: device.id,
        payload: { scope: payload },
      });
      toast.success("Saved which books this Device reaches");
    } catch (error) {
      setScopeType(device.scope_type);
      toastRequestError(
        error,
        "Failed to save which books this Device reaches",
      );
    }
  };

  const chooseType = (type: ScopeType) => {
    setScopeType(type);
    // A migrated Kobo still waiting to take its scope from its old sync URL
    // saves All Books too, so the URL's scope never arrives.
    if (
      type === ScopeAll &&
      (device.scope_type !== ScopeAll || device.url_scope_pending)
    ) {
      void save({ type: ScopeAll });
    }
  };

  const savedLibrary =
    device.scope_type === ScopeLibrary && device.scope_library_id
      ? String(device.scope_library_id)
      : "";
  const savedList =
    device.scope_type === ScopeList && device.scope_list_id
      ? String(device.scope_list_id)
      : "";

  return (
    <DeviceSection
      description="The books this Device can reach, however it connects."
      id="scope-heading"
      title="Which books"
    >
      {device.url_scope_pending && (
        <p className="text-sm text-muted-foreground">
          This Kobo was set up before Devices. Its next sync picks its books
          from its old sync URL, unless you choose here first.
        </p>
      )}
      <div
        aria-labelledby="scope-heading"
        className="flex rounded-md border border-input"
        role="group"
      >
        {SCOPE_OPTIONS.map((option, i) => (
          <Button
            aria-pressed={scopeType === option.type}
            className={cn(
              "flex-1 px-3 py-2 text-sm font-medium transition-colors first:rounded-l-md last:rounded-r-md",
              i === 1 && "border-x border-input",
              scopeType === option.type
                ? "bg-primary text-primary-foreground"
                : "hover:bg-muted",
            )}
            disabled={updateDevice.isPending}
            key={option.type}
            onClick={() => chooseType(option.type)}
            variant="unstyled"
          >
            {option.label}
          </Button>
        ))}
      </div>
      {scopeType === ScopeLibrary && libraries && (
        <Select
          onValueChange={(value) =>
            save({ type: ScopeLibrary, library_id: Number(value) })
          }
          value={savedLibrary}
        >
          <SelectTrigger aria-label="Library">
            <SelectValue placeholder="Choose a library..." />
          </SelectTrigger>
          <SelectContent>
            {libraries.map((library) => (
              <SelectItem key={library.id} value={String(library.id)}>
                {library.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      {scopeType === ScopeList && listsData && (
        <Select
          onValueChange={(value) =>
            save({ type: ScopeList, list_id: Number(value) })
          }
          value={savedList}
        >
          <SelectTrigger aria-label="List">
            <SelectValue placeholder="Choose a list..." />
          </SelectTrigger>
          <SelectContent>
            {listsData.items.map((list) => (
              <SelectItem key={list.id} value={String(list.id)}>
                {list.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      {scopeType !== device.scope_type && scopeType !== ScopeAll && (
        <p className="text-sm text-muted-foreground">
          Nothing is saved until you choose a {scopeType}.
        </p>
      )}
      {device.scope_type !== ScopeAll &&
        !device.scope_library_id &&
        !device.scope_list_id &&
        scopeType === device.scope_type && (
          <p className="text-sm text-destructive">
            The {device.scope_type} this Device used no longer exists, so it
            reaches no books. Choose another.
          </p>
        )}
    </DeviceSection>
  );
};

const CONNECTION_SETUP: Record<
  Connection,
  (props: { device: DeviceResponse }) => React.ReactNode
> = {
  [ConnectionKoboSync]: KoboSyncSetup,
  [ConnectionEReaderBrowser]: EReaderBrowserSetup,
  [ConnectionOPDS]: OPDSSetup,
  [ConnectionKOReaderSync]: KOReaderSyncSetup,
  [ConnectionKOReaderPlugin]: KOReaderPluginSetup,
};

const ConnectionSetups = ({
  device,
  connections,
}: {
  device: DeviceResponse;
  connections: Connection[];
}) =>
  connections.map((connection) => {
    const Setup = CONNECTION_SETUP[connection];
    return (
      <div
        className="space-y-4 border-t border-border pt-4 first:border-t-0 first:pt-0"
        key={connection}
      >
        <h3 className="text-base font-medium">
          {CONNECTION_LABELS[connection]}
        </h3>
        <Setup device={device} key={device.credential} />
      </div>
    );
  });

// StatusSection says whether the Device has reached Shisho yet, and when it
// was last used, with where to look if it never has.
const StatusSection = ({ device }: { device: DeviceResponse }) => {
  const lastUsed = device.last_accessed_at ?? device.plugin_last_connected_at;
  return (
    <DeviceSection id="status-heading" title="Status">
      {lastUsed ? (
        <p
          className="flex items-center gap-2 text-sm"
          data-testid="device-status"
        >
          <CircleCheck
            aria-hidden="true"
            className="h-4 w-4 shrink-0 text-primary"
          />
          Connected. Last used {formatDateTime(lastUsed)}.
        </p>
      ) : (
        <div className="space-y-1">
          <p
            className="flex items-center gap-2 text-sm"
            data-testid="device-status"
          >
            <CircleDashed
              aria-hidden="true"
              className="h-4 w-4 shrink-0 text-muted-foreground"
            />
            Not connected yet. Follow the setup steps below on your device.
          </p>
          <p className="text-sm text-muted-foreground">
            <NotWorkingLink
              href={connectChoiceFor(device.device_type).troubleshootingURL}
            >
              Not working?
            </NotWorkingLink>{" "}
            See what usually goes wrong and how to fix it.
          </p>
        </div>
      )}
    </DeviceSection>
  );
};

// SetupSection shows setup steps for each way the Device's type connects. A
// KOReader Device leads with the KOReader plugin and keeps the ways it
// replaces in a collapsed section, for anyone who doesn't want the plugin.
const SetupSection = ({ device }: { device: DeviceResponse }) => {
  const hasPlugin = device.connections.includes(ConnectionKOReaderPlugin);
  const others = device.connections.filter(
    (connection) => connection !== ConnectionKOReaderPlugin,
  );

  return (
    <DeviceSection
      description="How to connect this Device to Shisho."
      id="setup-heading"
      title="Setup"
    >
      <LoopbackWarning />
      {hasPlugin ? (
        <>
          <ConnectionSetups
            connections={[ConnectionKOReaderPlugin]}
            device={device}
          />
          <div className="border-t border-border pt-4">
            <Disclosure
              description="Set up OPDS and KOReader's progress sync by hand instead."
              title="Without the plugin"
            >
              <ConnectionSetups connections={others} device={device} />
            </Disclosure>
          </div>
        </>
      ) : (
        <ConnectionSetups connections={device.connections} device={device} />
      )}
    </DeviceSection>
  );
};

const RenameDeviceDialog = ({
  device,
  open,
  onOpenChange,
}: {
  device: DeviceResponse;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) => {
  const [name, setName] = useState(device.name);
  const updateDevice = useUpdateDevice();

  useEffect(() => {
    if (open) {
      setName(device.name);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const hasChanges = name !== device.name;
  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await updateDevice.mutateAsync({
        id: device.id,
        payload: { name: name.trim() },
      });
      requestClose();
    } catch (error) {
      toastRequestError(error, "Failed to rename device");
    }
  };

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Rename Device</DialogTitle>
          </DialogHeader>
          <DialogBody>
            <div className="space-y-2">
              <Label htmlFor="rename-device">Name</Label>
              <Input
                id="rename-device"
                maxLength={100}
                onChange={(e) => setName(e.target.value)}
                value={name}
              />
            </div>
          </DialogBody>
          <DialogFooter>
            <Button
              disabled={updateDevice.isPending || !hasChanges || !name.trim()}
              size="sm"
              type="submit"
            >
              {updateDevice.isPending ? "Saving..." : "Save"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </FormDialog>
  );
};

const ChangeTypeDialog = ({
  device,
  open,
  onOpenChange,
  rulesHaveChanges,
}: {
  device: DeviceResponse;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Unsaved Delivery Rules edits, which a type change would discard. */
  rulesHaveChanges: boolean;
}) => {
  const [deviceType, setDeviceType] = useState<DeviceType>(device.device_type);
  const updateDevice = useUpdateDevice();

  useEffect(() => {
    if (open) {
      setDeviceType(device.device_type);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const hasChanges = deviceType !== device.device_type;
  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await updateDevice.mutateAsync({
        id: device.id,
        payload: { device_type: deviceType },
      });
      toast.success("Device type changed");
      requestClose();
    } catch (error) {
      toastRequestError(error, "Failed to change device type");
    }
  };

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent>
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>Change Type</DialogTitle>
          </DialogHeader>
          <DialogBody className="space-y-4">
            <DeviceTypeRadioGroup
              id="change-device-type"
              onChange={setDeviceType}
              value={deviceType}
            />
            {rulesHaveChanges && (
              <p className="text-sm text-destructive" role="alert">
                Save or discard your Delivery Rules changes before changing the
                type.
              </p>
            )}
            {hasChanges && (
              <p className="text-sm text-muted-foreground">
                Its Delivery Rules go back to what a{" "}
                {DEVICE_TYPE_LABELS[deviceType]} Device starts with, and the
                ways it connects change to that type&apos;s. Its password stays
                the same.
              </p>
            )}
          </DialogBody>
          <DialogFooter>
            <Button
              disabled={
                updateDevice.isPending || !hasChanges || rulesHaveChanges
              }
              size="sm"
              type="submit"
            >
              {updateDevice.isPending ? "Saving..." : "Change Type"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </FormDialog>
  );
};

const DeviceDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const deviceQuery = useDevice(id);
  const device = deviceQuery.data;
  usePageTitle(device?.name);
  const deleteDevice = useDeleteDevice();
  const [renameOpen, setRenameOpen] = useState(false);
  const [changeTypeOpen, setChangeTypeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [rulesHaveChanges, setRulesHaveChanges] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  // Unrecognized Books goes at the top when books are waiting to be linked
  // as the page loads, since they're the one thing on it to act on, and
  // stays there for the visit, so linking the last one doesn't move the
  // section out from under the user.
  const [unrecognizedAtTop, setUnrecognizedAtTop] = useState<boolean | null>(
    null,
  );
  if (device && unrecognizedAtTop === null) {
    setUnrecognizedAtTop(device.unlinked_books > 0);
  }
  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(rulesHaveChanges);

  const handleDelete = async () => {
    if (!device) return;
    try {
      await deleteDevice.mutateAsync(device.id);
      toast.success("Device deleted");
      setDeleteOpen(false);
      navigate("/user/devices");
    } catch (error) {
      if (isDemoModeError(error)) setDeleteOpen(false);
      toastRequestError(error, "Failed to delete device");
    }
  };

  let body: React.ReactNode;
  if (deviceQuery.isLoading) {
    body = <LoadingSpinner />;
  } else if (isLoadFailure(deviceQuery)) {
    body = <QueryError fallback="Failed to load device" query={deviceQuery} />;
  } else if (!device) {
    body = (
      <div className="text-center">
        <h1 className="mb-4 text-2xl font-semibold">Device Not Found</h1>
        <p className="text-muted-foreground">
          The device you're looking for doesn't exist or may have been deleted.
        </p>
      </div>
    );
  } else {
    const hasPlugin = device.connections.includes(ConnectionKOReaderPlugin);
    body = (
      <>
        <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between md:mb-8">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2 md:gap-3">
              <h1 className="break-words text-2xl font-semibold">
                {device.name}
              </h1>
              <Badge variant="secondary">
                {DEVICE_TYPE_LABELS[device.device_type]}
              </Badge>
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              Added {formatDateTime(device.created_at)}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button
              aria-label="Rename"
              onClick={() => setRenameOpen(true)}
              size="sm"
              variant="outline"
            >
              <Pencil className="h-4 w-4 sm:mr-2" />
              <span className="hidden sm:inline">Rename</span>
            </Button>
            <Button
              aria-label="Change Type"
              onClick={() => {
                // Change Type waits on unsaved Delivery Rules edits, so they
                // show where it says to save or discard them.
                if (rulesHaveChanges) setAdvancedOpen(true);
                setChangeTypeOpen(true);
              }}
              size="sm"
              variant="outline"
            >
              <Tablet className="h-4 w-4 sm:mr-2" />
              <span className="hidden sm:inline">Change Type</span>
            </Button>
            <Button
              aria-label="Delete"
              onClick={() => setDeleteOpen(true)}
              size="sm"
              variant="outline"
            >
              <Trash2 className="h-4 w-4 text-destructive sm:mr-2" />
              <span className="hidden sm:inline">Delete</span>
            </Button>
          </div>
        </div>

        <div className="max-w-4xl space-y-6">
          {hasPlugin && unrecognizedAtTop && (
            <KOReaderUnrecognizedBooks device={device} />
          )}
          <StatusSection device={device} />
          <SetupSection device={device} />
          {hasPlugin && !unrecognizedAtTop && (
            <KOReaderUnrecognizedBooks device={device} />
          )}
          <section className="pt-2">
            {/* Kept mounted, so closing it never drops unsaved Delivery
                Rules edits. */}
            <Disclosure
              badge={
                rulesHaveChanges &&
                !advancedOpen && (
                  <Badge variant="secondary">Unsaved changes</Badge>
                )
              }
              description="This Device's password, which books it reaches, and which files it gets."
              heading="h2"
              keepMounted
              onOpenChange={setAdvancedOpen}
              open={advancedOpen}
              title="Advanced"
            >
              <div className="space-y-6">
                <CredentialSection device={device} />
                <ScopeSection device={device} />
                <DeviceSection
                  description="Which kinds of files this Device gets, and in what format. When a book has several files, the first rule that fits one of them decides what's sent."
                  id="delivery-rules-heading"
                  title="Delivery Rules"
                >
                  <DeviceRulesSection
                    device={device}
                    onHasChangesChange={setRulesHaveChanges}
                  />
                </DeviceSection>
              </div>
            </Disclosure>
          </section>
        </div>

        <RenameDeviceDialog
          device={device}
          onOpenChange={setRenameOpen}
          open={renameOpen}
        />
        <ChangeTypeDialog
          device={device}
          onOpenChange={setChangeTypeOpen}
          open={changeTypeOpen}
          rulesHaveChanges={rulesHaveChanges}
        />
        <ConfirmDialog
          confirmLabel="Delete"
          description={`"${device.name}" will no longer be able to reach your library. To use it again, add it as a new Device and set it up again.`}
          isPending={deleteDevice.isPending}
          onConfirm={handleDelete}
          onOpenChange={setDeleteOpen}
          open={deleteOpen}
          title="Delete Device"
          variant="destructive"
        />
      </>
    );
  }

  return (
    <div>
      <TopNav />
      <div className="w-full px-4 py-4 md:px-6 md:py-8">
        <Link
          className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
          to="/user/devices"
        >
          <ArrowLeft className="h-4 w-4" />
          Devices
        </Link>
        {body}
      </div>
      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />
    </div>
  );
};

export default DeviceDetail;
