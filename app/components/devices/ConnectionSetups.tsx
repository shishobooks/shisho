import { AlertTriangle } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import HiddenFolderHelp from "@/components/devices/HiddenFolderHelp";
import CopyField from "@/components/library/CopyField";
import { Button } from "@/components/ui/button";
import { DOCS_URL } from "@/constants/docs";
import {
  useClearKoboSync,
  useGenerateDeviceShortUrl,
} from "@/hooks/queries/devices";
import { useAuth } from "@/hooks/useAuth";
import { toastRequestError } from "@/libraries/api";
import {
  type DeviceResponse,
  type DeviceShortURL,
} from "@/types/generated/devices";
import {
  eReaderBrowserURL,
  koboSyncURL,
  koreaderSyncURL,
  opdsCatalogURL,
  shortURL,
} from "@/utils/devices";
import { isLoopbackHost, pageHostname } from "@/utils/location";

/**
 * LoopbackWarning says, while the page is open at localhost or another
 * loopback address, that a device can't reach the URLs the setup steps give.
 */
export const LoopbackWarning = () => {
  const loopback = isLoopbackHost(pageHostname());
  if (!loopback) return null;
  return (
    <div
      className="flex gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-400"
      role="note"
    >
      <AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
      <p>
        Your device can't reach{" "}
        <code className="break-all font-mono">{pageHostname()}</code>. Open
        Shisho at the address your devices use and set up the device from there.
      </p>
    </div>
  );
};

const Code = ({ children }: { children: React.ReactNode }) => (
  <code className="break-all rounded bg-muted px-1">{children}</code>
);

/** The link to a method's troubleshooting docs. */
export const NotWorkingLink = ({
  href,
  children,
}: {
  href: string;
  children: React.ReactNode;
}) => (
  <a
    className="underline underline-offset-2 hover:text-foreground"
    href={href}
    rel="noopener noreferrer"
    target="_blank"
  >
    {children}
  </a>
);

export const KoboSyncSetup = ({ device }: { device: DeviceResponse }) => {
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
      <CopyField
        label="Kobo sync URL"
        url
        value={koboSyncURL(device.credential)}
      />
      <ol
        aria-label="Kobo sync setup"
        className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground"
      >
        <li>Connect your Kobo to your computer with USB.</li>
        <li>
          On the Kobo&apos;s drive, open the <Code>.kobo</Code> folder.
          It&apos;s hidden. <HiddenFolderHelp />
        </li>
        <li>
          Open <Code>Kobo/Kobo eReader.conf</Code> in a text editor.
        </li>
        <li>
          Find <Code>api_endpoint=https://storeapi.kobo.com</Code> and replace
          the URL with the one above.
        </li>
        <li>
          Delete the <Code>image_host</Code>, <Code>image_url_template</Code>,
          and <Code>image_url_quality_template</Code> lines if they are there,
          so covers load from Shisho.
        </li>
        <li>Save the file, eject the Kobo, and sync.</li>
      </ol>
      <p className="text-sm text-muted-foreground">
        Not working? See{" "}
        <NotWorkingLink href={`${DOCS_URL}/kobo-sync#troubleshooting`}>
          Kobo sync troubleshooting
        </NotWorkingLink>
        .
      </p>
      {/* Only a Kobo that has synced has a history to clear. */}
      {device.last_accessed_at && (
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
      )}
    </div>
  );
};

export const EReaderBrowserSetup = ({ device }: { device: DeviceResponse }) => {
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
      <ol
        aria-label="eReader browser setup"
        className="list-decimal space-y-3 pl-5 text-sm text-muted-foreground"
      >
        <li className="space-y-3">
          <p>Get a setup URL. It&apos;s short, so it&apos;s easy to type.</p>
          {short && (
            <>
              <CopyField
                label="Setup URL"
                url
                value={shortURL(short.short_code)}
              />
              <p>
                It expires in 30 minutes, or when you get a new one. The page it
                opens keeps working.
              </p>
            </>
          )}
          <Button
            disabled={generateShortUrl.isPending}
            onClick={handleGenerate}
            size="sm"
            variant={short ? "outline" : "default"}
          >
            {generateShortUrl.isPending
              ? "Generating..."
              : short
                ? "Get New Setup URL"
                : "Get Setup URL"}
          </Button>
        </li>
        <li>Open the e-reader&apos;s web browser and go to the setup URL.</li>
        <li>Bookmark the page it opens, to come back to your books.</li>
      </ol>
      <div className="space-y-2">
        <p className="text-sm font-medium">Or bookmark this URL</p>
        <CopyField
          label="eReader browser URL"
          url
          value={eReaderBrowserURL(device.credential)}
        />
      </div>
    </div>
  );
};

export const OPDSSetup = ({ device }: { device: DeviceResponse }) => {
  const { user } = useAuth();
  return (
    <div className="space-y-4">
      <ol
        aria-label="OPDS setup"
        className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground"
      >
        <li>
          In the reading app, add an OPDS catalog. Some apps call it a library,
          a server, or a feed.
        </li>
        <li>Enter this URL, your username, and this Device&apos;s password.</li>
      </ol>
      <CopyField label="OPDS URL" url value={opdsCatalogURL()} />
      {user && <CopyField label="Username" value={user.username} />}
      <CopyField label="Password" value={device.credential} />
      <p className="text-sm text-muted-foreground">
        Your account password signs in too, but then the app gets books the way
        your default Delivery Rules say, not this Device&apos;s.
      </p>
    </div>
  );
};

export const KOReaderSyncSetup = ({ device }: { device: DeviceResponse }) => {
  const { user } = useAuth();
  return (
    <div className="space-y-4">
      <CopyField label="Custom sync server" url value={koreaderSyncURL()} />
      {user && <CopyField label="Sync username" value={user.username} />}
      <CopyField label="Sync password" value={device.credential} />
      <ol
        aria-label="KOReader progress sync setup"
        className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground"
      >
        <li>
          Open any book in KOReader, go to Tools &gt; Progress sync &gt; Custom
          sync server, and enter the server above.
        </li>
        <li>
          Choose Register / Login, enter the username and password above, and
          tap Login. Register doesn&apos;t work here: this Device is your
          account.
        </li>
        <li>
          Turn on Automatically keep documents in sync, so your position syncs
          when a book opens or closes and when the device sleeps or wakes.
          KOReader turns on Wi-Fi to sync, which uses some battery. On an
          e-reader, first set Settings &gt; Network &gt; Action when Wi-Fi is
          off to Turn on.
        </li>
        <li>Keep Document matching method on Binary, the default.</li>
      </ol>
      <p className="text-sm text-muted-foreground">
        This is a one-time setup: it applies to every book in KOReader. See the{" "}
        <NotWorkingLink href={`${DOCS_URL}/koreader`}>
          KOReader setup guide
        </NotWorkingLink>{" "}
        for which books sync and how positions move between readers.
      </p>
    </div>
  );
};
