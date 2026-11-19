import { Download, KeyRound } from "lucide-react";
import { Fragment, useEffect, useId, useState } from "react";

import Disclosure from "@/components/devices/Disclosure";
import HiddenFolderHelp from "@/components/devices/HiddenFolderHelp";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  useDownloadKOReaderPlugin,
  useGenerateDeviceShortUrl,
} from "@/hooks/queries/devices";
import { useAuth } from "@/hooks/useAuth";
import { toastRequestError } from "@/libraries/api";
import type { DeviceResponse, DeviceShortURL } from "@/types/generated/devices";
import { KOREADER_PLUGIN_FOLDERS } from "@/utils/devices";
import {
  formatDateTime,
  formatPlayerTime,
  formatTimeOfDay,
} from "@/utils/format";

// The zip's name, which matches what the server sends.
const PLUGIN_ZIP_NAME = "shisho.koplugin.zip";

const saveBlob = (blob: Blob, name: string) => {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
};

// remaining formats the time until expiresAt as m:ss, or null once it has
// passed.
const remaining = (expiresAt: string, now: number): string | null => {
  const seconds = Math.floor((new Date(expiresAt).getTime() - now) / 1000);
  return seconds > 0 ? formatPlayerTime(seconds) : null;
};

// PairingCode shows a Device short code for the KOReader plugin's sign-in,
// counting down to when it expires.
const PairingCode = ({ short }: { short: DeviceShortURL }) => {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  const left = remaining(short.expires_at, now);

  if (!left) {
    return (
      <p className="text-sm text-muted-foreground" role="status">
        That code has expired. Press Pair for a new one.
      </p>
    );
  }
  return (
    <div className="space-y-1" role="status">
      <p
        aria-label="Pairing code"
        className="font-mono text-3xl font-semibold tracking-widest"
      >
        {short.short_code}
      </p>
      <p className="text-sm text-muted-foreground">
        Enter it in KOReader under Tools &gt; Shisho &gt; Add a server. It
        expires in {left}, or when you press Pair or download the plugin again.
      </p>
    </div>
  );
};

/**
 * KOReaderPluginSetup leads a KOReader Device's setup: downloading and
 * installing the KOReader plugin, pairing it, and when it last connected.
 * In the Connect a device flow (inFlow), it's only the steps: the flow says
 * when the plugin connects.
 */
const KOReaderPluginSetup = ({
  device,
  inFlow = false,
}: {
  device: DeviceResponse;
  inFlow?: boolean;
}) => {
  const { demoMode } = useAuth();
  const download = useDownloadKOReaderPlugin();
  const pair = useGenerateDeviceShortUrl();
  const [short, setShort] = useState<DeviceShortURL | null>(null);
  const [signsInUntil, setSignsInUntil] = useState<Date | null>(null);
  const [pairingOpen, setPairingOpen] = useState(false);
  const pairingId = useId();

  const handleDownload = async () => {
    try {
      const { blob, signsInUntil } = await download.mutateAsync(device.id);
      saveBlob(blob, PLUGIN_ZIP_NAME);
      // The download replaced any code shown here.
      setShort(null);
      setSignsInUntil(signsInUntil);
    } catch (error) {
      toastRequestError(error, "Failed to download the KOReader plugin");
    }
  };

  const handlePair = async () => {
    try {
      setShort(await pair.mutateAsync(device.id));
      // The new code replaced the download's.
      setSignsInUntil(null);
    } catch (error) {
      toastRequestError(error, "Failed to get a pairing code");
    }
  };

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        {inFlow ? "" : "Recommended. "}The plugin signs in to Shisho once, sets
        up this server&apos;s OPDS catalog, syncs your reading time, position,
        status, and Rating, and keeps itself up to date.
      </p>
      {!inFlow && <PluginStatus device={device} />}
      {/* Demo Mode hides downloads. */}
      {!demoMode && (
        <div className="space-y-1">
          <Button
            disabled={download.isPending}
            onClick={handleDownload}
            size="sm"
          >
            <Download className="mr-2 h-4 w-4" />
            {download.isPending ? "Downloading..." : "Download Plugin"}
          </Button>
          {signsInUntil && (
            <p className="text-sm text-muted-foreground" role="status">
              This download signs itself in until{" "}
              {formatTimeOfDay(signsInUntil)}, no pairing code needed.
            </p>
          )}
        </div>
      )}
      {/* Once the plugin has connected, it's installed: the steps are there
          for a reinstall, out of the way. While connecting, they're the
          point, so they're always open. */}
      {inFlow ? (
        <InstallSteps />
      ) : (
        <Disclosure
          defaultOpen={!device.plugin_last_connected_at}
          size="sm"
          title="Install steps"
        >
          <InstallSteps />
        </Disclosure>
      )}
      {/* A download signs itself in, so pairing is for later sign-ins and
          stays out of the way until KOReader asks for a code. */}
      <div className="space-y-2">
        <Button
          aria-controls={pairingId}
          aria-expanded={pairingOpen}
          onClick={() => setPairingOpen(!pairingOpen)}
          variant="link"
        >
          KOReader asking for a pairing code?
        </Button>
        {pairingOpen && (
          <div className="space-y-3" id={pairingId}>
            <p className="text-sm text-muted-foreground">
              A code is only for signing in later than 30 minutes after
              downloading, for the plugin from Shisho's GitHub release, or for
              Add a server in KOReader. A new code also replaces the sign-in of
              a download KOReader hasn't used yet.
            </p>
            <Button
              disabled={pair.isPending}
              onClick={handlePair}
              size="sm"
              variant="outline"
            >
              <KeyRound className="mr-2 h-4 w-4" />
              {pair.isPending ? "Pairing..." : "Pair"}
            </Button>
            {short && <PairingCode key={short.short_code} short={short} />}
          </div>
        )}
      </div>
      {!inFlow && (
        <Disclosure size="sm" title="Why use the plugin?">
          <p className="text-sm text-muted-foreground">
            With the plugin, you sign in once with a pairing code instead of
            typing this Device's password into KOReader twice, the OPDS catalog
            is set up for you, and the plugin updates itself from Shisho. Your
            reading time comes from KOReader's own statistics, page by page, and
            reading done offline arrives on the days you did it, however long
            the e-reader stays offline. Finishing, pausing, and rating a book in
            KOReader carry over to Shisho, and the same changes made here show
            up in KOReader. One e-reader can sync with several Shisho servers,
            and a copy Shisho doesn't recognize, such as one downloaded before
            v0.7.0, can be linked to its book here. Progress sync is built into
            KOReader and syncs your position in the open book, but Shisho can
            only estimate your reading time from it, offline reading is dated
            when the e-reader next connects, and a book counts as read only once
            you reach its last pages. Once the plugin syncs, turn progress sync
            off.
          </p>
        </Disclosure>
      )}
    </div>
  );
};

// Which steps connect the plugin's folder on each platform: an e-reader over
// USB, Android in its own file manager, and a computer needs neither.
const openFolderStep = (platform: string | undefined): string | null => {
  switch (platform) {
    case "Android":
      return "Open a file manager on the Android device.";
    case "Linux":
    case "macOS":
      return null;
    case undefined:
      return "Connect the e-reader to your computer with USB, or on Android open a file manager.";
    default:
      return "Connect the e-reader to your computer with USB.";
  }
};

// How the restart step starts: an e-reader plugged in over USB is ejected
// first, and before the platform is known it might not be one.
const restartStep = (platform: string | undefined): string => {
  switch (platform) {
    case undefined:
      return "Eject the e-reader if it's connected with USB, then restart";
    case "Android":
    case "Linux":
    case "macOS":
      return "Restart";
    default:
      return "Eject the e-reader and restart";
  }
};

// InstallSteps says how to install the downloaded plugin, asking which
// device KOReader runs on so it names only that device's plugins folder.
const InstallSteps = () => {
  const [platform, setPlatform] = useState<string>();
  const groupId = useId();
  const folder = KOREADER_PLUGIN_FOLDERS.find(
    (f) => f.platform === platform,
  )?.folder;
  const openStep = openFolderStep(platform);

  return (
    <ol
      aria-label="KOReader plugin setup"
      className="list-decimal space-y-3 pl-5 text-sm text-muted-foreground"
    >
      <li className="space-y-2">
        <p id={`${groupId}-label`}>Which device runs KOReader?</p>
        <RadioGroup
          aria-labelledby={`${groupId}-label`}
          className="grid grid-cols-2 gap-2 sm:grid-cols-3"
          name="koreader-platform"
          onValueChange={setPlatform}
          value={platform ?? ""}
        >
          {KOREADER_PLUGIN_FOLDERS.map((option) => (
            <div className="flex items-center gap-2" key={option.platform}>
              <RadioGroupItem
                id={`${groupId}-${option.platform}`}
                value={option.platform}
              />
              <Label
                className="cursor-pointer font-normal text-foreground"
                htmlFor={`${groupId}-${option.platform}`}
              >
                {option.platform}
              </Label>
            </div>
          ))}
        </RadioGroup>
      </li>
      {openStep && <li>{openStep}</li>}
      <li className="space-y-1">
        <p>
          Unzip the download into{" "}
          {folder ? (
            <code
              aria-label="KOReader plugins folder"
              className="rounded bg-muted px-1 text-foreground"
            >
              <FolderPath path={folder} />
            </code>
          ) : (
            "KOReader's plugins folder"
          )}
          , so a <code className="rounded bg-muted px-1">shisho.koplugin</code>{" "}
          folder sits inside it.
        </p>
        {folder?.startsWith(".") && (
          <p>
            Folders whose names start with a dot are hidden.{" "}
            <HiddenFolderHelp />
          </p>
        )}
      </li>
      <li>
        {restartStep(platform)} KOReader with Wi-Fi on, within 30 minutes of
        downloading. The plugin signs itself in, with no pairing code.
      </li>
      <li>
        Later than that, KOReader asks for a pairing code: get one under
        &quot;KOReader asking for a pairing code?&quot; below and enter it.
      </li>
    </ol>
  );
};

// FolderPath lets a long folder wrap only after a slash, never mid-name,
// even inside a name with a space, such as "Application Support".
const FolderPath = ({ path }: { path: string }) =>
  path.split("/").map((part, i) => (
    <Fragment key={i}>
      {i > 0 && (
        <>
          /<wbr />
        </>
      )}
      <span className="whitespace-nowrap">{part}</span>
    </Fragment>
  ));

// PluginStatus says when the Device last synced through the plugin, or,
// before its first sync, when it last connected. Once it syncs, Shisho
// ignores its progress sync, so it says to turn that off.
const PluginStatus = ({ device }: { device: DeviceResponse }) => {
  const version = device.plugin_version
    ? `, version ${device.plugin_version}`
    : "";
  if (device.plugin_last_synced_at) {
    return (
      <div className="space-y-1 text-sm">
        <p data-testid="plugin-status">
          {`Last synced through the plugin ${formatDateTime(device.plugin_last_synced_at)}${version}.`}
        </p>
        <p className="text-muted-foreground">
          Turn off progress sync in KOReader (Tools &gt; Progress sync). Shisho
          now ignores it from this Device, since the plugin reports the same
          reading sessions with their real time.
        </p>
      </div>
    );
  }
  return (
    <p className="text-sm" data-testid="plugin-status">
      {device.plugin_last_connected_at
        ? `Last connected through the plugin ${formatDateTime(device.plugin_last_connected_at)}${version}.`
        : "Not connected through the plugin yet."}
    </p>
  );
};

export default KOReaderPluginSetup;
