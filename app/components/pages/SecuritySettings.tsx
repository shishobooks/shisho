import { useState, type FormEvent } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { toast } from "sonner";

import Logo from "@/components/library/Logo";
import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useResetPassword } from "@/hooks/queries/users";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";
import { toastRequestError } from "@/libraries/api";

const SecuritySettings = () => {
  usePageTitle("Security Settings");

  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const redirectTo = searchParams.get("redirect");
  const { user, refetch } = useAuth();
  const resetPasswordMutation = useResetPassword();
  const isForcedPasswordReset = Boolean(user?.must_change_password);

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  const handleResetPassword = async () => {
    if (!isForcedPasswordReset && !currentPassword) {
      toast.error("Current password is required");
      return;
    }

    if (newPassword.length < 8) {
      toast.error("Password must be at least 8 characters");
      return;
    }

    if (newPassword !== confirmPassword) {
      toast.error("Passwords do not match");
      return;
    }

    try {
      await resetPasswordMutation.mutateAsync({
        id: String(user!.id),
        payload: {
          current_password: isForcedPasswordReset ? undefined : currentPassword,
          new_password: newPassword,
          require_password_reset: false,
        },
      });
      await refetch();
      toast.success("Password changed successfully");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      if (redirectTo) {
        navigate(redirectTo, { replace: true });
      }
    } catch (error) {
      toastRequestError(error, "Failed to change password");
    }
  };

  const handlePasswordSubmit = (e: FormEvent) => {
    e.preventDefault();
    void handleResetPassword();
  };

  if (isForcedPasswordReset) {
    return (
      <div className="min-h-dvh flex items-center justify-center bg-background px-4 py-8">
        <div className="w-full max-w-md">
          <div className="flex flex-col items-center mb-6 md:mb-8">
            <Logo className="mb-2" size="lg" />
            <h1 className="text-xl font-semibold mb-1">
              Password Reset Required
            </h1>
            <p className="text-muted-foreground text-center">
              Set a new password to continue.
            </p>
          </div>

          <form className="space-y-6" onSubmit={handlePasswordSubmit}>
            <div className="space-y-2">
              <Label htmlFor="new-password">New Password</Label>
              <Input
                autoComplete="new-password"
                id="new-password"
                onChange={(e) => setNewPassword(e.target.value)}
                placeholder="Enter a new password"
                type="password"
                value={newPassword}
              />
              <p className="text-xs text-muted-foreground">
                Password must be at least 8 characters
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirm-password">Confirm New Password</Label>
              <Input
                autoComplete="new-password"
                id="confirm-password"
                onChange={(e) => setConfirmPassword(e.target.value)}
                placeholder="Confirm your new password"
                type="password"
                value={confirmPassword}
              />
            </div>
            <Button
              className="w-full"
              disabled={resetPasswordMutation.isPending}
              type="submit"
            >
              {resetPasswordMutation.isPending
                ? "Changing..."
                : "Change Password"}
            </Button>
          </form>
        </div>
      </div>
    );
  }

  return (
    <div>
      <TopNav />
      <div className="mx-auto w-full max-w-7xl px-6 py-8">
        <div className="mb-6 md:mb-8">
          <h1 className="mb-2 text-2xl font-semibold">Security Settings</h1>
          <p className="text-muted-foreground">Manage your password</p>
        </div>

        <div className="max-w-2xl space-y-6">
          {/* Password Change */}
          <div className="rounded-md border border-border p-4 md:p-6">
            <h2 className="mb-4 text-lg font-semibold">Change Password</h2>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="current-password">Current Password</Label>
                <Input
                  autoComplete="current-password"
                  id="current-password"
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  placeholder="Enter your current password"
                  type="password"
                  value={currentPassword}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="new-password">New Password</Label>
                <Input
                  autoComplete="new-password"
                  id="new-password"
                  onChange={(e) => setNewPassword(e.target.value)}
                  placeholder="Enter a new password"
                  type="password"
                  value={newPassword}
                />
                <p className="text-xs text-muted-foreground">
                  Password must be at least 8 characters
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="confirm-password">Confirm New Password</Label>
                <Input
                  autoComplete="new-password"
                  id="confirm-password"
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  placeholder="Confirm your new password"
                  type="password"
                  value={confirmPassword}
                />
              </div>
              <div className="flex justify-end pt-2">
                <Button
                  disabled={resetPasswordMutation.isPending}
                  onClick={handleResetPassword}
                >
                  {resetPasswordMutation.isPending
                    ? "Changing..."
                    : "Change Password"}
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default SecuritySettings;
