import { ArrowRight } from "lucide-react";
import { Link } from "react-router-dom";

import KoboWarning from "@/components/devices/KoboWarning";
import QueryError from "@/components/library/QueryError";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useBookDevices } from "@/hooks/queries/devices";
import { cn } from "@/libraries/utils";
import {
  NotDeliveredOutsideScope,
  type Book,
  type BookDelivery,
  type BookDevice,
  type File,
} from "@/types";
import { DEVICE_TYPE_LABELS } from "@/utils/devices";
import { fileLabel, fileTypeLabel, getFilename } from "@/utils/format";

interface BookDevicesPanelProps {
  book: Book;
}

// listOf joins words for a sentence: "EPUB", "EPUB or PDF", "EPUB, CBZ, or
// PDF".
const listOf = (words: string[]): string => {
  if (words.length <= 2) return words.join(" or ");
  return `${words.slice(0, -1).join(", ")}, or ${words[words.length - 1]}`;
};

// editionLabel names one of the book's Files so it can be told apart from
// the others: its label, and, only when another File of the same type
// shares it, its file name. An EPUB and an M4B named after the book, the
// common case, already differ by the source type each row shows.
const editionLabel = (file: File, files: File[]): string => {
  const label = fileLabel(file);
  const shared = files.some(
    (f) =>
      f.id !== file.id &&
      f.file_type === file.file_type &&
      fileLabel(f) === label,
  );
  return shared ? `${label} (${getFilename(file.filepath)})` : label;
};

// whyNot says why a Device gets nothing of the book.
const whyNot = (line: BookDevice, mainFiles: File[]): string => {
  if (line.not_delivered_reason === NotDeliveredOutsideScope) {
    return "Outside its scope";
  }
  const types = [...new Set(mainFiles.map((f) => fileTypeLabel(f.file_type)))];
  return types.length > 0
    ? `No rule for ${listOf(types)}`
    : "The book has no main files";
};

// Delivered shows one File a Device gets as its source and delivered File
// Types, the way the Delivery Rules editor shows a rule.
const Delivered = ({
  delivery,
  edition,
}: {
  delivery: BookDelivery;
  edition?: string;
}) => {
  const source = fileTypeLabel(delivery.source_file_type);
  const delivered = fileTypeLabel(delivery.delivered_file_type);
  return (
    <div className="flex min-w-0 items-center justify-between gap-3">
      {edition && (
        // Wraps rather than truncates: two editions often share a prefix,
        // and the end of the name is what tells them apart.
        <span className="min-w-0 break-words text-xs text-muted-foreground">
          {edition}
        </span>
      )}
      {/* Fixed widths line the arrows up from row to row. */}
      <span
        aria-label={`${source} as ${delivered}`}
        className="ml-auto flex shrink-0 items-center gap-1"
        role="img"
      >
        <Badge className="w-12 justify-center text-[10px]" variant="secondary">
          {source}
        </Badge>
        <ArrowRight
          aria-hidden="true"
          className="size-3 text-muted-foreground"
        />
        <Badge className="w-12 justify-center text-[10px]" variant="outline">
          {delivered}
        </Badge>
      </span>
    </div>
  );
};

// BookDevicesPanel lists each of the reader's Devices with what it gets of
// the book, or why it gets nothing, and warns when a Kobo's next sync would
// move the book on it to another of its Files or switch it to another File
// Type. It fills the Devices tab of the book page's Related panel, so it
// stacks to fit a narrow column at every screen width.
const BookDevicesPanel = ({ book }: BookDevicesPanelProps) => {
  const query = useBookDevices(book);
  if (query.error && !query.data) {
    return (
      <QueryError
        fallback="Failed to load what your Devices get"
        query={query}
      />
    );
  }
  // Devices that get the book come first, in the server's order.
  const lines = [...(query.data ?? [])].sort(
    (a, b) => Number(b.deliveries.length > 0) - Number(a.deliveries.length > 0),
  );
  if (lines.length === 0) return null;
  const mainFiles = (book.files ?? []).filter(
    (f) => f.file_role !== "supplement",
  );
  const fileByID = (id: number | null | undefined) =>
    mainFiles.find((f) => f.id === id);

  return (
    <div>
      <ul className="divide-y divide-border">
        {lines.map((line) => {
          const delivered = line.deliveries.length > 0;
          return (
            <li
              aria-label={line.device_name}
              className="py-2.5 first:pt-1"
              key={line.device_id}
            >
              <div className="flex min-w-0 items-baseline justify-between gap-3">
                <Link
                  className={cn(
                    "min-w-0 truncate text-sm font-medium hover:underline",
                    !delivered && "text-muted-foreground",
                  )}
                  title={line.device_name}
                  to={`/user/devices/${line.device_id}`}
                >
                  {line.device_name}
                </Link>
                <span className="shrink-0 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
                  {DEVICE_TYPE_LABELS[line.device_type]}
                </span>
              </div>
              <div className="mt-1 space-y-1">
                {delivered ? (
                  line.deliveries.map((d) => {
                    const file = fileByID(d.file_id);
                    return (
                      <Delivered
                        delivery={d}
                        // Named even when it is the only one, so every
                        // delivery reads as a line, not a lone pill.
                        edition={
                          file ? editionLabel(file, mainFiles) : undefined
                        }
                        key={d.file_id}
                      />
                    );
                  })
                ) : (
                  <p className="text-xs text-muted-foreground">
                    Not delivered · {whyNot(line, mainFiles)}
                  </p>
                )}
                {line.type_switch && (
                  <KoboWarning>
                    Its next sync switches this book from{" "}
                    {fileTypeLabel(line.type_switch.from_file_type)} to{" "}
                    {fileTypeLabel(line.type_switch.to_file_type)}. The Kobo
                    adds it again as a new book, without its reading position or
                    highlights.
                  </KoboWarning>
                )}
                {line.edition_moves.map((move) => {
                  const to = fileByID(move.to_file_id);
                  return (
                    <KoboWarning
                      key={`${move.from_file_id}-${move.to_file_id}`}
                    >
                      {`Its next sync moves this book to a different edition${to ? `, ${editionLabel(to, mainFiles)}` : ""}, which may reopen at the start.`}
                    </KoboWarning>
                  );
                })}
              </div>
            </li>
          );
        })}
      </ul>
      <Button asChild className="mt-2 text-sm" variant="link">
        <Link to="/user/devices">Manage Devices</Link>
      </Button>
    </div>
  );
};

export default BookDevicesPanel;
