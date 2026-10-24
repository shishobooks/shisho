import {
  ConnectionEReaderBrowser,
  ConnectionKoboSync,
  ConnectionOPDS,
  DeviceTypeBrowser,
  DeviceTypeKindle,
  DeviceTypeKobo,
  DeviceTypeOPDS,
  ScopeLibrary,
  ScopeList,
  type Connection,
  type DeviceResponse,
  type DeviceType,
} from "@/types/generated/devices";
import type { BookDevice } from "@/types/generated/reach";

export const DEVICE_TYPE_LABELS: Record<DeviceType, string> = {
  [DeviceTypeKobo]: "Kobo",
  [DeviceTypeKindle]: "Kindle",
  [DeviceTypeOPDS]: "OPDS app",
  [DeviceTypeBrowser]: "Browser",
};

// The order the create dialog offers the types in, with what each one is for.
export const DEVICE_TYPE_OPTIONS: { type: DeviceType; description: string }[] =
  [
    {
      type: DeviceTypeKobo,
      description:
        "A Kobo's built-in reader. Syncs over Wi-Fi and can also browse from the Kobo's web browser.",
    },
    {
      type: DeviceTypeKindle,
      description: "A Kindle's web browser.",
    },
    {
      type: DeviceTypeOPDS,
      description:
        "A reading app that browses an OPDS catalog, such as KOReader, Thorium, Panels, etc.",
    },
    {
      type: DeviceTypeBrowser,
      description:
        "Any other e-reader's web browser, such as a Boox or PocketBook.",
    },
  ];

export const CONNECTION_LABELS: Record<Connection, string> = {
  [ConnectionKoboSync]: "Kobo sync",
  [ConnectionEReaderBrowser]: "eReader browser",
  [ConnectionOPDS]: "OPDS",
};

// The URL a Kobo's api_endpoint points at. It has no scope segment: the
// Device's scope decides what syncs.
export const koboSyncURL = (credential: string) =>
  `${window.location.origin}/kobo/${credential}`;

export const eReaderBrowserURL = (credential: string) =>
  `${window.location.origin}/ereader/key/${credential}/`;

// The catalog URL an OPDS app signs in to with the user's username and the
// Device's credential.
export const opdsCatalogURL = () => `${window.location.origin}/opds/v1/catalog`;

export const shortURL = (shortCode: string) =>
  `${window.location.origin}/e/${shortCode}`;

// scopeLabel describes which books a Device reaches, for lists of Devices.
export const scopeLabel = (device: DeviceResponse): string => {
  switch (device.scope_type) {
    case ScopeLibrary:
      return device.scope_name
        ? `Library: ${device.scope_name}`
        : "Library: deleted, reaches no books";
    case ScopeList:
      return device.scope_name
        ? `List: ${device.scope_name}`
        : "List: unavailable, reaches no books";
    default:
      return "All books";
  }
};

/** Whether a Kobo's next sync would change the book on it. */
export const deviceHasWarning = (line: BookDevice): boolean =>
  Boolean(line.type_switch) || line.edition_moves.length > 0;
