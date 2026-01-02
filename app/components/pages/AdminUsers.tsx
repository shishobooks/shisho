import { Plus } from "lucide-react";
import { Link } from "react-router-dom";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useUsers } from "@/hooks/queries/users";
import { useAuth } from "@/hooks/useAuth";
import type { User } from "@/types";

interface UserRowProps {
  user: User;
}

const UserRow = ({ user }: UserRowProps) => (
  <Link
    className="flex items-center justify-between py-4 px-6 hover:bg-muted/50 transition-colors"
    to={`/admin/users/${user.id}`}
  >
    <div className="flex-1 min-w-0">
      <div className="flex items-center gap-3">
        <span className="font-medium text-foreground">{user.username}</span>
        {user.role && <Badge variant="secondary">{user.role.name}</Badge>}
        {!user.is_active && <Badge variant="destructive">Inactive</Badge>}
      </div>
      {user.email && (
        <p className="text-sm text-muted-foreground mt-1">{user.email}</p>
      )}
    </div>
  </Link>
);

const AdminUsers = () => {
  const { hasPermission } = useAuth();
  const { data, isLoading, error } = useUsers();

  const canCreateUsers = hasPermission("users", "write");

  if (isLoading) {
    return <LoadingSpinner />;
  }

  if (error) {
    return (
      <div className="text-center">
        <h1 className="text-2xl font-semibold mb-4">Error Loading Users</h1>
        <p className="text-muted-foreground">{error.message}</p>
      </div>
    );
  }

  const users = data?.users ?? [];

  return (
    <div>
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl font-semibold mb-2">Users</h1>
          <p className="text-muted-foreground">
            Manage user accounts and permissions.
          </p>
        </div>
        {canCreateUsers && (
          <Button asChild size="sm">
            <Link to="/admin/users/create">
              <Plus className="h-4 w-4 mr-2" />
              Add User
            </Link>
          </Button>
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
  );
};

export default AdminUsers;
