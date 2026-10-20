import { ArrowRight, Tablet } from "lucide-react";
import { Link } from "react-router-dom";

import KoboWarning from "@/components/devices/KoboWarning";
import QueryError from "@/components/library/QueryError";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
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
// the others: its label, and, when another File shares the label (as two
// editions named after the book do), the file name the Files list shows
// beneath it.
const editionLabel = (file: File, files: File[]): string => {
  const label = fileLabel(file);
  const shared = files.some((f) => f.id !== file.id && fileLabel(f) === label);
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
    <div className="flex min-w-0 items-center gap-2 sm:justify-end">
      {edition && (
        <span
          className="truncate text-xs text-muted-foreground"
          title={edition}
        >
          {edition}
        </span>
      )}
      {/* Fixed widths line the arrows up from row to row. */}
      <span
        aria-label={`${source} as ${delivered}`}
        className="flex shrink-0 items-center gap-1.5"
        role="img"
      >
        <Badge className="w-14 justify-center" variant="secondary">
          {source}
        </Badge>
        <ArrowRight
          aria-hidden="true"
          className="h-3.5 w-3.5 text-muted-foreground"
        />
        <Badge className="w-14 justify-center" variant="outline">
          {delivered}
        </Badge>
      </span>
    </div>
  );
};

// BookDevicesPanel lists each of the reader's Devices with what it gets of
// the book, or why it gets nothing, and warns when a Kobo's next sync would
// move the book on it to another of its Files or switch it to another File
// Type.
const BookDevicesPanel = ({ book }: BookDevicesPanelProps) => {
  // What a Device gets follows the book's Files, so a File added, removed,
  // or moved on this page fetches the panel again.
  const filesKey = (book.files ?? [])
    .map((f) => `${f.id}:${f.file_type}:${f.file_role}`)
    .join(",");
  const query = useBookDevices(book.id, filesKey);
  if (query.error && !query.data) {
    return (
      <>
        <Separator />
        <QueryError
          fallback="Failed to load what your Devices get"
          query={query}
        />
      </>
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
    <>
      <Separator />
      <div>
        <div className="mb-3 flex items-center justify-between">
          <h3 className="font-semibold">On your Devices</h3>
          <Button asChild size="sm" variant="ghost">
            <Link to="/user/devices">Manage</Link>
          </Button>
        </div>
        <ul className="space-y-1">
          {lines.map((line) => {
            const delivered = line.deliveries.length > 0;
            // Files tied on a rule each go, so name each one.
            const tied = line.deliveries.length > 1;
            return (
              <li
                aria-label={line.device_name}
                className="rounded-md px-2 py-2"
                key={line.device_id}
              >
                <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex min-w-0 items-center gap-3">
                    <Tablet
                      aria-hidden="true"
                      className={cn(
                        "h-4 w-4 shrink-0",
                        delivered ? "text-foreground" : "text-muted-foreground",
                      )}
                    />
                    <div className="min-w-0">
                      <Link
                        className={cn(
                          "block truncate text-sm font-medium hover:underline",
                          !delivered && "text-muted-foreground",
                        )}
                        title={line.device_name}
                        to={`/user/devices/${line.device_id}`}
                      >
                        {line.device_name}
                      </Link>
                      <p className="text-xs text-muted-foreground">
                        {DEVICE_TYPE_LABELS[line.device_type]}
                      </p>
                    </div>
                  </div>
                  <div className="flex min-w-0 flex-col gap-1.5 pl-7 sm:items-end sm:pl-0">
                    {delivered ? (
                      line.deliveries.map((d) => {
                        const file = fileByID(d.file_id);
                        return (
                          <Delivered
                            delivery={d}
                            edition={
                              tied && file
                                ? editionLabel(file, mainFiles)
                                : undefined
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
                  </div>
                </div>
                {(line.type_switch || line.edition_moves.length > 0) && (
                  <div className="mt-2 space-y-1.5 pl-7">
                    {line.type_switch && (
                      <KoboWarning>
                        Its next sync switches this book from{" "}
                        {fileTypeLabel(line.type_switch.from_file_type)} to{" "}
                        {fileTypeLabel(line.type_switch.to_file_type)}. The Kobo
                        adds it again as a new book, without its reading
                        position or highlights.
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
                )}
              </li>
            );
          })}
        </ul>
      </div>
    </>
  );
};

export default BookDevicesPanel;
