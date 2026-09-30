import { Ban, Copy, Info, Loader2, PowerOff, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
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
  ShareLinkPausedCreatorDeactivated,
  ShareLinkStateActive,
  ShareLinkStateExpired,
  ShareLinkStateRevoked,
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

// A revoked link shows when it was pulled rather than an expiry that no
// longer matters.
const expiryText = (link: ShareLinkResponse) => {
  if (link.state === ShareLinkStateRevoked && link.revoked_at) {
    return `Revoked ${formatDateTime(link.revoked_at)}`;
  }
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

const pausedText = (link: ShareLinkResponse) =>
  link.paused_reason === ShareLinkPausedCreatorDeactivated
    ? `Paused because ${link.created_by_username} was deactivated`
    : `Paused because ${link.created_by_username} no longer has access to this library`;

// The confirmation names the link, since on a phone it covers the row.
const linkSubject = (link: ShareLinkResponse | null) => {
  if (!link) return "The link";
  return link.label
    ? `The link "${link.label}"`
    : `The unlabeled link created by ${link.created_by_username}`;
};

// A link that already does not work (paused, or sharing off) is told it will
// not come back, rather than that it stops now.
const revokeDescription = (
  link: ShareLinkResponse | null,
  sharingEnabled: boolean,
) => {
  const subject = linkSubject(link);
  const tail =
    "It stays in the list as revoked, with its counts. This cannot be undone; create a new link to share the book again.";
  if (link?.paused_reason) {
    return `${subject} is paused and will not work again when its creator's access returns. ${tail}`;
  }
  if (!sharingEnabled) {
    return `${subject} will not work again when sharing is turned back on. ${tail}`;
  }
  return `${subject} stops working immediately and stays in the list as revoked, with its counts. This cannot be undone; create a new link to share the book again.`;
};

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
  /**
   * Sharing is on. While it is off no link works and none can be created,
   * but links can still be revoked and deleted so an admin never has to turn
   * sharing back on, and so restore every link, just to pull one.
   */
  sharingEnabled: boolean;
  /** Config Write: the sharing-off notice links to the Sharing settings. */
  canManageSharing: boolean;
}

export function ShareLinkDialog({
  open,
  onOpenChange,
  bookId,
  bookTitle,
  canWrite,
  canList,
  requireExpiration,
  sharingEnabled,
  canManageSharing,
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

  const linksQuery = useBookShareLinks(bookId, { enabled: open });
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
      toastRequestError(error, "Failed to create share link");
    }
  };

  const handleRevoke = async () => {
    if (!toRevoke) return;
    try {
      await revokeMutation.mutateAsync({ bookId, linkId: toRevoke.id });
      toast.success("Share link revoked");
    } catch (error) {
      toastRequestError(error, "Failed to revoke share link");
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
      toastRequestError(error, "Failed to delete share link");
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
            {!sharingEnabled && (
              <div
                className="flex items-start gap-2 p-3 rounded-md bg-muted/50 text-sm text-muted-foreground"
                role="status"
              >
                <PowerOff className="h-4 w-4 mt-0.5 shrink-0" />
                <p>
                  Sharing is turned off, so no link works right now. Links you
                  revoke or delete here stay that way when an admin turns
                  sharing back on in{" "}
                  {canManageSharing ? (
                    <Link
                      className="underline underline-offset-4 hover:text-foreground"
                      onClick={() => onOpenChange(false)}
                      to="/settings/sharing"
                    >
                      Settings &gt; Sharing
                    </Link>
                  ) : (
                    "Settings > Sharing"
                  )}
                  .
                </p>
              </div>
            )}

            {canWrite && sharingEnabled && (
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
                  <LoadingSpinner />
                ) : linksQuery.error && !linksQuery.data ? (
                  <QueryError
                    fallback="Failed to load share links"
                    query={linksQuery}
                  />
                ) : links.length === 0 ? (
                  <p className="text-sm text-muted-foreground py-2">
                    This book has no share links yet.
                  </p>
                ) : (
                  <ul className="space-y-2">
                    {links.map((link) => {
                      // A link that does not resolve keeps its row but recedes:
                      // no fill, a muted label, and the muted status badge. A
                      // paused link is still active (it can be revoked) but its
                      // creator stops it from resolving.
                      const active = link.state === ShareLinkStateActive;
                      const paused = active && Boolean(link.paused_reason);
                      const live = active && !paused && sharingEnabled;
                      return (
                        <li
                          className={cn(
                            "flex items-center justify-between gap-2 py-2 px-3 rounded-md border",
                            live && "bg-muted/50",
                          )}
                          key={link.id}
                        >
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center gap-2">
                              <span
                                className={cn(
                                  "font-medium truncate",
                                  (!link.label || !live) &&
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
                                  !live &&
                                    "border-transparent bg-muted text-muted-foreground",
                                )}
                                variant={live ? "success" : "outline"}
                              >
                                {paused ? "paused" : link.state}
                              </Badge>
                            </div>
                            <p className="text-xs text-muted-foreground">
                              Created by {link.created_by_username} ·{" "}
                              {expiryText(link)}
                            </p>
                            {paused && (
                              <p className="text-xs text-muted-foreground">
                                {pausedText(link)}
                              </p>
                            )}
                            <p className="text-xs text-muted-foreground">
                              {usageText(link)}
                            </p>
                          </div>
                          <div className="flex items-center shrink-0">
                            <Button
                              aria-label={`Copy link ${link.label || ""}`.trim()}
                              className="h-8 w-8"
                              disabled={!live}
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
        description={revokeDescription(toRevoke, sharingEnabled)}
        isPending={revokeMutation.isPending}
        onConfirm={handleRevoke}
        onOpenChange={setRevokeOpen}
        open={revokeOpen}
        title="Revoke Link"
      />
      <ConfirmDialog
        confirmLabel="Delete"
        description={
          toDelete?.state === ShareLinkStateActive &&
          !toDelete.paused_reason &&
          sharingEnabled
            ? `${linkSubject(toDelete)} stops working immediately and is removed from the list with its counts. This cannot be undone.`
            : `${linkSubject(toDelete)} is removed from the list with its counts. This cannot be undone.`
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
