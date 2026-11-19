import { Loader2 } from "lucide-react";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import LoadingSpinner from "@/components/library/LoadingSpinner";
import Logo from "@/components/library/Logo";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Toaster } from "@/components/ui/sonner";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";
import { useAcceptInvite, useInvite } from "@/hooks/queries/invites";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import {
  isDemoModeError,
  isNotFoundError,
  ShishoAPIError,
  toastRequestError,
} from "@/libraries/api";
import { cn } from "@/libraries/utils";
import type { MeResponse } from "@/types";

const INITIAL_VALUES = {
  username: "",
  email: "",
  password: "",
  confirmPassword: "",
};

// The page frame: the logo above a narrow column, centered on any width.
const Frame = ({ children }: { children: ReactNode }) => (
  <div className="min-h-dvh flex items-center justify-center bg-background px-4 py-8">
    <Toaster richColors />
    <div className="w-full max-w-md">
      <div className="flex justify-center mb-6 md:mb-8">
        <Logo size="lg" />
      </div>
      {children}
    </div>
  </div>
);

const Message = ({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) => (
  <div className="text-center space-y-2">
    <h1 className="text-xl font-semibold">{title}</h1>
    <div className="text-muted-foreground space-y-4">{children}</div>
  </div>
);

// The one page for an invite that can't be accepted: expired, revoked, used,
// or unknown. It never says which. When the server knows who sent it, it
// names them.
const InviteUnavailable = ({ invitedBy }: { invitedBy?: string }) => (
  <Message title="This invite link is no longer valid">
    <p>
      Ask{" "}
      {invitedBy ? (
        <span className="font-medium text-foreground break-words">
          {invitedBy}
        </span>
      ) : (
        "the person who invited you"
      )}{" "}
      for a new one.
    </p>
  </Message>
);

interface AcceptFormProps {
  token: string;
  invitedBy: string;
  onUnavailable: () => void;
  /** Called with the new session's user once the account exists. */
  onAccepted: (me: MeResponse) => void;
}

const AcceptForm = ({
  token,
  invitedBy,
  onUnavailable,
  onAccepted,
}: AcceptFormProps) => {
  const acceptMutation = useAcceptInvite(token);

  const [username, setUsername] = useState(INITIAL_VALUES.username);
  const [email, setEmail] = useState(INITIAL_VALUES.email);
  const [password, setPassword] = useState(INITIAL_VALUES.password);
  const [confirmPassword, setConfirmPassword] = useState(
    INITIAL_VALUES.confirmPassword,
  );
  const [usernameError, setUsernameError] = useState<string | null>(null);
  const [emailError, setEmailError] = useState<string | null>(null);

  const hasUnsavedChanges = useMemo(
    () =>
      username.trim() !== INITIAL_VALUES.username ||
      email.trim() !== INITIAL_VALUES.email ||
      password !== INITIAL_VALUES.password ||
      confirmPassword !== INITIAL_VALUES.confirmPassword,
    [username, email, password, confirmPassword],
  );
  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(hasUnsavedChanges);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (username.trim().length < 3) {
      toast.error("Username must be at least 3 characters");
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

    try {
      const me = await acceptMutation.mutateAsync({
        username: username.trim(),
        email: email.trim() || undefined,
        password,
      });
      onAccepted(me);
    } catch (error) {
      if (error instanceof ShishoAPIError && error.code === "username_taken") {
        setUsernameError("That username is taken. Pick another one.");
      } else if (
        error instanceof ShishoAPIError &&
        error.code === "email_taken"
      ) {
        setEmailError("That email address is already in use.");
      } else if (isNotFoundError(error)) {
        // Used, revoked, or expired while the form was open.
        onUnavailable();
      } else {
        toastRequestError(error, "Could not create your account");
      }
    }
  };

  return (
    <>
      <div className="text-center mb-6 space-y-2">
        <h1 className="text-xl font-semibold break-words">
          {invitedBy} invited you to their Shisho library
        </h1>
        <p className="text-muted-foreground">
          Shisho is where they keep their books, comics, and audiobooks. Read in
          your browser or send books to your e-reader.
        </p>
      </div>

      <form className="space-y-6" noValidate onSubmit={handleSubmit}>
        <div className="space-y-2">
          <Label htmlFor="username">Username</Label>
          <Input
            aria-describedby={usernameError ? "username-error" : undefined}
            aria-invalid={usernameError ? true : undefined}
            autoComplete="username"
            autoFocus
            className={cn(usernameError && "border-destructive")}
            id="username"
            onChange={(e) => {
              setUsername(e.target.value);
              setUsernameError(null);
            }}
            placeholder="Choose a username"
            type="text"
            value={username}
          />
          {usernameError ? (
            <p className="text-xs text-destructive" id="username-error">
              {usernameError}
            </p>
          ) : (
            <p className="text-xs text-muted-foreground">
              At least 3 characters
            </p>
          )}
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
          <p className="text-xs text-muted-foreground">At least 8 characters</p>
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

        <div className="space-y-2">
          <Label htmlFor="email">
            Email <span className="text-muted-foreground">(optional)</span>
          </Label>
          <Input
            aria-describedby={emailError ? "email-error" : undefined}
            aria-invalid={emailError ? true : undefined}
            autoComplete="email"
            className={cn(emailError && "border-destructive")}
            id="email"
            onChange={(e) => {
              setEmail(e.target.value);
              setEmailError(null);
            }}
            placeholder="Enter your email"
            type="email"
            value={email}
          />
          {emailError && (
            <p className="text-xs text-destructive" id="email-error">
              {emailError}
            </p>
          )}
        </div>

        <Button
          className="w-full"
          disabled={acceptMutation.isPending}
          type="submit"
        >
          {acceptMutation.isPending ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Creating account...
            </>
          ) : (
            "Create account"
          )}
        </Button>
      </form>

      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />
    </>
  );
};

// The welcome page an Invite Link opens. It sits outside the protected route
// tree, so someone without an account is never sent to sign in. Someone
// already signed in is offered to sign out first, so accepting never
// switches accounts by accident.
const AcceptInvite = () => {
  usePageTitle("Invite");

  const { token } = useParams<{ token: string }>();
  const navigate = useNavigate();
  const {
    user,
    isLoading: authLoading,
    logout,
    demoMode,
    setAuthUser,
  } = useAuth();
  const [accepted, setAccepted] = useState(false);
  // Storing the new session clears every cached query, so the lookup stays
  // off once the invite is used.
  const inviteQuery = useInvite(token, { enabled: !accepted });
  const [unavailable, setUnavailable] = useState(false);
  const [signingOut, setSigningOut] = useState(false);

  // The form, and its unsaved-changes blocker, are gone by the time this
  // runs, so nothing holds the navigation. Getting Started opens on its own
  // for a new user.
  useEffect(() => {
    if (accepted) navigate("/", { replace: true });
  }, [accepted, navigate]);

  const handleAccepted = (me: MeResponse) => {
    setAccepted(true);
    setAuthUser(me);
  };
  const invite = inviteQuery.data;

  const markUnavailable = () => {
    setUnavailable(true);
    void inviteQuery.refetch();
  };

  const handleSignOut = async () => {
    setSigningOut(true);
    try {
      await logout();
    } finally {
      setSigningOut(false);
    }
  };

  let content: ReactNode;
  if (accepted) {
    content = <LoadingSpinner />;
  } else if (demoMode || isDemoModeError(inviteQuery.error)) {
    content = (
      <Message title="Invites aren't available in the demo">
        <p>This is a read-only demo of Shisho, so it can't add accounts.</p>
      </Message>
    );
  } else if (authLoading || (token && inviteQuery.isLoading)) {
    content = <LoadingSpinner />;
  } else if (!token || isNotFoundError(inviteQuery.error)) {
    content = <InviteUnavailable />;
  } else if (inviteQuery.isError || !invite) {
    // The server failed or couldn't be reached; the invite may be fine.
    content = (
      <Message title="Could not load this invite">
        <p>The server did not respond. Check your connection and try again.</p>
        <Button
          disabled={inviteQuery.isFetching}
          onClick={() => inviteQuery.refetch()}
          variant="outline"
        >
          Try again
        </Button>
      </Message>
    );
  } else if (!invite.valid || unavailable) {
    content = <InviteUnavailable invitedBy={invite.invited_by} />;
  } else if (user) {
    content = (
      <Message
        title={`${invite.invited_by} invited you to their Shisho library`}
      >
        <p>
          You&apos;re signed in as{" "}
          <span className="font-medium text-foreground break-words">
            {user.username}
          </span>
          . Sign out to accept this invite.
        </p>
        <Button disabled={signingOut} onClick={handleSignOut}>
          {signingOut && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Sign out
        </Button>
      </Message>
    );
  } else {
    content = (
      <AcceptForm
        invitedBy={invite.invited_by}
        onAccepted={handleAccepted}
        onUnavailable={markUnavailable}
        token={token}
      />
    );
  }

  return <Frame>{content}</Frame>;
};

export default AcceptInvite;
