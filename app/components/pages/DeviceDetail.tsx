import {
  ArrowLeft,
  Copy,
  Pencil,
  RefreshCw,
  Tablet,
  Trash2,
} from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import DeviceRulesSection from "@/components/devices/DeviceRulesSection";
import DeviceTypeRadioGroup from "@/components/devices/DeviceTypeRadioGroup";
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
  useClearKoboSync,
  useDeleteDevice,
  useDevice,
  useGenerateDeviceShortUrl,
  useRegenerateDeviceCredential,
  useUpdateDevice,
} from "@/hooks/queries/devices";
import { useUserLibraries } from "@/hooks/queries/libraries";
import { useListLists } from "@/hooks/queries/lists";
import { useAuth } from "@/hooks/useAuth";
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
  ConnectionOPDS,
  ScopeAll,
  ScopeLibrary,
  ScopeList,
  type Connection,
  type DeviceResponse,
  type DeviceShortURL,
  type DeviceType,
  type ScopeType,
} from "@/types/generated/devices";
import { copyText } from "@/utils/clipboard";
import {
  CONNECTION_LABELS,
  DEVICE_TYPE_LABELS,
  eReaderBrowserURL,
  koboSyncURL,
  opdsCatalogURL,
  shortURL,
} from "@/utils/devices";
import { formatDateTime } from "@/utils/format";

const copyToClipboard = async (value: string) => {
  if (await copyText(value)) {
    toast.success("Copied to clipboard");
  } else {
    toast.error("Could not copy");
  }
};

// CopyField shows a value in monospace with a copy button. A long URL
// wraps rather than scrolling inside a field, so it can be read on a phone.
const CopyField = ({ label, value }: { label: string; value: string }) => (
  <div className="flex items-start gap-2">
    <code
      aria-label={label}
      className="min-w-0 flex-1 break-all rounded-md border border-input px-3 py-2 font-mono text-sm"
    >
      {value}
    </code>
    <Button
      aria-label={`Copy ${label.toLowerCase()}`}
      onClick={() => copyToClipboard(value)}
      variant="outline"
    >
      <Copy className="h-4 w-4" />
    </Button>
  </div>
);

const Section = ({
  id,
  title,
  description,
  children,
}: {
  id: string;
  title: string;
  description?: string;
  children: React.ReactNode;
}) => (
  <section className="rounded-md border border-border p-4 md:p-6">
    <h2 className="text-lg font-semibold" id={id}>
      {title}
    </h2>
    {description && (
      <p className="mt-1 text-sm text-muted-foreground">{description}</p>
    )}
    <div className="mt-4 space-y-4">{children}</div>
  </section>
);

const CredentialSection = ({ device }: { device: DeviceResponse }) => {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const regenerate = useRegenerateDeviceCredential();

  const handleRegenerate = async () => {
    try {
      await regenerate.mutateAsync(device.id);
      toast.success("Credential regenerated");
      setConfirmOpen(false);
    } catch (error) {
      // Demo Mode already toasted the rejection; nothing will change, so
      // there is nothing left to confirm.
      if (isDemoModeError(error)) setConfirmOpen(false);
      toastRequestError(error, "Failed to regenerate credential");
    }
  };

  return (
    <Section
      description="Every connection signs in with this credential. Type it exactly as shown."
      id="credential-heading"
      title="Credential"
    >
      <CopyField label="Credential" value={device.credential} />
      <Button onClick={() => setConfirmOpen(true)} size="sm" variant="outline">
        <RefreshCw className="mr-2 h-4 w-4" />
        Regenerate
      </Button>
      <ConfirmDialog
        confirmLabel="Regenerate"
        description="Every URL with the current credential stops working, including the Kobo sync URL and eReader browser bookmarks. You'll need to set this Device up again with the new one."
        isPending={regenerate.isPending}
        onConfirm={handleRegenerate}
        onOpenChange={setConfirmOpen}
        open={confirmOpen}
        title="Regenerate Credential"
      />
    </Section>
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
      toast.success("Scope saved");
    } catch (error) {
      setScopeType(device.scope_type);
      toastRequestError(error, "Failed to save scope");
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
    <Section
      description="Which books this Device reaches, through every connection."
      id="scope-heading"
      title="Scope"
    >
      {device.url_scope_pending && (
        <p className="text-sm text-muted-foreground">
          This Kobo was set up before Devices. Its next sync sets the scope from
          the scope in its sync URL, unless you choose one here first.
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
            The {device.scope_type} this Device was scoped to no longer exists,
            so it reaches no books. Choose another.
          </p>
        )}
    </Section>
  );
};

const KoboSyncSetup = ({ device }: { device: DeviceResponse }) => {
  const clearKoboSync = useClearKoboSync();

  const handleResetSync = async () => {
    try {
      await clearKoboSync.mutateAsync(device.id);
      toast.success("Sync history cleared. The next sync is a fresh sync.");
    } catch (error) {
      toastRequestError(error, "Failed to clear sync history");
    }
  };

  return (
    <div className="space-y-4">
      <CopyField label="Kobo sync URL" value={koboSyncURL(device.credential)} />
      <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
        <li>Connect your Kobo to your computer with USB.</li>
        <li>
          Open{" "}
          <code className="rounded bg-muted px-1">
            .kobo/Kobo/Kobo eReader.conf
          </code>
          .
        </li>
        <li>
          Find{" "}
          <code className="break-all rounded bg-muted px-1">
            api_endpoint=https://storeapi.kobo.com
          </code>{" "}
          and replace the URL with the one above.
        </li>
        <li>Eject the Kobo and sync.</li>
      </ol>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          Clear the sync history to send everything again on the next sync.
        </p>
        <Button
          disabled={clearKoboSync.isPending}
          onClick={handleResetSync}
          size="sm"
          variant="outline"
        >
          {clearKoboSync.isPending ? "Resetting..." : "Reset Sync"}
        </Button>
      </div>
    </div>
  );
};

const EReaderBrowserSetup = ({ device }: { device: DeviceResponse }) => {
  const generateShortUrl = useGenerateDeviceShortUrl();
  const [short, setShort] = useState<DeviceShortURL | null>(null);

  const handleGenerate = async () => {
    try {
      setShort(await generateShortUrl.mutateAsync(device.id));
    } catch (error) {
      toastRequestError(error, "Failed to generate setup URL");
    }
  };

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        Open the setup URL in the e-reader's web browser, then bookmark the page
        it opens.
      </p>
      {short && (
        <>
          <CopyField label="Setup URL" value={shortURL(short.short_code)} />
          <p className="text-sm text-muted-foreground">
            The setup URL expires in 30 minutes, or when you get a new one. The
            page it opens keeps working.
          </p>
        </>
      )}
      <Button
        disabled={generateShortUrl.isPending}
        onClick={handleGenerate}
        size="sm"
        variant="outline"
      >
        {generateShortUrl.isPending
          ? "Generating..."
          : short
            ? "Get New Setup URL"
            : "Get Setup URL"}
      </Button>
      <div className="space-y-2">
        <p className="text-sm font-medium">Or bookmark this URL</p>
        <CopyField
          label="eReader browser URL"
          value={eReaderBrowserURL(device.credential)}
        />
      </div>
    </div>
  );
};

const OPDSSetup = ({ device }: { device: DeviceResponse }) => {
  const { user } = useAuth();
  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        Add an OPDS catalog in the reading app with this URL, your username, and
        this Device's credential as the password. Signing in with your account
        password instead follows your default Delivery Rules.
      </p>
      <CopyField label="OPDS URL" value={opdsCatalogURL()} />
      {user && <CopyField label="Username" value={user.username} />}
      <CopyField label="Password" value={device.credential} />
    </div>
  );
};

const CONNECTION_SETUP: Record<
  Connection,
  (props: { device: DeviceResponse }) => React.ReactNode
> = {
  [ConnectionKoboSync]: KoboSyncSetup,
  [ConnectionEReaderBrowser]: EReaderBrowserSetup,
  [ConnectionOPDS]: OPDSSetup,
};

// ConnectionsSection shows setup instructions for each connection the
// Device's type uses.
const ConnectionsSection = ({ device }: { device: DeviceResponse }) => (
  <Section
    description="How this Device connects to Shisho."
    id="connections-heading"
    title="Connections"
  >
    {device.connections.map((connection) => {
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
    })}
  </Section>
);

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
                {DEVICE_TYPE_LABELS[deviceType]} Device starts with, and its
                connections change to the ones that type uses. The credential
                stays the same.
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
              {device.last_accessed_at &&
                ` · Last used ${formatDateTime(device.last_accessed_at)}`}
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
              onClick={() => setChangeTypeOpen(true)}
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

        <div className="grid gap-6 lg:grid-cols-2 lg:items-start">
          <div className="min-w-0 space-y-6">
            <CredentialSection device={device} />
            <ScopeSection device={device} />
            <Section
              description="Which File Types this Device receives, and as what. For each book, the earliest rule matching one of its files decides what goes."
              id="delivery-rules-heading"
              title="Delivery Rules"
            >
              <DeviceRulesSection
                device={device}
                onHasChangesChange={setRulesHaveChanges}
              />
            </Section>
          </div>
          <div className="min-w-0">
            <ConnectionsSection device={device} />
          </div>
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
      <div className="mx-auto w-full max-w-7xl px-4 py-4 md:px-6 md:py-8">
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
