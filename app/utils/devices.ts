import { DOCS_URL } from "@/constants/docs";
import {
  ConnectionEReaderBrowser,
  ConnectionKoboSync,
  ConnectionKOReaderPlugin,
  ConnectionKOReaderSync,
  ConnectionOPDS,
  DeviceTypeBrowser,
  DeviceTypeKindle,
  DeviceTypeKobo,
  DeviceTypeKOReader,
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
  [DeviceTypeKOReader]: "KOReader",
  [DeviceTypeOPDS]: "OPDS app",
  [DeviceTypeBrowser]: "Browser",
};

// The order Change Type offers the types in, with what each one is for.
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
      type: DeviceTypeKOReader,
      description:
        "KOReader on any device. Signs in through the KOReader plugin, browses over OPDS, and syncs your reading position.",
    },
    {
      type: DeviceTypeOPDS,
      description:
        "Another reading app that browses an OPDS catalog, such as Thorium, Panels, etc.",
    },
    {
      type: DeviceTypeBrowser,
      description:
        "Any other e-reader's web browser, such as a Boox or PocketBook.",
    },
  ];

/** One answer to "What do you read on?" in the Connect a device flow. */
export interface ConnectChoice {
  type: DeviceType;
  label: string;
  description: string;
  /** The name a new Device starts with. */
  name: string;
  /** The troubleshooting docs for this way of connecting. */
  troubleshootingURL: string;
}

// What the Connect a device flow offers, in plain words, each mapped to the
// Device type it creates.
export const CONNECT_CHOICES: ConnectChoice[] = [
  {
    type: DeviceTypeKobo,
    label: "A Kobo",
    description:
      "Kobo's own built-in reader. Books sync to it over Wi-Fi. Using KOReader on your Kobo? Choose KOReader.",
    name: "Kobo",
    troubleshootingURL: `${DOCS_URL}/kobo-sync#troubleshooting`,
  },
  {
    type: DeviceTypeKOReader,
    label: "KOReader",
    description: "On a Kobo, Kindle, Android, PocketBook, and others.",
    name: "KOReader",
    troubleshootingURL: `${DOCS_URL}/koreader#troubleshooting`,
  },
  {
    type: DeviceTypeKindle,
    label: "A Kindle's browser",
    description: "Download books in the Kindle's web browser.",
    name: "Kindle",
    troubleshootingURL: `${DOCS_URL}/ereader-browser#troubleshooting`,
  },
  {
    type: DeviceTypeOPDS,
    label: "A reading app on a phone or tablet",
    description:
      "An app that can add an OPDS catalog, such as Panels or Thorium.",
    name: "Reading app",
    troubleshootingURL: `${DOCS_URL}/opds#troubleshooting`,
  },
  {
    type: DeviceTypeBrowser,
    label: "Another e-reader's browser",
    description:
      "Download books in the web browser of a Boox, PocketBook, or other e-reader.",
    name: "E-reader",
    troubleshootingURL: `${DOCS_URL}/ereader-browser#troubleshooting`,
  },
];

/** The Connect a device choice that creates a Device of type. */
export const connectChoiceFor = (type: DeviceType): ConnectChoice =>
  CONNECT_CHOICES.find((choice) => choice.type === type) ?? CONNECT_CHOICES[0];

// uniqueName returns name, or name with the first number after it that no
// Device in taken uses, so a second Kobo starts as "Kobo 2".
export const uniqueName = (name: string, taken: string[]): string => {
  const used = new Set(taken.map((t) => t.toLowerCase()));
  if (!used.has(name.toLowerCase())) return name;
  let n = 2;
  while (used.has(`${name} ${n}`.toLowerCase())) n++;
  return `${name} ${n}`;
};

export const CONNECTION_LABELS: Record<Connection, string> = {
  [ConnectionKoboSync]: "Kobo sync",
  [ConnectionEReaderBrowser]: "eReader browser",
  [ConnectionOPDS]: "OPDS",
  [ConnectionKOReaderSync]: "KOReader progress sync",
  [ConnectionKOReaderPlugin]: "KOReader plugin",
};

// Where to unzip the KOReader plugin on each platform KOReader runs on, as
// the device shows its storage over USB or in a file manager. The setup
// asks which one KOReader runs on and shows only its folder.
export const KOREADER_PLUGIN_FOLDERS: { platform: string; folder: string }[] = [
  { platform: "Kobo", folder: ".adds/koreader/plugins" },
  { platform: "Kindle", folder: "koreader/plugins" },
  { platform: "PocketBook", folder: "applications/koreader/plugins" },
  { platform: "Android", folder: "koreader/plugins" },
  { platform: "Linux", folder: "~/.config/koreader/plugins" },
  {
    platform: "macOS",
    folder: "~/Library/Application Support/koreader/plugins",
  },
];

// The URL a Kobo's api_endpoint points at. It has no scope segment: the
// Device's scope decides what syncs.
export const koboSyncURL = (credential: string) =>
  `${window.location.origin}/kobo/${credential}`;

export const eReaderBrowserURL = (credential: string) =>
  `${window.location.origin}/ereader/key/${credential}/`;

// The catalog URL an OPDS app signs in to with the user's username and the
// Device's credential.
export const opdsCatalogURL = () => `${window.location.origin}/opds/v1/catalog`;

// What KOReader's progress sync takes as its custom sync server.
export const koreaderSyncURL = () => `${window.location.origin}/koreader`;

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
