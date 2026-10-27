import type { ReactNode } from "react";

import type { LabelGridRow } from "@/components/library/LabelGrid";
import TextLink from "@/components/library/TextLink";
import { languageLabel } from "@/constants/languages";
import { FileTypeM4B, type File, type PluginIdentifierType } from "@/types";
import {
  fileTypeLabel,
  formatBitrate,
  formatDate,
  formatDateTime,
  formatDuration,
  formatFileSize,
  formatIdentifierType,
  formatPageCount,
  getFilename,
} from "@/utils/format";
import { getIdentifierUrl } from "@/utils/identifiers";

// An outside link that reads like the page's other links, underlined, not
// colored.
const externalLink = (href: string, children: ReactNode) => (
  <a
    className="underline decoration-foreground/25 underline-offset-4 transition-colors hover:decoration-foreground"
    href={href}
    rel="noopener noreferrer"
    target="_blank"
  >
    {children}
  </a>
);

/**
 * The quiet line under the file's name: length, the audio's codec and
 * bitrate, and size. The year is in the Released row.
 */
export const fileHeaderFacts = (file: File): string[] =>
  [
    file.audiobook_duration_seconds
      ? formatDuration(file.audiobook_duration_seconds)
      : null,
    file.page_count ? formatPageCount(file.page_count) : null,
    file.audiobook_codec ?? null,
    file.audiobook_bitrate_bps
      ? formatBitrate(file.audiobook_bitrate_bps)
      : null,
    formatFileSize(file.filesize_bytes),
  ].filter((fact): fact is string => fact !== null);

/**
 * What edition the file is: publisher, release date, language, abridged.
 * `linkPublisher: false` (a Share Link) names the publisher as plain text.
 */
export const fileEditionRows = (
  file: File,
  { linkPublisher = true }: { linkPublisher?: boolean } = {},
): LabelGridRow[] => {
  const rows: LabelGridRow[] = [];
  if (file.publisher) {
    rows.push({
      label: "Publisher",
      value: (
        <TextLink
          to={
            linkPublisher
              ? `/libraries/${file.library_id}/publishers/${file.publisher.id}`
              : null
          }
        >
          {file.publisher.name}
        </TextLink>
      ),
    });
  }
  if (file.release_date) {
    rows.push({ label: "Released", value: formatDate(file.release_date) });
  }
  if (file.language) {
    rows.push({ label: "Language", value: languageLabel(file.language) });
  }
  // Audiobooks were often sold abridged, so an M4B always says which it is;
  // another format says so only when it is.
  if (file.file_type === FileTypeM4B || file.abridged === true) {
    rows.push({
      label: "Abridged",
      value: file.abridged == null ? "Unknown" : file.abridged ? "Yes" : "No",
    });
  }
  return rows;
};

/** The file's identifiers, each linking out when its type has a page, and its URL. */
export const fileIdentifierRows = (
  file: File,
  pluginIdentifierTypes: PluginIdentifierType[] | undefined,
): LabelGridRow[] => {
  const rows: LabelGridRow[] = (file.identifiers ?? []).map((id) => {
    const url = getIdentifierUrl(id.type, id.value, pluginIdentifierTypes);
    const value = (
      <span className="select-all break-all font-mono text-[13px]">
        {id.value}
      </span>
    );
    return {
      label: formatIdentifierType(id.type, pluginIdentifierTypes),
      value: url ? externalLink(url, value) : value,
    };
  });
  if (file.url) {
    // The host says where it goes; the whole URL is the link.
    let host = file.url;
    try {
      host = new URL(file.url).host.replace(/^www\./, "");
    } catch {
      // Not a URL the browser can parse: show it as stored.
    }
    rows.push({
      label: "URL",
      value: externalLink(file.url, <span className="break-all">{host}</span>),
    });
  }
  return rows;
};

/** Where the file sits and its format's technical facts. */
export const fileDiskRows = (file: File): LabelGridRow[] => {
  const rows: LabelGridRow[] = [
    { label: "Format", value: fileTypeLabel(file.file_type) },
    { label: "Size", value: formatFileSize(file.filesize_bytes) },
  ];
  if (file.page_count) {
    rows.push({ label: "Pages", value: formatPageCount(file.page_count) });
  }
  if (file.audiobook_duration_seconds) {
    rows.push({
      label: "Duration",
      value: formatDuration(file.audiobook_duration_seconds),
    });
  }
  if (file.audiobook_codec) {
    rows.push({ label: "Codec", value: file.audiobook_codec });
  }
  if (file.audiobook_bitrate_bps) {
    rows.push({
      label: "Bitrate",
      value: formatBitrate(file.audiobook_bitrate_bps),
    });
  }
  const slash = file.filepath.lastIndexOf("/");
  rows.push({
    label: "File",
    value: <span className="break-all">{getFilename(file.filepath)}</span>,
  });
  rows.push({
    label: "Folder",
    value: (
      <span className="break-all text-muted-foreground">
        {slash > 0 ? file.filepath.slice(0, slash) : "/"}
      </span>
    ),
  });
  if (file.file_modified_at) {
    rows.push({
      label: "Modified",
      value: formatDateTime(file.file_modified_at),
    });
  }
  rows.push({ label: "Added", value: formatDateTime(file.created_at) });
  return rows;
};
