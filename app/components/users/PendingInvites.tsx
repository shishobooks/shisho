import { Ban, Copy } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import QueryError from "@/components/library/QueryError";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { useInviteLinks, useRevokeInviteLink } from "@/hooks/queries/invites";
import { useLibraries } from "@/hooks/queries/libraries";
import { toastRequestError } from "@/libraries/api";
import type { InviteLinkResponse, Library } from "@/types";
import { copyTextWithToast } from "@/utils/copyWithToast";
import { formatDateTime } from "@/utils/format";
import { inviteLinkUrl } from "@/utils/inviteUrl";

const accessText = (link: InviteLinkResponse, libraries: Library[]) => {
  if (link.all_library_access) return "All libraries";
  if (link.library_ids.length === 0) return "No libraries";
  const names = link.library_ids.map(
    (id) => libraries.find((library) => library.id === id)?.name ?? `#${id}`,
  );
  return names.join(", ");
};

/**
 * The Invite Links still waiting to be accepted, at the top of the Users
 * page. Each can have its link copied again or be revoked. Nothing renders
 * while none are pending.
 */
const PendingInvites = () => {
  const invitesQuery = useInviteLinks();
  const { data: librariesData } = useLibraries();
  const revokeMutation = useRevokeInviteLink();
  const [toRevoke, setToRevoke] = useState<InviteLinkResponse | null>(null);

  const libraries = librariesData?.items ?? [];
  const invites = invitesQuery.data ?? [];

  const handleRevoke = async () => {
    if (!toRevoke) return;
    try {
      await revokeMutation.mutateAsync(toRevoke.id);
      toast.success("Invite revoked");
    } catch (error) {
      toastRequestError(error, "Failed to revoke invite");
    } finally {
      setToRevoke(null);
    }
  };

  if (invitesQuery.error && !invitesQuery.data) {
    return (
      <section aria-labelledby="pending-invites-heading" className="mb-8">
        <h2 className="text-lg font-medium mb-3" id="pending-invites-heading">
          Pending invites
        </h2>
        <QueryError fallback="Failed to load invites" query={invitesQuery} />
      </section>
    );
  }
  if (invites.length === 0) return null;

  return (
    <section aria-labelledby="pending-invites-heading" className="mb-8">
      <h2 className="text-lg font-medium mb-3" id="pending-invites-heading">
        Pending invites
      </h2>
      <ul className="border border-border rounded-md divide-y divide-border">
        {invites.map((link) => {
          const access = accessText(link, libraries);
          return (
            <li
              className="flex items-center justify-between gap-3 py-3 px-4 md:px-6"
              key={link.id}
            >
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 flex-wrap">
                  {link.role && (
                    <Badge variant="secondary">{link.role.name}</Badge>
                  )}
                  <span className="text-sm text-foreground break-words min-w-0">
                    {access}
                  </span>
                </div>
                <p className="text-xs md:text-sm text-muted-foreground mt-1">
                  Expires {formatDateTime(link.expires_at)} · Created by{" "}
                  {link.created_by_username}
                </p>
              </div>
              <div className="flex items-center gap-1 shrink-0">
                <Button
                  aria-label={`Copy invite link for ${link.role?.name ?? "invite"}`}
                  onClick={() =>
                    copyTextWithToast(
                      inviteLinkUrl(link.token),
                      "Could not copy the link",
                    )
                  }
                  size="icon"
                  title="Copy link"
                  variant="ghost"
                >
                  <Copy className="h-4 w-4" />
                </Button>
                <Button
                  aria-label={`Revoke invite for ${link.role?.name ?? "invite"}`}
                  onClick={() => setToRevoke(link)}
                  size="icon"
                  title="Revoke"
                  variant="ghost"
                >
                  <Ban className="h-4 w-4" />
                </Button>
              </div>
            </li>
          );
        })}
      </ul>

      <ConfirmDialog
        confirmLabel="Revoke"
        description="The link stops working right away. Create a new invite if they still need one."
        isPending={revokeMutation.isPending}
        onConfirm={handleRevoke}
        onOpenChange={(open) => !open && setToRevoke(null)}
        open={toRevoke !== null}
        title="Revoke this invite?"
      />
    </section>
  );
};

export default PendingInvites;
