import { ArrowLeft, Loader2, Save, Shield, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { useLibraries } from "@/hooks/queries/libraries";
import {
  useDeactivateUser,
  useResetPassword,
  useRoles,
  useUpdateUser,
  useUser,
} from "@/hooks/queries/users";
import { useAuth } from "@/hooks/useAuth";

const UserDetail = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { user: currentUser, hasPermission } = useAuth();

  const { data: user, isLoading, error } = useUser(id);
  const { data: rolesData } = useRoles();
  const { data: librariesData } = useLibraries();
  const updateUserMutation = useUpdateUser();
  const resetPasswordMutation = useResetPassword();
  const deactivateUserMutation = useDeactivateUser();

  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [roleId, setRoleId] = useState<number | null>(null);
  const [allLibraryAccess, setAllLibraryAccess] = useState(false);
  const [selectedLibraries, setSelectedLibraries] = useState<number[]>([]);

  const [passwordDialogOpen, setPasswordDialogOpen] = useState(false);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  const [deactivateDialogOpen, setDeactivateDialogOpen] = useState(false);

  const canWrite = hasPermission("users", "write");
  const isSelf = currentUser?.id === Number(id);

  useEffect(() => {
    if (user) {
      setUsername(user.username);
      setEmail(user.email ?? "");
      setRoleId(user.role_id);

      const hasAllAccess = user.library_access?.some(
        (a) => a.library_id === null,
      );
      setAllLibraryAccess(hasAllAccess ?? false);

      if (!hasAllAccess) {
        setSelectedLibraries(
          user.library_access
            ?.filter((a) => a.library_id !== null)
            .map((a) => a.library_id!) ?? [],
        );
      }
    }
  }, [user]);

  const handleSave = async () => {
    if (!id) return;

    try {
      await updateUserMutation.mutateAsync({
        id,
        payload: {
          username: username !== user?.username ? username : undefined,
          email: email !== (user?.email ?? "") ? email || undefined : undefined,
          role_id: roleId !== user?.role_id ? (roleId ?? undefined) : undefined,
          all_library_access: allLibraryAccess,
          library_ids: allLibraryAccess ? undefined : selectedLibraries,
        },
      });
      toast.success("User updated successfully");
    } catch (error) {
      let msg = "Failed to update user";
      if (error instanceof Error) {
        msg = error.message;
      }
      toast.error(msg);
    }
  };

  const handleResetPassword = async () => {
    if (!id) return;

    if (newPassword.length < 8) {
      toast.error("Password must be at least 8 characters");
      return;
    }

    if (newPassword !== confirmPassword) {
      toast.error("Passwords do not match");
      return;
    }

    if (isSelf && !currentPassword) {
      toast.error("Current password is required");
      return;
    }

    try {
      await resetPasswordMutation.mutateAsync({
        id,
        payload: {
          current_password: isSelf ? currentPassword : undefined,
          new_password: newPassword,
        },
      });
      toast.success("Password reset successfully");
      setPasswordDialogOpen(false);
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    } catch (error) {
      let msg = "Failed to reset password";
      if (error instanceof Error) {
        msg = error.message;
      }
      toast.error(msg);
    }
  };

  const handleDeactivate = async () => {
    if (!id) return;

    try {
      await deactivateUserMutation.mutateAsync(id);
      toast.success("User deactivated successfully");
      navigate("/admin/users");
    } catch (error) {
      let msg = "Failed to deactivate user";
      if (error instanceof Error) {
        msg = error.message;
      }
      toast.error(msg);
    }
  };

  const handleLibraryToggle = (libraryId: number) => {
    setSelectedLibraries((prev) =>
      prev.includes(libraryId)
        ? prev.filter((id) => id !== libraryId)
        : [...prev, libraryId],
    );
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error || !user) {
    return (
      <div className="text-center py-20">
        <p className="text-destructive">{error?.message ?? "User not found"}</p>
      </div>
    );
  }

  const roles = rolesData?.roles ?? [];
  const libraries = librariesData?.libraries ?? [];

  return (
    <div>
      <div className="mb-6">
        <Button asChild variant="ghost">
          <Link to="/admin/users">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Users
          </Link>
        </Button>
      </div>

      <div className="flex items-center justify-between mb-8">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-semibold">{user.username}</h1>
          <div className="flex items-center gap-2">
            {!user.is_active && <Badge variant="secondary">Inactive</Badge>}
            <Badge className="flex items-center gap-1" variant="outline">
              <Shield className="h-3 w-3" />
              {user.role?.name ?? "Unknown"}
            </Badge>
          </div>
        </div>
      </div>

      <div className="max-w-2xl space-y-6 border border-border rounded-md p-6">
        {/* Basic Info */}
        <div className="space-y-4">
          <h2 className="text-lg font-medium">Basic Information</h2>

          <div className="space-y-2">
            <Label htmlFor="username">Username</Label>
            <Input
              disabled={!canWrite}
              id="username"
              onChange={(e) => setUsername(e.target.value)}
              value={username}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input
              disabled={!canWrite}
              id="email"
              onChange={(e) => setEmail(e.target.value)}
              placeholder="Optional"
              type="email"
              value={email}
            />
          </div>
        </div>

        <Separator />

        {/* Role */}
        <div className="space-y-4">
          <h2 className="text-lg font-medium">Role</h2>
          <div className="space-y-2">
            <Label>Select Role</Label>
            <div className="grid gap-2">
              {roles.map((role) => (
                <div className="flex items-center space-x-2" key={role.id}>
                  <Checkbox
                    checked={roleId === role.id}
                    disabled={!canWrite}
                    id={`role-${role.id}`}
                    onCheckedChange={(checked) => {
                      if (checked) setRoleId(role.id);
                    }}
                  />
                  <Label
                    className="text-sm font-normal cursor-pointer"
                    htmlFor={`role-${role.id}`}
                  >
                    {role.name}
                    {role.is_system && (
                      <span className="text-muted-foreground ml-1">
                        (system)
                      </span>
                    )}
                  </Label>
                </div>
              ))}
            </div>
          </div>
        </div>

        <Separator />

        {/* Library Access */}
        <div className="space-y-4">
          <h2 className="text-lg font-medium">Library Access</h2>

          <div className="flex items-center space-x-2">
            <Checkbox
              checked={allLibraryAccess}
              disabled={!canWrite}
              id="all-library-access"
              onCheckedChange={(checked) =>
                setAllLibraryAccess(checked as boolean)
              }
            />
            <Label
              className="text-sm font-normal cursor-pointer"
              htmlFor="all-library-access"
            >
              Access to all libraries
            </Label>
          </div>

          {!allLibraryAccess && (
            <div className="space-y-2 pl-6">
              <Label>Select Libraries</Label>
              <div className="grid gap-2">
                {libraries.map((library) => (
                  <div className="flex items-center space-x-2" key={library.id}>
                    <Checkbox
                      checked={selectedLibraries.includes(library.id)}
                      disabled={!canWrite}
                      id={`library-${library.id}`}
                      onCheckedChange={() => handleLibraryToggle(library.id)}
                    />
                    <Label
                      className="text-sm font-normal cursor-pointer"
                      htmlFor={`library-${library.id}`}
                    >
                      {library.name}
                    </Label>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        <Separator />

        {/* Actions */}
        <div className="flex flex-wrap gap-4 pt-4">
          {canWrite && (
            <Button
              disabled={updateUserMutation.isPending}
              onClick={handleSave}
            >
              {updateUserMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                <>
                  <Save className="mr-2 h-4 w-4" />
                  Save Changes
                </>
              )}
            </Button>
          )}

          {(canWrite || isSelf) && (
            <Button
              onClick={() => setPasswordDialogOpen(true)}
              variant="outline"
            >
              Reset Password
            </Button>
          )}

          {canWrite && !isSelf && user.is_active && (
            <Button
              onClick={() => setDeactivateDialogOpen(true)}
              variant="destructive"
            >
              <Trash2 className="mr-2 h-4 w-4" />
              Deactivate User
            </Button>
          )}
        </div>
      </div>

      {/* Password Reset Dialog */}
      <Dialog onOpenChange={setPasswordDialogOpen} open={passwordDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Reset Password</DialogTitle>
            <DialogDescription>
              {isSelf
                ? "Enter your current password and a new password."
                : "Enter a new password for this user."}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-4">
            {isSelf && (
              <div className="space-y-2">
                <Label htmlFor="current-password">Current Password</Label>
                <Input
                  id="current-password"
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  type="password"
                  value={currentPassword}
                />
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="new-password">New Password</Label>
              <Input
                id="new-password"
                onChange={(e) => setNewPassword(e.target.value)}
                type="password"
                value={newPassword}
              />
              <p className="text-xs text-muted-foreground">
                At least 8 characters
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirm-new-password">Confirm New Password</Label>
              <Input
                id="confirm-new-password"
                onChange={(e) => setConfirmPassword(e.target.value)}
                type="password"
                value={confirmPassword}
              />
            </div>
          </div>
          <DialogFooter>
            <Button
              onClick={() => setPasswordDialogOpen(false)}
              variant="outline"
            >
              Cancel
            </Button>
            <Button
              disabled={resetPasswordMutation.isPending}
              onClick={handleResetPassword}
            >
              {resetPasswordMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Resetting...
                </>
              ) : (
                "Reset Password"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Deactivate Confirmation Dialog */}
      <Dialog
        onOpenChange={setDeactivateDialogOpen}
        open={deactivateDialogOpen}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Deactivate User</DialogTitle>
            <DialogDescription>
              Are you sure you want to deactivate this user? They will no longer
              be able to log in.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              onClick={() => setDeactivateDialogOpen(false)}
              variant="outline"
            >
              Cancel
            </Button>
            <Button
              disabled={deactivateUserMutation.isPending}
              onClick={handleDeactivate}
              variant="destructive"
            >
              {deactivateUserMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Deactivating...
                </>
              ) : (
                "Deactivate"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default UserDetail;
