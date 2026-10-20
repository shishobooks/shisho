import { useEffect, useState, type FormEvent } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { toast } from "sonner";

import Logo from "@/components/library/Logo";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useResetPassword } from "@/hooks/queries/users";
import { useAuth } from "@/hooks/useAuth";
import { toastRequestError } from "@/libraries/api";

/** The id of the Preferences section holding the password form. */
export const PASSWORD_SECTION_ID = "password";

// The form state both password screens share. A forced reset skips the
// current password, and after a change the user returns to the page that
// sent them here (`?redirect=`).
const useChangePassword = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const redirectTo = searchParams.get("redirect");
  const { user, refetch } = useAuth();
  const resetPasswordMutation = useResetPassword();
  const isForced = Boolean(user?.must_change_password);

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  const submit = async () => {
    if (!isForced && !currentPassword) {
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
          current_password: isForced ? undefined : currentPassword,
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

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    void submit();
  };

  return {
    currentPassword,
    setCurrentPassword,
    newPassword,
    setNewPassword,
    confirmPassword,
    setConfirmPassword,
    hasChanges: Boolean(currentPassword || newPassword || confirmPassword),
    isPending: resetPasswordMutation.isPending,
    handleSubmit,
  };
};

type ChangePasswordForm = ReturnType<typeof useChangePassword>;

const NewPasswordFields = ({ form }: { form: ChangePasswordForm }) => (
  <>
    <div className="space-y-2">
      <Label htmlFor="new-password">New Password</Label>
      <Input
        autoComplete="new-password"
        id="new-password"
        onChange={(e) => form.setNewPassword(e.target.value)}
        placeholder="Enter a new password"
        type="password"
        value={form.newPassword}
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
        onChange={(e) => form.setConfirmPassword(e.target.value)}
        placeholder="Confirm your new password"
        type="password"
        value={form.confirmPassword}
      />
    </div>
  </>
);

/** The full-screen form shown instead of any page until a temporary password is replaced. */
export const ForcedPasswordReset = () => {
  const form = useChangePassword();

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

        <form className="space-y-6" onSubmit={form.handleSubmit}>
          <NewPasswordFields form={form} />
          <Button className="w-full" disabled={form.isPending} type="submit">
            {form.isPending ? "Changing..." : "Change Password"}
          </Button>
        </form>
      </div>
    </div>
  );
};

/** The Password section of Preferences. */
export const PasswordSection = ({
  onHasChangesChange,
}: {
  onHasChangesChange: (hasChanges: boolean) => void;
}) => {
  const form = useChangePassword();
  const { hasChanges } = form;

  useEffect(() => {
    onHasChangesChange(hasChanges);
  }, [hasChanges, onHasChangesChange]);

  return (
    <div
      className="scroll-mt-20 rounded-md border border-border p-4 md:p-6"
      id={PASSWORD_SECTION_ID}
    >
      <h2 className="mb-4 text-lg font-semibold">Password</h2>
      <form className="space-y-4" onSubmit={form.handleSubmit}>
        <div className="space-y-2">
          <Label htmlFor="current-password">Current Password</Label>
          <Input
            autoComplete="current-password"
            id="current-password"
            onChange={(e) => form.setCurrentPassword(e.target.value)}
            placeholder="Enter your current password"
            type="password"
            value={form.currentPassword}
          />
        </div>
        <NewPasswordFields form={form} />
        <div className="flex justify-end pt-2">
          <Button disabled={form.isPending} type="submit">
            {form.isPending ? "Changing..." : "Change Password"}
          </Button>
        </div>
      </form>
    </div>
  );
};
