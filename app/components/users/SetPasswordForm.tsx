import equal from "fast-deep-equal";
import { Loader2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import CopyField from "@/components/library/CopyField";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import UserAccessFields, {
  type UserAccess,
} from "@/components/users/UserAccessFields";
import { useCreateUser } from "@/hooks/queries/users";
import { toastRequestError } from "@/libraries/api";
import type { User } from "@/types";

const INITIAL_VALUES = {
  username: "",
  email: "",
  password: "",
  confirmPassword: "",
  // The password the admin picks is temporary unless they say otherwise.
  requirePasswordReset: true,
  access: {
    roleId: null,
    allLibraryAccess: true,
    selectedLibraries: [],
  } as UserAccess,
};

interface Created {
  user: User;
  requirePasswordReset: boolean;
}

interface SetPasswordFormProps {
  onHasChangesChange: (hasChanges: boolean) => void;
}

/**
 * The "Set a password" path of Add User: the admin creates the account and
 * then sees what to pass on to the new user.
 */
const SetPasswordForm = ({ onHasChangesChange }: SetPasswordFormProps) => {
  const createUserMutation = useCreateUser();

  const [username, setUsername] = useState(INITIAL_VALUES.username);
  const [email, setEmail] = useState(INITIAL_VALUES.email);
  const [password, setPassword] = useState(INITIAL_VALUES.password);
  const [confirmPassword, setConfirmPassword] = useState(
    INITIAL_VALUES.confirmPassword,
  );
  const [requirePasswordReset, setRequirePasswordReset] = useState(
    INITIAL_VALUES.requirePasswordReset,
  );
  const [access, setAccess] = useState<UserAccess>(INITIAL_VALUES.access);
  const [created, setCreated] = useState<Created | null>(null);

  const hasChanges = useMemo(
    () =>
      created === null &&
      (username.trim() !== INITIAL_VALUES.username ||
        email.trim() !== INITIAL_VALUES.email ||
        password !== INITIAL_VALUES.password ||
        confirmPassword !== INITIAL_VALUES.confirmPassword ||
        requirePasswordReset !== INITIAL_VALUES.requirePasswordReset ||
        !equal(access, INITIAL_VALUES.access)),
    [
      username,
      email,
      password,
      confirmPassword,
      requirePasswordReset,
      access,
      created,
    ],
  );
  useEffect(
    () => onHasChangesChange(hasChanges),
    [hasChanges, onHasChangesChange],
  );

  const reset = () => {
    setUsername(INITIAL_VALUES.username);
    setEmail(INITIAL_VALUES.email);
    setPassword(INITIAL_VALUES.password);
    setConfirmPassword(INITIAL_VALUES.confirmPassword);
    setRequirePasswordReset(INITIAL_VALUES.requirePasswordReset);
    setAccess(INITIAL_VALUES.access);
    setCreated(null);
  };

  const handleCreate = async () => {
    if (!username.trim()) {
      toast.error("Username is required");
      return;
    }
    if (username.length < 3) {
      toast.error("Username must be at least 3 characters");
      return;
    }
    if (!password) {
      toast.error("Password is required");
      return;
    }
    if (password.length < 8) {
      toast.error("Password must be at least 8 characters");
      return;
    }
    if (password !== confirmPassword) {
      toast.error("Passwords do not match");
      return;
    }
    if (!access.roleId) {
      toast.error("Please select a role");
      return;
    }

    try {
      const user = await createUserMutation.mutateAsync({
        username,
        email: email.trim() || undefined,
        password,
        role_id: access.roleId,
        require_password_reset: requirePasswordReset,
        all_library_access: access.allLibraryAccess,
        library_ids: access.allLibraryAccess ? [] : access.selectedLibraries,
      });
      toast.success("User created");
      setCreated({ user, requirePasswordReset });
    } catch (error) {
      toastRequestError(error, "Failed to create user");
    }
  };

  if (created) {
    return (
      <div className="space-y-4">
        <div>
          <h2 className="text-lg font-medium">Send these to them</h2>
          <p className="text-sm text-muted-foreground mt-1">
            They sign in at this address with this username and the password you
            set.
            {created.requirePasswordReset &&
              " They'll choose a new password the first time they sign in."}
          </p>
        </div>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <p className="text-sm font-medium">Server URL</p>
            <CopyField label="Server URL" value={window.location.origin} />
          </div>
          <div className="space-y-1.5">
            <p className="text-sm font-medium">Username</p>
            <CopyField label="Username" value={created.user.username} />
          </div>
        </div>
        <div className="flex flex-col-reverse gap-2 pt-2 sm:flex-row sm:justify-end">
          <Button onClick={reset} variant="outline">
            Add another user
          </Button>
          <Button asChild>
            <Link to={`/admin/users/${created.user.id}`}>View user</Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="space-y-4">
        <h2 className="text-lg font-medium">Account Information</h2>

        <div className="space-y-2">
          <Label htmlFor="username">Username</Label>
          <Input
            id="username"
            onChange={(e) => setUsername(e.target.value)}
            placeholder="Enter username"
            value={username}
          />
          <p className="text-xs text-muted-foreground">At least 3 characters</p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="email">
            Email <span className="text-muted-foreground">(optional)</span>
          </Label>
          <Input
            id="email"
            onChange={(e) => setEmail(e.target.value)}
            placeholder="Enter email"
            type="email"
            value={email}
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="password">Password</Label>
          <Input
            id="password"
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Enter password"
            type="password"
            value={password}
          />
          <p className="text-xs text-muted-foreground">At least 8 characters</p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="confirm-password">Confirm Password</Label>
          <Input
            id="confirm-password"
            onChange={(e) => setConfirmPassword(e.target.value)}
            placeholder="Confirm password"
            type="password"
            value={confirmPassword}
          />
        </div>

        <div className="flex items-center space-x-2">
          <Checkbox
            checked={requirePasswordReset}
            id="require-password-reset"
            onCheckedChange={(checked) =>
              setRequirePasswordReset(checked === true)
            }
          />
          <Label
            className="text-sm font-normal cursor-pointer"
            htmlFor="require-password-reset"
          >
            Require password reset on first login
          </Label>
        </div>
      </div>

      <Separator />

      <UserAccessFields
        idPrefix="password"
        onChange={setAccess}
        value={access}
      />

      <Separator />

      <div className="flex justify-end pt-4">
        <Button disabled={createUserMutation.isPending} onClick={handleCreate}>
          {createUserMutation.isPending ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Creating...
            </>
          ) : (
            "Create User"
          )}
        </Button>
      </div>
    </div>
  );
};

export default SetPasswordForm;
