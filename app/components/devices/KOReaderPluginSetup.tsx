import { Download, KeyRound } from "lucide-react";
import { Fragment, useEffect, useState } from "react";

import Disclosure from "@/components/devices/Disclosure";
import { Button } from "@/components/ui/button";
import {
  useDownloadKOReaderPlugin,
  useGenerateDeviceShortUrl,
} from "@/hooks/queries/devices";
import { useAuth } from "@/hooks/useAuth";
import { toastRequestError } from "@/libraries/api";
import type { DeviceResponse, DeviceShortURL } from "@/types/generated/devices";
import { KOREADER_PLUGIN_FOLDERS } from "@/utils/devices";
import { formatDateTime, formatPlayerTime } from "@/utils/format";

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
 * KOReaderPluginSetup leads a KOReader Device's connections: downloading
 * and installing the KOReader plugin, pairing it, and when it last
 * connected.
 */
const KOReaderPluginSetup = ({ device }: { device: DeviceResponse }) => {
  const { demoMode } = useAuth();
  const download = useDownloadKOReaderPlugin();
  const pair = useGenerateDeviceShortUrl();
  const [short, setShort] = useState<DeviceShortURL | null>(null);

  const handleDownload = async () => {
    try {
      const blob = await download.mutateAsync(device.id);
      saveBlob(blob, PLUGIN_ZIP_NAME);
      // The download replaced any code shown here.
      setShort(null);
    } catch (error) {
      toastRequestError(error, "Failed to download the KOReader plugin");
    }
  };

  const handlePair = async () => {
    try {
      setShort(await pair.mutateAsync(device.id));
    } catch (error) {
      toastRequestError(error, "Failed to get a pairing code");
    }
  };

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        Recommended. The plugin signs in to Shisho once, sets up this server's
        OPDS catalog, syncs your reading time and position, and keeps itself up
        to date.
      </p>
      <PluginStatus device={device} />
      <div className="flex flex-wrap gap-2">
        {/* Demo Mode hides downloads. */}
        {!demoMode && (
          <Button
            disabled={download.isPending}
            onClick={handleDownload}
            size="sm"
          >
            <Download className="mr-2 h-4 w-4" />
            {download.isPending ? "Downloading..." : "Download Plugin"}
          </Button>
        )}
        <Button
          disabled={pair.isPending}
          onClick={handlePair}
          size="sm"
          variant="outline"
        >
          <KeyRound className="mr-2 h-4 w-4" />
          {pair.isPending ? "Pairing..." : "Pair"}
        </Button>
      </div>
      {short && <PairingCode key={short.short_code} short={short} />}
      {/* Once the plugin has connected, it's installed: the steps are there
          for a reinstall, out of the way. */}
      <Disclosure
        defaultOpen={!device.plugin_last_connected_at}
        size="sm"
        title="Install steps"
      >
        <ol
          aria-label="KOReader plugin setup"
          className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground"
        >
          <li>
            Connect the e-reader to your computer with USB, or on Android open a
            file manager.
          </li>
          <li>
            Unzip the download into KOReader's plugins folder, so a{" "}
            <code className="rounded bg-muted px-1">shisho.koplugin</code>{" "}
            folder sits inside it. The folder for each platform is below.
          </li>
          <li>
            Eject the e-reader and restart KOReader with Wi-Fi on, within 30
            minutes of downloading. The plugin signs itself in.
          </li>
          <li>
            Later than that, KOReader asks for a pairing code: press Pair and
            enter it.
          </li>
        </ol>
        <dl
          aria-label="KOReader plugins folders"
          className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm"
        >
          {KOREADER_PLUGIN_FOLDERS.map(({ platform, folder }) => (
            <div className="contents" key={platform}>
              <dt className="text-muted-foreground">{platform}</dt>
              <dd className="min-w-0">
                <code className="rounded bg-muted px-1">
                  <FolderPath path={folder} />
                </code>
              </dd>
            </div>
          ))}
        </dl>
      </Disclosure>
      <Disclosure size="sm" title="Why use the plugin?">
        <p className="text-sm text-muted-foreground">
          With the plugin, you sign in once with a pairing code instead of
          typing this Device's password into KOReader twice, the OPDS catalog is
          set up for you, and the plugin updates itself from Shisho. Your
          reading time comes from KOReader's own statistics, page by page, and
          reading done offline arrives on the days you did it, however long the
          e-reader stays offline. Progress sync is built into KOReader and syncs
          your position in the open book, but Shisho can only estimate your
          reading time from it, and offline reading is dated when the e-reader
          next connects. Once the plugin syncs, turn progress sync off.
        </p>
      </Disclosure>
    </div>
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
          reading with its real time.
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
