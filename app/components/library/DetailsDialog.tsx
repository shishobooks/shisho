import type { ReactNode } from "react";

import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export interface DetailRow {
  label: string;
  value: ReactNode;
}

interface DetailsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  rows: DetailRow[];
}

/**
 * A read-only dialog of labeled values: the details a curator checks now
 * and then, such as paths, dates, and identifiers, kept off the page so the
 * page leads with what a reader needs.
 */
const DetailsDialog = ({
  open,
  onOpenChange,
  title,
  rows,
}: DetailsDialogProps) => (
  <Dialog onOpenChange={onOpenChange} open={open}>
    <DialogContent className="max-w-2xl">
      <DialogHeader>
        <DialogTitle className="break-words">{title}</DialogTitle>
      </DialogHeader>
      <DialogBody>
        <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-6 gap-y-2 text-sm">
          {/* Keyed by position: a file can list two identifiers of a type. */}
          {rows.map((row, index) => (
            <div className="contents" key={index}>
              <dt className="text-muted-foreground">{row.label}</dt>
              <dd className="min-w-0 break-words">{row.value}</dd>
            </div>
          ))}
        </dl>
      </DialogBody>
    </DialogContent>
  </Dialog>
);

export default DetailsDialog;
