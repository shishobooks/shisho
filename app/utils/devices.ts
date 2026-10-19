import {
  ConnectionEReaderBrowser,
  ConnectionKoboSync,
  DeviceTypeBrowser,
  DeviceTypeKindle,
  DeviceTypeKobo,
  type Connection,
  type DeviceType,
} from "@/types/generated/devices";

export const DEVICE_TYPE_LABELS: Record<DeviceType, string> = {
  [DeviceTypeKobo]: "Kobo",
  [DeviceTypeKindle]: "Kindle",
  [DeviceTypeBrowser]: "Browser",
};

// The order the create dialog offers the types in, with what each one is for.
export const DEVICE_TYPE_OPTIONS: { type: DeviceType; description: string }[] =
  [
    {
      type: DeviceTypeKobo,
      description:
        "Kobo's own reader, through Kobo sync and the Kobo's web browser.",
    },
    {
      type: DeviceTypeKindle,
      description: "A Kindle's web browser.",
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
};

// The URL a Kobo's api_endpoint points at. It has no scope segment: the
// Device's scope decides what syncs.
export const koboSyncURL = (credential: string) =>
  `${window.location.origin}/kobo/${credential}`;

export const eReaderBrowserURL = (credential: string) =>
  `${window.location.origin}/ereader/key/${credential}/`;

export const shortURL = (shortCode: string) =>
  `${window.location.origin}/e/${shortCode}`;
