import { Ban, Copy, Link2, Plus, Shield } from "lucide-react";
import { useMemo, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import QueryError from "@/components/library/QueryError";
import RoleDialog from "@/components/library/RoleDialog";
import TruncatedText from "@/components/library/TruncatedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useInviteLinks, useRevokeInviteLink } from "@/hooks/queries/invites";
import { useLibraries } from "@/hooks/queries/libraries";
import { useRoles, useUsers } from "@/hooks/queries/users";
import { useCan } from "@/hooks/useCan";
import { usePageTitle } from "@/hooks/usePageTitle";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import type { InviteLinkResponse, Role, User } from "@/types";
import { copyTextWithToast } from "@/utils/copyWithToast";
import { formatCount, formatDateTime, formatTimeUntil } from "@/utils/format";
import { inviteLinkUrl } from "@/utils/inviteUrl";
import { sortRoles } from "@/utils/roles";

type Tab = "users" | "roles";

const ROW_CLASS = "flex items-center gap-3 py-3 px-4 md:px-6";

// A user's Library access in words. Names need the Library list, which only
// roles that can assign access can read, so other roles see a count.
const userAccessText = (
  user: User,
  libraryNames: Map<number, string> | null,
) => {
  const access = user.library_access ?? [];
  if (access.some((a) => a.library_id == null)) return "All libraries";
  const ids = access.flatMap((a) =>
    a.library_id == null ? [] : [a.library_id],
  );
  if (ids.length === 0) return "No libraries";
  if (!libraryNames) return formatCount(ids.length, "library", "libraries");
  return ids
    .map((id) => libraryNames.get(id))
    .filter((name): name is string => Boolean(name))
    .sort((a, b) => a.localeCompare(b))
    .join(", ");
};

const inviteAccessText = (link: InviteLinkResponse) => {
  if (link.all_library_access) return "All libraries";
  if (link.libraries.length === 0) return "No libraries";
  return link.libraries.map((library) => library.name).join(", ");
};

// A custom role's name can be long, so its badge shrinks to fit the row and
// cuts the name off there, with the whole name on hover.
const RoleBadge = ({ name }: { name: string }) => (
  <Badge className="min-w-0 max-w-full shrink" variant="secondary">
    <TruncatedText className="truncate">{name}</TruncatedText>
  </Badge>
);

// The first letter of a username in a circle, or a dashed circle with a link
// icon for an invite nobody has accepted yet.
const Avatar = ({ username }: { username?: string }) => (
  <span
    aria-hidden="true"
    className={cn(
      "flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-semibold",
      username
        ? "bg-secondary text-secondary-foreground"
        : "border border-dashed border-muted-foreground/60 text-muted-foreground",
    )}
  >
    {username ? (
      username.charAt(0).toUpperCase()
    ) : (
      <Link2 className="h-4 w-4" />
    )}
  </span>
);

const UserRow = ({
  user,
  libraryNames,
}: {
  user: User;
  libraryNames: Map<number, string> | null;
}) => (
  <li>
    <Link
      className={cn(ROW_CLASS, "hover:bg-muted/50 transition-colors")}
      to={`/admin/users/${user.id}`}
    >
      <Avatar username={user.username} />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="font-medium text-foreground text-sm md:text-base break-all">
            {user.username}
          </span>
          {user.role && <RoleBadge name={user.role.name} />}
          {!user.is_active && <Badge variant="destructive">Inactive</Badge>}
        </div>
        <p className="mt-0.5 text-xs md:text-sm text-muted-foreground break-words">
          {[user.email, userAccessText(user, libraryNames)]
            .filter(Boolean)
            .join(" · ")}
        </p>
      </div>
    </Link>
  </li>
);

const InviteRow = ({
  link,
  onRevoke,
}: {
  link: InviteLinkResponse;
  onRevoke: (link: InviteLinkResponse) => void;
}) => {
  const roleName = link.role?.name ?? "invite";
  return (
    <li className={ROW_CLASS}>
      <Avatar />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="font-medium text-foreground text-sm md:text-base">
            Invite link
          </span>
          <Badge
            className="border-primary/30 bg-primary/10 text-primary"
            variant="outline"
          >
            Pending
          </Badge>
          {link.role && <RoleBadge name={link.role.name} />}
        </div>
        <p className="mt-0.5 text-xs md:text-sm text-muted-foreground break-words">
          {inviteAccessText(link)} ·{" "}
          <span title={formatDateTime(link.expires_at)}>
            Expires {formatTimeUntil(link.expires_at)}
          </span>{" "}
          · Created by {link.created_by_username}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-0.5">
        <Button
          aria-label={`Copy invite link for ${roleName}`}
          onClick={() =>
            copyTextWithToast(
              inviteLinkUrl(link.token),
              "Could not copy the link",
            )
          }
          size="icon-sm"
          variant="ghost"
        >
          <Copy className="h-4 w-4" />
        </Button>
        <Button
          aria-label={`Revoke invite for ${roleName}`}
          onClick={() => onRevoke(link)}
          size="icon-sm"
          variant="ghost"
        >
          <Ban className="h-4 w-4" />
        </Button>
      </div>
    </li>
  );
};

// Every account, with pending invites at the top of the same list: an
// invite is a user who hasn't signed up yet.
const UsersTab = ({ canWriteUsers }: { canWriteUsers: boolean }) => {
  const usersQuery = useUsers();
  const invitesQuery = useInviteLinks();
  const librariesQuery = useLibraries();
  const revokeMutation = useRevokeInviteLink();
  const [toRevoke, setToRevoke] = useState<InviteLinkResponse | null>(null);

  const libraryNames = useMemo(
    () =>
      librariesQuery.data
        ? new Map(librariesQuery.data.items.map((l) => [l.id, l.name]))
        : null,
    [librariesQuery.data],
  );

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

  if (usersQuery.error && !usersQuery.data) {
    return <QueryError fallback="Failed to load users" query={usersQuery} />;
  }
  if (!usersQuery.data) return <LoadingSpinner />;

  const users = usersQuery.data.items;
  const invites = canWriteUsers ? (invitesQuery.data ?? []) : [];

  return (
    <div className="space-y-4">
      {canWriteUsers && invitesQuery.error && !invitesQuery.data && (
        <QueryError fallback="Failed to load invites" query={invitesQuery} />
      )}
      <ul
        aria-label="Users"
        className="border border-border rounded-md divide-y divide-border"
      >
        {invites.map((link) => (
          <InviteRow
            key={`invite-${link.id}`}
            link={link}
            onRevoke={setToRevoke}
          />
        ))}
        {users.map((user) => (
          <UserRow key={user.id} libraryNames={libraryNames} user={user} />
        ))}
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
    </div>
  );
};

const RolesTab = ({ onOpenRole }: { onOpenRole: (role: Role) => void }) => {
  const rolesQuery = useRoles();

  if (rolesQuery.error && !rolesQuery.data) {
    return <QueryError fallback="Failed to load roles" query={rolesQuery} />;
  }
  if (!rolesQuery.data) return <LoadingSpinner />;

  const roles = sortRoles(rolesQuery.data.items);
  return (
    <ul
      aria-label="Roles"
      className="border border-border rounded-md divide-y divide-border"
    >
      {roles.map((role) => (
        <li key={role.id}>
          <Button
            className={cn(
              ROW_CLASS,
              "w-full justify-between text-left hover:bg-muted/50 transition-colors",
            )}
            onClick={() => onOpenRole(role)}
            variant="unstyled"
          >
            <span className="flex flex-wrap items-center gap-2 md:gap-3">
              <Shield className="h-4 w-4 text-muted-foreground shrink-0" />
              <span className="font-medium text-foreground text-sm md:text-base">
                {role.name}
              </span>
              {role.is_system && <Badge variant="outline">System</Badge>}
            </span>
            <span className="text-xs md:text-sm text-muted-foreground shrink-0">
              {formatCount(role.permissions?.length ?? 0, "permission")}
            </span>
          </Button>
        </li>
      ))}
    </ul>
  );
};

// Administration > Users: accounts and pending invites on the Users tab,
// roles on the Roles tab at /admin/users/roles. The header's button adds
// whatever the open tab lists.
const AdminUsers = () => {
  usePageTitle("Users & Roles");

  const location = useLocation();
  const navigate = useNavigate();
  const tab: Tab = location.pathname.endsWith("/users/roles")
    ? "roles"
    : "users";

  // Adding users and managing roles both need Users Write.
  const canWriteUsers = useCan("users:write");
  const { data: usersData } = useUsers();
  const { data: rolesData } = useRoles();
  const { data: invitesData } = useInviteLinks();

  const [roleDialogOpen, setRoleDialogOpen] = useState(false);
  const [selectedRole, setSelectedRole] = useState<Role | null>(null);

  const openRoleDialog = (role?: Role) => {
    setSelectedRole(role ?? null);
    setRoleDialogOpen(true);
  };

  const handleRoleDialogChange = (open: boolean) => {
    setRoleDialogOpen(open);
    if (!open) setSelectedRole(null);
  };

  const handleTabChange = (value: string) =>
    navigate(value === "roles" ? "/admin/users/roles" : "/admin/users");

  const userCount =
    usersData &&
    usersData.total + (canWriteUsers ? (invitesData?.length ?? 0) : 0);

  return (
    <div>
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between md:mb-8">
        <div className="min-w-0">
          <h1 className="mb-1 text-xl font-semibold md:mb-2 md:text-2xl">
            Users &amp; Roles
          </h1>
          <p className="text-sm text-muted-foreground md:text-base">
            Who can sign in, and what each role can do.
          </p>
        </div>
        {canWriteUsers && (
          <div className="flex shrink-0 items-center gap-2">
            {tab === "users" ? (
              <Button aria-label="Add User" asChild size="sm">
                <Link to="/admin/users/create">
                  <Plus className="h-4 w-4 sm:mr-2" />
                  <span className="hidden sm:inline">Add User</span>
                </Link>
              </Button>
            ) : (
              <Button
                aria-label="Add Role"
                onClick={() => openRoleDialog()}
                size="sm"
              >
                <Plus className="h-4 w-4 sm:mr-2" />
                <span className="hidden sm:inline">Add Role</span>
              </Button>
            )}
          </div>
        )}
      </div>

      <Tabs onValueChange={handleTabChange} value={tab}>
        <TabsList className="mb-4 w-full justify-start overflow-x-auto">
          <TabsTrigger className="text-xs sm:text-sm" value="users">
            Users
            {userCount !== undefined && (
              <span className="ml-1.5 text-muted-foreground">
                {userCount.toLocaleString()}
              </span>
            )}
          </TabsTrigger>
          <TabsTrigger className="text-xs sm:text-sm" value="roles">
            Roles
            {rolesData && (
              <span className="ml-1.5 text-muted-foreground">
                {rolesData.total.toLocaleString()}
              </span>
            )}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="users">
          <UsersTab canWriteUsers={canWriteUsers} />
        </TabsContent>
        <TabsContent value="roles">
          <RolesTab onOpenRole={openRoleDialog} />
        </TabsContent>
      </Tabs>

      <RoleDialog
        onOpenChange={handleRoleDialogChange}
        open={roleDialogOpen}
        role={selectedRole}
      />
    </div>
  );
};

export default AdminUsers;
