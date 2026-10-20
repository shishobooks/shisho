import { AlertTriangle } from "lucide-react";
import { Link } from "react-router-dom";

import QueryError from "@/components/library/QueryError";
import { Separator } from "@/components/ui/separator";
import { useBookDevices } from "@/hooks/queries/devices";
import {
  NotDeliveredOutsideScope,
  type Book,
  type BookDevice,
  type File,
} from "@/types";
import { fileLabel, fileTypeLabel } from "@/utils/format";

interface BookDevicesPanelProps {
  book: Book;
}

// listOf joins words for a sentence: "EPUB", "EPUB or PDF", "EPUB, CBZ, or
// PDF".
const listOf = (words: string[]): string => {
  if (words.length <= 2) return words.join(" or ");
  return `${words.slice(0, -1).join(", ")}, or ${words[words.length - 1]}`;
};

// whatItGets says what a Device gets of the book, or why it gets nothing,
// such as "as KePub (from EPUB)" or "not delivered, no rule for EPUB".
const whatItGets = (line: BookDevice, mainFiles: File[]): string[] => {
  if (line.not_delivered_reason === NotDeliveredOutsideScope) {
    return ["not delivered, outside its scope"];
  }
  if (line.deliveries.length === 0) {
    const types = [
      ...new Set(mainFiles.map((f) => fileTypeLabel(f.file_type))),
    ];
    return types.length > 0
      ? [`not delivered, no rule for ${listOf(types)}`]
      : ["not delivered, the book has no files"];
  }
  const tied = line.deliveries.length > 1;
  return line.deliveries.map((d) => {
    const how = `as ${fileTypeLabel(d.delivered_file_type)} (from ${fileTypeLabel(d.source_file_type)})`;
    const file = mainFiles.find((f) => f.id === d.file_id);
    // Files tied on a rule each go, so name each one.
    return tied && file ? `${how}: ${fileLabel(file)}` : how;
  });
};

// BookDevicesPanel lists each of the reader's Devices with what it gets of
// the book and why, and warns when a Kobo's next sync would move the book
// on it to another of its Files.
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
  const lines = query.data ?? [];
  if (lines.length === 0) return null;
  const mainFiles = (book.files ?? []).filter(
    (f) => f.file_role !== "supplement",
  );

  return (
    <>
      <Separator />
      <div>
        <h3 className="font-semibold mb-3">On your Devices</h3>
        <ul className="space-y-2 text-sm">
          {lines.map((line) => (
            <li key={line.device_id}>
              {whatItGets(line, mainFiles).map((text) => (
                <p key={text}>
                  <Link
                    className="font-medium hover:underline"
                    to={`/user/devices/${line.device_id}`}
                  >
                    {line.device_name}
                  </Link>
                  <span className="text-muted-foreground">: {text}</span>
                </p>
              ))}
              {line.edition_moves.map((move) => {
                const to = mainFiles.find((f) => f.id === move.to_file_id);
                return (
                  <p
                    className="mt-1 flex gap-2 text-amber-700 dark:text-amber-400"
                    key={`${move.from_file_id}-${move.to_file_id}`}
                  >
                    <AlertTriangle
                      aria-hidden="true"
                      className="mt-0.5 h-4 w-4 shrink-0"
                    />
                    {`Its next sync moves this book to a different edition${to ? ` (${fileLabel(to)})` : ""}, which may reopen at the start.`}
                  </p>
                );
              })}
            </li>
          ))}
        </ul>
      </div>
    </>
  );
};

export default BookDevicesPanel;
