import { CircleCheck, Loader2 } from "lucide-react";
import { useEffect, useId, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";

import {
  EReaderBrowserSetup,
  KoboSyncSetup,
  LoopbackWarning,
  NotWorkingLink,
  OPDSSetup,
} from "@/components/devices/ConnectionSetups";
import KOReaderPluginSetup from "@/components/devices/KOReaderPluginSetup";
import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import { Button } from "@/components/ui/button";
import {
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  useCreateDevice,
  useDevice,
  useDevices,
} from "@/hooks/queries/devices";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import { isNotFoundError, toastRequestError } from "@/libraries/api";
import {
  DeviceTypeBrowser,
  DeviceTypeKindle,
  DeviceTypeKobo,
  DeviceTypeKOReader,
  DeviceTypeOPDS,
  type DeviceResponse,
  type DeviceType,
} from "@/types/generated/devices";
import { CONNECT_CHOICES, connectChoiceFor, uniqueName } from "@/utils/devices";
import { formatCount } from "@/utils/format";

type Stage = "setup" | "waiting";

// How often the waiting screen checks on the Device itself, in case the
// device.connected event never arrived, such as while a phone's browser had
// the event stream closed in the background.
const CONNECTED_POLL_MS = 5000;

/** Whether a Device has reached Shisho through any of its connections. */
const hasConnected = (device: DeviceResponse) =>
  Boolean(device.last_accessed_at || device.plugin_last_connected_at);

// What the device does that the waiting screen is waiting for.
const WAITING_FOR: Record<DeviceType, string> = {
  [DeviceTypeKobo]: "Sync the Kobo once you've saved the file.",
  [DeviceTypeKOReader]:
    "Restart KOReader with Wi-Fi on once the plugin is in place.",
  [DeviceTypeKindle]: "Open the setup URL in the Kindle's web browser.",
  [DeviceTypeOPDS]: "Open the catalog in the reading app.",
  [DeviceTypeBrowser]: "Open the setup URL in the e-reader's web browser.",
};

const SetupSteps = ({ device }: { device: DeviceResponse }) => {
  switch (device.device_type) {
    case DeviceTypeKobo:
      return <KoboSyncSetup device={device} />;
    case DeviceTypeKOReader:
      return (
        <div className="space-y-4">
          <KOReaderPluginSetup device={device} inFlow />
          <p className="text-sm text-muted-foreground">
            Rather not use the plugin? Set up KOReader by hand from{" "}
            <Link
              className="underline underline-offset-2 hover:text-foreground"
              to={`/user/devices/${device.id}`}
            >
              this Device&apos;s page
            </Link>
            .
          </p>
        </div>
      );
    case DeviceTypeOPDS:
      return <OPDSSetup device={device} />;
    default:
      return <EReaderBrowserSetup device={device} />;
  }
};

// What a connected Device's owner does next, for the types with nothing
// left to set up.
const CONNECTED_NEXT: Record<Exclude<DeviceType, "koreader">, string> = {
  [DeviceTypeKobo]:
    "Your books arrive on the Kobo each time it syncs, under My Books.",
  [DeviceTypeKindle]:
    "Bookmark the page the setup URL opened, to come back to your books.",
  [DeviceTypeOPDS]: "Browse and download your books in the reading app.",
  [DeviceTypeBrowser]:
    "Bookmark the page the setup URL opened, to come back to your books.",
};

// What to do once a Device has connected. KOReader has a few things left to
// set; the rest say where the books are.
const ConnectedNext = ({ device }: { device: DeviceResponse }) => {
  if (device.device_type !== DeviceTypeKOReader) {
    return (
      <p className="text-sm text-muted-foreground">
        {CONNECTED_NEXT[device.device_type]}
      </p>
    );
  }
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">A few things left</p>
      <ul
        aria-label="What's next"
        className="list-disc space-y-2 pl-5 text-sm text-muted-foreground"
      >
        <li>
          If you set up KOReader&apos;s own progress sync before, turn it off
          under Tools &gt; Progress sync. The plugin syncs your reading time,
          position, and status now.
        </li>
        <li>
          Books on the e-reader that Shisho doesn&apos;t recognize wait on{" "}
          <Link
            className="underline underline-offset-2 hover:text-foreground"
            to={`/user/devices/${device.id}`}
          >
            this Device&apos;s page
          </Link>{" "}
          for you to link them to their books.
          {device.unlinked_books > 0 &&
            ` ${formatCount(device.unlinked_books, "book")} waiting now.`}
        </li>
        <li>
          Find your books in KOReader under Search &gt; OPDS catalog &gt;
          Shisho.
        </li>
      </ul>
    </div>
  );
};

interface ConnectDeviceFlowProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The Device this run of the flow created, once it has one. */
  deviceId?: string;
  /**
   * Keeps the created Device where a reload finds it, so the flow resumes
   * its setup instead of starting over and creating another. Undefined
   * starts over.
   */
  onDeviceIdChange: (deviceId: string | undefined) => void;
}

/**
 * ConnectDeviceFlow adds a Device in three steps: what the user reads on, in
 * plain words, the setup steps for that one way of connecting, and a screen
 * that waits for the device and switches to "Connected!" when it first
 * reaches Shisho. The device.connected event refreshes the Device (useSSE),
 * which is what switches it, with a slow poll behind it in case the event
 * was missed. A bottom sheet on phones, a dialog elsewhere.
 */
const ConnectDeviceFlow = ({
  open,
  onOpenChange,
  deviceId,
  onDeviceIdChange,
}: ConnectDeviceFlowProps) => {
  const [stage, setStage] = useState<Stage>("setup");
  const [deviceType, setDeviceType] = useState<DeviceType | null>(null);
  const [name, setName] = useState("");
  const [nameEdited, setNameEdited] = useState(false);
  const { data: devices } = useDevices();
  const createDevice = useCreateDevice();
  const deviceQuery = useDevice(deviceId, {
    refetchInterval: (query) =>
      open && query.state.data && !hasConnected(query.state.data)
        ? CONNECTED_POLL_MS
        : false,
  });
  // The app keeps a query's previous data while a new key loads, which here
  // would be the Device from an earlier run of the flow.
  const device =
    deviceQuery.data &&
    !deviceQuery.isPlaceholderData &&
    deviceQuery.data.id === deviceId
      ? deviceQuery.data
      : undefined;
  const groupId = useId();

  useEffect(() => {
    if (open) {
      setStage("setup");
      setDeviceType(null);
      setName("");
      setNameEdited(false);
    }
  }, [open]);

  // Only a name typed by hand is worth asking about before closing: the rest
  // is one click to choose again, and a created Device is already saved.
  const hasChanges = !deviceId && nameEdited && name.trim() !== "";
  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const choose = (type: DeviceType) => {
    setDeviceType(type);
    if (!nameEdited) {
      setName(
        uniqueName(
          connectChoiceFor(type).name,
          (devices ?? []).map((d) => d.name),
        ),
      );
    }
  };

  const handleContinue = async (e: FormEvent) => {
    e.preventDefault();
    if (!deviceType || !name.trim()) return;
    try {
      const created = await createDevice.mutateAsync({
        name: name.trim(),
        device_type: deviceType,
      });
      setNameEdited(false);
      setStage("setup");
      onDeviceIdChange(created.id);
    } catch (error) {
      toastRequestError(error, "Failed to add device");
    }
  };

  const connected = device ? hasConnected(device) : false;
  const choice = device ? connectChoiceFor(device.device_type) : undefined;

  let title: string;
  let description: string;
  let body: React.ReactNode;
  let footer: React.ReactNode;

  if (device && connected) {
    title = "Connected!";
    description = `${device.name} reached Shisho.`;
    body = (
      <div className="space-y-4">
        <div className="flex items-center gap-3 rounded-md bg-primary/10 p-3">
          <CircleCheck
            aria-hidden="true"
            className="h-6 w-6 shrink-0 text-primary"
          />
          <p className="text-sm font-medium">
            {device.name} is connected to Shisho.
          </p>
        </div>
        <ConnectedNext device={device} />
      </div>
    );
    footer = (
      <>
        <Button asChild size="sm" variant="outline">
          <Link onClick={requestClose} to={`/user/devices/${device.id}`}>
            Open Device
          </Link>
        </Button>
        <Button onClick={requestClose} size="sm">
          Done
        </Button>
      </>
    );
  } else if (device && stage === "waiting") {
    title = "Waiting for your device";
    description = "Step 3 of 3";
    body = (
      <div className="space-y-4">
        <div className="flex items-start gap-3">
          <Loader2
            aria-hidden="true"
            className="mt-0.5 h-5 w-5 shrink-0 animate-spin text-muted-foreground"
          />
          <div className="space-y-1">
            <p className="text-sm font-medium">
              Waiting for {device.name} to connect...
            </p>
            <p className="text-sm text-muted-foreground">
              {WAITING_FOR[device.device_type]} This changes to
              &quot;Connected!&quot; the moment it reaches Shisho.
            </p>
          </div>
        </div>
        {choice && (
          <p className="text-sm text-muted-foreground">
            <NotWorkingLink href={choice.troubleshootingURL}>
              Not working?
            </NotWorkingLink>{" "}
            See what usually goes wrong and how to fix it.
          </p>
        )}
      </div>
    );
    footer = (
      <Button onClick={() => setStage("setup")} size="sm" variant="outline">
        Back to the Steps
      </Button>
    );
  } else if (device) {
    title = `Set up ${device.name}`;
    description = "Step 2 of 3";
    body = (
      <div className="space-y-4">
        <LoopbackWarning />
        <SetupSteps device={device} />
      </div>
    );
    footer = (
      <Button onClick={() => setStage("waiting")} size="sm">
        Next
      </Button>
    );
  } else if (deviceId) {
    // The created Device is loading, or failed to.
    title = "Connect a device";
    description = "Step 2 of 3";
    const gone = isNotFoundError(deviceQuery.error);
    body =
      deviceQuery.error && !deviceQuery.isPlaceholderData ? (
        gone ? (
          <p className="text-sm text-muted-foreground">
            This Device was deleted, so there&apos;s nothing to set up. Start
            over to add another.
          </p>
        ) : (
          <QueryError fallback="Failed to load device" query={deviceQuery} />
        )
      ) : (
        <LoadingSpinner />
      );
    footer = gone ? (
      <Button onClick={() => onDeviceIdChange(undefined)} size="sm">
        Start Over
      </Button>
    ) : null;
  } else {
    title = "Connect a device";
    description = "Step 1 of 3";
    body = (
      <div className="space-y-6">
        <div className="space-y-3">
          <Label className="text-base font-semibold" id={`${groupId}-label`}>
            What do you read on?
          </Label>
          <RadioGroup
            aria-labelledby={`${groupId}-label`}

            name="connect-device-choice"
            onValueChange={(value) => choose(value as DeviceType)}
            value={deviceType ?? ""}
          >
            {CONNECT_CHOICES.map((option) => (
              <Label
                className="flex cursor-pointer items-start gap-3 rounded-md border border-border p-3 font-normal leading-snug has-[[data-state=checked]]:border-primary has-[[data-state=checked]]:bg-primary/5"
                htmlFor={`${groupId}-${option.type}`}
                key={option.type}
              >
                <RadioGroupItem
                  className="mt-0.5"
                  id={`${groupId}-${option.type}`}
                  value={option.type}
                />
                <span>
                  <span className="block font-medium">{option.label}</span>
                  <span className="block text-sm text-muted-foreground">
                    {option.description}
                  </span>
                </span>
              </Label>
            ))}
          </RadioGroup>
        </div>
        {deviceType && (
          <div className="space-y-2">
            <Label htmlFor={`${groupId}-name`}>Name</Label>
            <Input
              id={`${groupId}-name`}
              maxLength={100}
              onChange={(e) => {
                setName(e.target.value);
                setNameEdited(true);
              }}
              value={name}
            />
            <p className="text-xs text-muted-foreground">
              To tell your devices apart. You can rename it later.
            </p>
          </div>
        )}
      </div>
    );
    footer = (
      <Button
        disabled={createDevice.isPending || !deviceType || !name.trim()}
        size="sm"
        type="submit"
      >
        {createDevice.isPending && (
          <Loader2 aria-hidden="true" className="h-4 w-4 animate-spin" />
        )}
        Continue
      </Button>
    );
  }

  // One live region for the whole flow, so a screen reader hears the
  // waiting screen switch to Connected!, which a region mounted along with
  // its text may not announce.
  const announcement = !device
    ? ""
    : connected
      ? `${device.name} is connected to Shisho.`
      : stage === "waiting"
        ? `Waiting for ${device.name} to connect.`
        : "";

  const content = (
    <>
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{description}</DialogDescription>
      </DialogHeader>
      <DialogBody>{body}</DialogBody>
      {footer && <DialogFooter>{footer}</DialogFooter>}
      <p className="sr-only" role="status">
        {announcement}
      </p>
    </>
  );

  return (
    <FormDialog hasChanges={hasChanges} onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-xl" mobileSheet>
        {deviceId ? (
          content
        ) : (
          <form
            className="flex min-h-0 flex-1 flex-col"
            onSubmit={handleContinue}
          >
            {content}
          </form>
        )}
      </DialogContent>
    </FormDialog>
  );
};

export default ConnectDeviceFlow;
