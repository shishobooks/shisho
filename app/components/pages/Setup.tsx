import { Loader2 } from "lucide-react";
import { useMemo, useState } from "react";
import { Navigate } from "react-router-dom";
import { toast } from "sonner";

import Logo from "@/components/library/Logo";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Toaster } from "@/components/ui/sonner";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";
import { useAuth } from "@/hooks/useAuth";
import { useNavigateAfterSave } from "@/hooks/useNavigateAfterSave";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import { API } from "@/libraries/api";

interface SetupResponse {
  id: number;
  username: string;
  email?: string;
  role_id: number;
  role_name: string;
  permissions: string[];
}

// Initial values for the setup form - stored once to compare against
const INITIAL_VALUES = {
  username: "",
  email: "",
  password: "",
  confirmPassword: "",
};

const Setup = () => {
  usePageTitle("Setup");

  const { needsSetup, isLoading: authLoading, setAuthUser } = useAuth();

  const [username, setUsername] = useState(INITIAL_VALUES.username);
  const [email, setEmail] = useState(INITIAL_VALUES.email);
  const [password, setPassword] = useState(INITIAL_VALUES.password);
  const [confirmPassword, setConfirmPassword] = useState(
    INITIAL_VALUES.confirmPassword,
  );
  const [isLoading, setIsLoading] = useState(false);
  const [changesSaved, setChangesSaved] = useState(false);

  // Track whether user has unsaved changes by comparing against initial values
  const hasUnsavedChanges = useMemo(() => {
    if (changesSaved) return false;
    return (
      username.trim() !== INITIAL_VALUES.username ||
      email.trim() !== INITIAL_VALUES.email ||
      password !== INITIAL_VALUES.password ||
      confirmPassword !== INITIAL_VALUES.confirmPassword
    );
  }, [username, email, password, confirmPassword, changesSaved]);

  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(hasUnsavedChanges);

  const { requestNavigate } = useNavigateAfterSave(hasUnsavedChanges);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

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

    setIsLoading(true);
    try {
      const userData = await API.request<SetupResponse>("POST", "/auth/setup", {
        username,
        email: email.trim() || null,
        password,
      });

      toast.success("Admin account created successfully!");
      setAuthUser(userData);
      setChangesSaved(true);
      requestNavigate("/");
    } catch (error) {
      let msg = "Setup failed. Please try again.";
      if (error instanceof Error) {
        msg = error.message;
      }
      toast.error(msg);
    } finally {
      setIsLoading(false);
    }
  };

  if (authLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // Redirect if setup is not needed (user may already be authenticated)
  if (!needsSetup) {
    return <Navigate replace to="/" />;
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-background">
      <Toaster richColors />
      <div className="w-full max-w-md p-8">
        <div className="flex flex-col items-center mb-8">
          <Logo className="mb-2" size="lg" />
          <h1 className="text-xl font-semibold mb-1">Welcome!</h1>
          <p className="text-muted-foreground text-center">
            Create your admin account to get started
          </p>
        </div>

        <form className="space-y-6" onSubmit={handleSubmit}>
          <div className="space-y-2">
            <Label htmlFor="username">Username</Label>
            <Input
              autoComplete="username"
              autoFocus
              id="username"
              onChange={(e) => setUsername(e.target.value)}
              placeholder="Choose a username"
              type="text"
              value={username}
            />
            <p className="text-xs text-muted-foreground">
              At least 3 characters
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="email">
              Email <span className="text-muted-foreground">(optional)</span>
            </Label>
            <Input
              autoComplete="email"
              id="email"
              onChange={(e) => setEmail(e.target.value)}
              placeholder="Enter your email"
              type="email"
              value={email}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="password">Password</Label>
            <Input
              autoComplete="new-password"
              id="password"
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Choose a password"
              type="password"
              value={password}
            />
            <p className="text-xs text-muted-foreground">
              At least 8 characters
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="confirm-password">Confirm Password</Label>
            <Input
              autoComplete="new-password"
              id="confirm-password"
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="Confirm your password"
              type="password"
              value={confirmPassword}
            />
          </div>

          <Button className="w-full" disabled={isLoading} type="submit">
            {isLoading ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Creating account...
              </>
            ) : (
              "Create Admin Account"
            )}
          </Button>
        </form>
      </div>

      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />
    </div>
  );
};

export default Setup;
