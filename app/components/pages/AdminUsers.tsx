import { Plus, Shield } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import RoleDialog from "@/components/library/RoleDialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useRoles, useUsers } from "@/hooks/queries/users";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { Role, User } from "@/types";
import { sortRoles } from "@/utils/roles";

interface UserRowProps {
  user: User;
}

const UserRow = ({ user }: UserRowProps) => (
  <Link
    className="flex items-center justify-between py-3 md:py-4 px-4 md:px-6 hover:bg-muted/50 transition-colors"
    to={`/settings/users/${user.id}`}
  >
    <div className="flex-1 min-w-0">
      <div className="flex items-center gap-2 md:gap-3 flex-wrap">
        <span className="font-medium text-foreground text-sm md:text-base">
          {user.username}
        </span>
        {user.role && <Badge variant="secondary">{user.role.name}</Badge>}
        {!user.is_active && <Badge variant="destructive">Inactive</Badge>}
      </div>
      {user.email && (
        <p className="text-xs md:text-sm text-muted-foreground mt-1 truncate">
          {user.email}
        </p>
      )}
    </div>
  </Link>
);

interface RoleRowProps {
  role: Role;
  onClick: () => void;
}

const RoleRow = ({ role, onClick }: RoleRowProps) => {
  const permissionCount = role.permissions?.length ?? 0;

  return (
    <button
      className="w-full flex items-center justify-between py-3 md:py-4 px-4 md:px-6 hover:bg-muted/50 transition-colors text-left cursor-pointer gap-3"
      onClick={onClick}
      type="button"
    >
      <div className="flex items-center gap-2 md:gap-3 flex-wrap">
        <Shield className="h-4 w-4 text-muted-foreground shrink-0" />
        <span className="font-medium text-foreground text-sm md:text-base">
          {role.name}
        </span>
        {role.is_system && <Badge variant="outline">System</Badge>}
      </div>
      <span className="text-xs md:text-sm text-muted-foreground shrink-0">
        {permissionCount} permission{permissionCount !== 1 ? "s" : ""}
      </span>
    </button>
  );
};

const AdminUsers = () => {
  usePageTitle("Users & Roles");

  const { hasPermission } = useAuth();
  const {
    data: usersData,
    isLoading: usersLoading,
    error: usersError,
  } = useUsers();
  const {
    data: rolesData,
    isLoading: rolesLoading,
    error: rolesError,
  } = useRoles();

  const [roleDialogOpen, setRoleDialogOpen] = useState(false);
  const [selectedRole, setSelectedRole] = useState<Role | null>(null);

  const canCreateUsers = hasPermission("users", "write");
  const canManageRoles = hasPermission("users", "write");

  const handleOpenRoleDialog = (role?: Role) => {
    setSelectedRole(role ?? null);
    setRoleDialogOpen(true);
  };

  const handleCloseRoleDialog = (open: boolean) => {
    setRoleDialogOpen(open);
    if (!open) {
      setSelectedRole(null);
    }
  };

  if (usersLoading || rolesLoading) {
    return <LoadingSpinner />;
  }

  if (usersError) {
    return (
      <div className="text-center">
        <h1 className="text-2xl font-semibold mb-4">Error Loading Users</h1>
        <p className="text-muted-foreground">{usersError.message}</p>
      </div>
    );
  }

  if (rolesError) {
    return (
      <div className="text-center">
        <h1 className="text-2xl font-semibold mb-4">Error Loading Roles</h1>
        <p className="text-muted-foreground">{rolesError.message}</p>
      </div>
    );
  }

  const users = usersData?.users ?? [];
  const roles = sortRoles(rolesData?.roles ?? []);

  return (
    <div className="space-y-12">
      {/* Users Section */}
      <div>
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6 md:mb-8">
          <div>
            <h1 className="text-xl md:text-2xl font-semibold mb-1 md:mb-2">
              Users
            </h1>
            <p className="text-sm md:text-base text-muted-foreground">
              Manage user accounts and permissions.
            </p>
          </div>
          {canCreateUsers && (
            <div className="flex items-center gap-2 shrink-0">
              <Button asChild size="sm">
                <Link to="/settings/users/create">
                  <Plus className="h-4 w-4 sm:mr-2" />
                  <span className="hidden sm:inline">Add User</span>
                </Link>
              </Button>
            </div>
          )}
        </div>

        {users.length === 0 ? (
          <div className="border border-border rounded-md p-8 text-center">
            <p className="text-muted-foreground">No users found.</p>
          </div>
        ) : (
          <div className="border border-border rounded-md divide-y divide-border">
            {users.map((user) => (
              <UserRow key={user.id} user={user} />
            ))}
          </div>
        )}
      </div>

      {/* Roles Section */}
      <div>
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6 md:mb-8">
          <div>
            <h2 className="text-xl md:text-2xl font-semibold mb-1 md:mb-2">
              Roles
            </h2>
            <p className="text-sm md:text-base text-muted-foreground">
              Define roles with custom permissions.
            </p>
          </div>
          {canManageRoles && (
            <div className="flex items-center gap-2 shrink-0">
              <Button onClick={() => handleOpenRoleDialog()} size="sm">
                <Plus className="h-4 w-4 sm:mr-2" />
                <span className="hidden sm:inline">Add Role</span>
              </Button>
            </div>
          )}
        </div>

        {roles.length === 0 ? (
          <div className="border border-border rounded-md p-8 text-center">
            <p className="text-muted-foreground">No roles found.</p>
          </div>
        ) : (
          <div className="border border-border rounded-md divide-y divide-border">
            {roles.map((role) => (
              <RoleRow
                key={role.id}
                onClick={() => handleOpenRoleDialog(role)}
                role={role}
              />
            ))}
          </div>
        )}
      </div>

      {/* Role Dialog */}
      <RoleDialog
        onOpenChange={handleCloseRoleDialog}
        open={roleDialogOpen}
        role={selectedRole}
      />
    </div>
  );
};

export default AdminUsers;
