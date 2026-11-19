import equal from "fast-deep-equal";
import { Loader2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import CopyField from "@/components/library/CopyField";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import UserAccessFields, {
  type UserAccess,
} from "@/components/users/UserAccessFields";
import { useCreateInviteLink } from "@/hooks/queries/invites";
import { toastRequestError } from "@/libraries/api";
import type { InviteLinkResponse } from "@/types";
import { formatDateTime } from "@/utils/format";
import { inviteLinkUrl } from "@/utils/inviteUrl";

// How long a new invite works for, in days.
const INVITE_EXPIRY_OPTIONS = [
  { value: "1", label: "1 day" },
  { value: "7", label: "7 days" },
  { value: "30", label: "30 days" },
];

const INITIAL_VALUES = {
  access: {
    roleId: null,
    allLibraryAccess: true,
    selectedLibraries: [],
  } as UserAccess,
  expiresInDays: "7",
};

interface InviteLinkFormProps {
  onHasChangesChange: (hasChanges: boolean) => void;
}

/**
 * The "Invite link" path of Add User: the admin picks the new user's role,
 * Library access, and how long the link works, then copies the link. Shisho
 * never sends it.
 */
const InviteLinkForm = ({ onHasChangesChange }: InviteLinkFormProps) => {
  const createMutation = useCreateInviteLink();
  const [access, setAccess] = useState<UserAccess>(INITIAL_VALUES.access);
  const [expiresInDays, setExpiresInDays] = useState(
    INITIAL_VALUES.expiresInDays,
  );
  const [created, setCreated] = useState<InviteLinkResponse | null>(null);

  const hasChanges = useMemo(
    () =>
      created === null &&
      (!equal(access, INITIAL_VALUES.access) ||
        expiresInDays !== INITIAL_VALUES.expiresInDays),
    [access, expiresInDays, created],
  );
  useEffect(
    () => onHasChangesChange(hasChanges),
    [hasChanges, onHasChangesChange],
  );

  const reset = () => {
    setAccess(INITIAL_VALUES.access);
    setExpiresInDays(INITIAL_VALUES.expiresInDays);
    setCreated(null);
  };

  const handleCreate = async () => {
    if (!access.roleId) {
      toast.error("Please select a role");
      return;
    }
    try {
      const link = await createMutation.mutateAsync({
        role_id: access.roleId,
        all_library_access: access.allLibraryAccess,
        library_ids: access.allLibraryAccess ? [] : access.selectedLibraries,
        expires_in_days: Number(expiresInDays),
      });
      setCreated(link);
    } catch (error) {
      toastRequestError(error, "Failed to create invite link");
    }
  };

  if (created) {
    return (
      <div className="space-y-4">
        <div>
          <h2 className="text-lg font-medium">Invite link ready</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Send it to them however you like: Shisho doesn&apos;t send it for
            you. It works once and expires {formatDateTime(created.expires_at)}.
          </p>
        </div>
        <CopyField
          label="Invite link"
          url
          value={inviteLinkUrl(created.token)}
        />
        <div className="flex flex-col-reverse gap-2 pt-2 sm:flex-row sm:justify-end">
          <Button onClick={reset} variant="outline">
            Invite someone else
          </Button>
          <Button asChild>
            <Link to="/admin/users">Done</Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <p className="text-sm text-muted-foreground">
        They&apos;ll open the link, choose their own username and password, and
        land in Shisho with the access you pick here.
      </p>

      <UserAccessFields idPrefix="invite" onChange={setAccess} value={access} />

      <Separator />

      <div className="space-y-2">
        <Label htmlFor="invite-expiry">Link expires after</Label>
        <Select onValueChange={setExpiresInDays} value={expiresInDays}>
          <SelectTrigger className="w-full sm:w-48" id="invite-expiry">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {INVITE_EXPIRY_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex justify-end pt-4">
        <Button disabled={createMutation.isPending} onClick={handleCreate}>
          {createMutation.isPending ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Creating...
            </>
          ) : (
            "Create invite link"
          )}
        </Button>
      </div>
    </div>
  );
};

export default InviteLinkForm;
