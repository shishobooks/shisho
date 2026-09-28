import { Ban, Copy, Info, Loader2, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useBookShareLinks,
  useCreateShareLink,
  useDeleteShareLink,
  useRevokeShareLink,
} from "@/hooks/queries/sharing";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import {
  ShareLinkStateActive,
  ShareLinkStateExpired,
  type ShareLinkResponse,
} from "@/types";
import { copyText } from "@/utils/clipboard";
import { formatDateTime } from "@/utils/format";

const DAY_MS = 24 * 60 * 60 * 1000;
const NEVER = "never";
const DEFAULT_PRESET = "7";

// Expiration presets in days. Never is appended when the admin allows it.
const PRESETS = [
  { value: "1", label: "1 day" },
  { value: "7", label: "7 days" },
  { value: "30", label: "30 days" },
];

// The recipient URL uses the origin the sharer is browsing on, so it works for
// the recipient the same way it works for the sharer.
const shareLinkUrl = (token: string) =>
  `${window.location.origin}/share/${token}`;

const expiryText = (link: ShareLinkResponse) => {
  if (!link.expires_at) return "Never expires";
  const when = formatDateTime(link.expires_at);
  return link.state === ShareLinkStateExpired
    ? `Expired ${when}`
    : `Expires ${when}`;
};

const plural = (count: number, noun: string) =>
  `${count} ${noun}${count === 1 ? "" : "s"}`;

// Opens count recipient page loads and downloads count file downloads, so a
// download is definite proof the recipient got the file.
const usageText = (link: ShareLinkResponse) =>
  [
    plural(link.open_count, "open"),
    plural(link.download_count, "download"),
    link.last_accessed_at
      ? `Last used ${formatDateTime(link.last_accessed_at)}`
      : "Never used",
  ].join(" · ");

interface ShareLinkDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bookId: number;
  bookTitle: string;
  /** Shares Write: show the new-link form and the revoke and delete actions. */
  canWrite: boolean;
  /** Shares Read: show the book's links. */
  canList: boolean;
  /** The admin requires every link to expire, so Never is not offered. */
  requireExpiration: boolean;
}

export function ShareLinkDialog({
  open,
  onOpenChange,
  bookId,
  bookTitle,
  canWrite,
  canList,
  requireExpiration,
}: ShareLinkDialogProps) {
  const [label, setLabel] = useState("");
  const [preset, setPreset] = useState(DEFAULT_PRESET);

  // A closed dialog starts over, including after discarding a draft.
  useEffect(() => {
    if (!open) {
      setLabel("");
      setPreset(DEFAULT_PRESET);
    }
  }, [open]);
  const hasChanges = label.trim() !== "" || preset !== DEFAULT_PRESET;

  const linksQuery = useBookShareLinks(bookId, { enabled: open && canList });
  const createMutation = useCreateShareLink();
  const revokeMutation = useRevokeShareLink();
  const deleteMutation = useDeleteShareLink();
  // The target outlives its confirmation's open flag so the text does not
  // change while the confirmation animates closed.
  const [toRevoke, setToRevoke] = useState<ShareLinkResponse | null>(null);
  const [revokeOpen, setRevokeOpen] = useState(false);
  const [toDelete, setToDelete] = useState<ShareLinkResponse | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const presets = requireExpiration
    ? PRESETS
    : [...PRESETS, { value: NEVER, label: "Never" }];
  // Never can disappear if the admin changes the policy while it is selected.
  const selectedPreset = presets.some((p) => p.value === preset)
    ? preset
    : DEFAULT_PRESET;

  const handleCopy = async (token: string) => {
    if (await copyText(shareLinkUrl(token))) {
      toast.success("Copied to clipboard");
    } else {
      toast.error("Could not copy the link");
    }
  };

  const handleCreate = async () => {
    const trimmed = label.trim();
    try {
      await createMutation.mutateAsync({
        bookId,
        payload: {
          label: trimmed || undefined,
          expires_at:
            selectedPreset === NEVER
              ? undefined
              : new Date(
                  Date.now() + Number(selectedPreset) * DAY_MS,
                ).toISOString(),
        },
      });
      toast.success("Share link created");
      setLabel("");
      setPreset(DEFAULT_PRESET);
    } catch (error) {
      toastRequestError(
        error,
        error instanceof Error ? error.message : "Failed to create share link",
      );
    }
  };

  const handleRevoke = async () => {
    if (!toRevoke) return;
    try {
      await revokeMutation.mutateAsync({ bookId, linkId: toRevoke.id });
      toast.success("Share link revoked");
    } catch (error) {
      toastRequestError(
        error,
        error instanceof Error ? error.message : "Failed to revoke share link",
      );
    } finally {
      // The list refetches after either outcome, so a link another sharer
      // already removed drops out instead of holding the confirmation open.
      setRevokeOpen(false);
    }
  };

  const handleDelete = async () => {
    if (!toDelete) return;
    try {
      await deleteMutation.mutateAsync({ bookId, linkId: toDelete.id });
      toast.success("Share link deleted");
    } catch (error) {
      toastRequestError(
        error,
        error instanceof Error ? error.message : "Failed to delete share link",
      );
    } finally {
      setDeleteOpen(false);
    }
  };

  const links = linksQuery.data ?? [];

  return (
    <>
      <FormDialog
        hasChanges={hasChanges}
        onOpenChange={onOpenChange}
        open={open}
      >
        <DialogContent className="max-w-lg overflow-x-hidden">
          <DialogHeader className="pr-8">
            <DialogTitle>Share</DialogTitle>
            <DialogDescription>
              Anyone with a link can view and download "{bookTitle}" without an
              account.
            </DialogDescription>
          </DialogHeader>

          <DialogBody className="space-y-6">
            {canWrite && (
              <div className="space-y-3">
                <h3 className="text-sm font-medium">New Link</h3>
                <div className="space-y-2">
                  <Label htmlFor="share-link-label">Label (optional)</Label>
                  <Input
                    id="share-link-label"
                    maxLength={200}
                    onChange={(e) => setLabel(e.target.value)}
                    placeholder="e.g. for Alice"
                    value={label}
                  />
                </div>
                <div className="flex flex-col sm:flex-row sm:items-end gap-2">
                  <div className="space-y-2 flex-1">
                    <Label htmlFor="share-link-expiration">Expires after</Label>
                    <Select onValueChange={setPreset} value={selectedPreset}>
                      <SelectTrigger id="share-link-expiration">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {presets.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {option.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <Button
                    disabled={createMutation.isPending}
                    onClick={handleCreate}
                  >
                    {createMutation.isPending && (
                      <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                    )}
                    Create link
                  </Button>
                </div>
              </div>
            )}

            {canList && (
              <div className="space-y-3">
                <h3 className="text-sm font-medium">Links</h3>
                {linksQuery.isLoading ? (
                  <div className="flex items-center justify-center py-4">
                    <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                  </div>
                ) : links.length === 0 ? (
                  <p className="text-sm text-muted-foreground py-2">
                    This book has no share links yet.
                  </p>
                ) : (
                  <ul className="space-y-2">
                    {links.map((link) => {
                      // An inactive link keeps its row but recedes: no fill, a
                      // muted label, and the muted status badge.
                      const active = link.state === ShareLinkStateActive;
                      return (
                        <li
                          className={cn(
                            "flex items-center justify-between gap-2 py-2 px-3 rounded-md border",
                            active && "bg-muted/50",
                          )}
                          key={link.id}
                        >
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center gap-2">
                              <span
                                className={cn(
                                  "font-medium truncate",
                                  (!link.label || !active) &&
                                    "text-muted-foreground",
                                  !link.label && "italic",
                                )}
                                title={link.label}
                              >
                                {link.label || "No label"}
                              </span>
                              <Badge
                                className={cn(
                                  "capitalize",
                                  !active &&
                                    "border-transparent bg-muted text-muted-foreground",
                                )}
                                variant={active ? "success" : "outline"}
                              >
                                {link.state}
                              </Badge>
                            </div>
                            <p className="text-xs text-muted-foreground">
                              Created by {link.created_by_username} ·{" "}
                              {expiryText(link)}
                            </p>
                            <p className="text-xs text-muted-foreground">
                              {usageText(link)}
                            </p>
                          </div>
                          <div className="flex items-center shrink-0">
                            <Button
                              aria-label={`Copy link ${link.label || ""}`.trim()}
                              className="h-8 w-8"
                              disabled={!active}
                              onClick={() => handleCopy(link.token)}
                              size="icon"
                              title="Copy link"
                              variant="ghost"
                            >
                              <Copy className="h-4 w-4" />
                            </Button>
                            {canWrite && active && (
                              <Button
                                aria-label={`Revoke ${link.label || ""}`.trim()}
                                className="h-8 w-8"
                                onClick={() => {
                                  setToRevoke(link);
                                  setRevokeOpen(true);
                                }}
                                size="icon"
                                title="Revoke"
                                variant="ghost"
                              >
                                <Ban className="h-4 w-4" />
                              </Button>
                            )}
                            {canWrite && (
                              <Button
                                aria-label={`Delete ${link.label || ""}`.trim()}
                                className="h-8 w-8 text-muted-foreground hover:text-destructive"
                                onClick={() => {
                                  setToDelete(link);
                                  setDeleteOpen(true);
                                }}
                                size="icon"
                                title="Delete"
                                variant="ghost"
                              >
                                <Trash2 className="h-4 w-4" />
                              </Button>
                            )}
                          </div>
                        </li>
                      );
                    })}
                  </ul>
                )}
              </div>
            )}

            <div className="flex items-start gap-2 p-3 rounded-md bg-muted/50 text-sm text-muted-foreground">
              <Info className="h-4 w-4 mt-0.5 shrink-0" />
              <p>
                Recipients see the book's cover, details, and files, and who
                shared it. The link must be opened from a network that can reach
                this server.
              </p>
            </div>
          </DialogBody>
        </DialogContent>
      </FormDialog>

      <ConfirmDialog
        confirmLabel="Revoke"
        description="The link stops working immediately and stays in the list as revoked, with its counts. This cannot be undone; create a new link to share the book again."
        isPending={revokeMutation.isPending}
        onConfirm={handleRevoke}
        onOpenChange={setRevokeOpen}
        open={revokeOpen}
        title="Revoke Link"
      />
      <ConfirmDialog
        confirmLabel="Delete"
        description={
          toDelete?.state === ShareLinkStateActive
            ? "The link stops working immediately and is removed from the list with its counts. This cannot be undone."
            : "The link is removed from the list with its counts. This cannot be undone."
        }
        isPending={deleteMutation.isPending}
        onConfirm={handleDelete}
        onOpenChange={setDeleteOpen}
        open={deleteOpen}
        title="Delete Link"
      />
    </>
  );
}
