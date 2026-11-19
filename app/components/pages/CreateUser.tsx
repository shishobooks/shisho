import { KeyRound, Link2 } from "lucide-react";
import { useCallback, useState } from "react";
import { useSearchParams } from "react-router-dom";

import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";
import InviteLinkForm from "@/components/users/InviteLinkForm";
import SetPasswordForm from "@/components/users/SetPasswordForm";
import { usePageTitle } from "@/hooks/usePageTitle";
import {
  useUnsavedChanges,
  type UseUnsavedChangesOptions,
} from "@/hooks/useUnsavedChanges";
import { cn } from "@/libraries/utils";

type Method = "invite" | "password";

const METHODS: {
  value: Method;
  label: string;
  description: string;
  icon: typeof Link2;
}[] = [
  {
    value: "invite",
    label: "Invite link",
    description:
      "Send them a link. They choose their own username and password.",
    icon: Link2,
  },
  {
    value: "password",
    label: "Set a password",
    description: "Create the account yourself and pass on its password.",
    icon: KeyRound,
  },
];

// Switching between the two forms changes only `method` in the URL, and both
// forms keep their input, so it doesn't count as leaving the page with
// unsaved changes. Any other navigation still asks.
const onlySwitchesMethod: NonNullable<
  UseUnsavedChangesOptions["allowNavigation"]
> = ({ currentLocation, nextLocation }) => {
  if (
    currentLocation.pathname !== nextLocation.pathname ||
    currentLocation.hash !== nextLocation.hash
  ) {
    return false;
  }
  const current = new URLSearchParams(currentLocation.search);
  const next = new URLSearchParams(nextLocation.search);
  current.delete("method");
  next.delete("method");
  return current.toString() === next.toString();
};

// Add User. An Invite Link is the default; setting a password is the other
// path. The chosen path is kept in the URL (`?method=password`), and both
// forms stay mounted so switching between them keeps what was typed.
const CreateUser = () => {
  usePageTitle("Add User");

  const [searchParams, setSearchParams] = useSearchParams();
  const method: Method =
    searchParams.get("method") === "password" ? "password" : "invite";
  const setMethod = (value: string) =>
    setSearchParams(value === "password" ? { method: value } : {}, {
      preventScrollReset: true,
      replace: true,
    });

  const [inviteChanges, setInviteChanges] = useState(false);
  const [passwordChanges, setPasswordChanges] = useState(false);
  const onInviteChanges = useCallback((v: boolean) => setInviteChanges(v), []);
  const onPasswordChanges = useCallback(
    (v: boolean) => setPasswordChanges(v),
    [],
  );
  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(inviteChanges || passwordChanges, {
      allowNavigation: onlySwitchesMethod,
    });

  return (
    <div>
      <div className="mb-6 md:mb-8">
        <h1 className="text-2xl font-semibold mb-2">Add User</h1>
        <p className="text-muted-foreground">
          Give someone their own account on this server.
        </p>
      </div>

      <div className="max-w-2xl space-y-6">
        <div className="space-y-3">
          <Label id="add-user-method-label">How do you want to add them?</Label>
          <RadioGroup
            aria-labelledby="add-user-method-label"
            className="grid gap-3 sm:grid-cols-2"
            name="add-user-method"
            onValueChange={setMethod}
            value={method}
          >
            {METHODS.map((option) => {
              const Icon = option.icon;
              return (
                <Label
                  className={cn(
                    "flex cursor-pointer items-start gap-3 rounded-md border border-border p-4 font-normal transition-colors hover:bg-muted/50",
                    method === option.value && "border-primary bg-primary/5",
                  )}
                  htmlFor={`add-user-method-${option.value}`}
                  key={option.value}
                >
                  {/* The card is the label, so all of it selects; the radio
                      is named by its title alone and described by the rest. */}
                  <RadioGroupItem
                    aria-describedby={`add-user-method-${option.value}-description`}
                    aria-labelledby={`add-user-method-${option.value}-title`}
                    className="mt-0.5"
                    id={`add-user-method-${option.value}`}
                    value={option.value}
                  />
                  <span className="min-w-0 space-y-1">
                    <span
                      className="flex items-center gap-2 text-sm font-medium"
                      id={`add-user-method-${option.value}-title`}
                    >
                      <Icon aria-hidden="true" className="h-4 w-4" />
                      {option.label}
                    </span>
                    <span
                      className="block text-sm text-muted-foreground"
                      id={`add-user-method-${option.value}-description`}
                    >
                      {option.description}
                    </span>
                  </span>
                </Label>
              );
            })}
          </RadioGroup>
        </div>

        <div className="border border-border rounded-md p-4 md:p-6">
          <div hidden={method !== "invite"}>
            <InviteLinkForm onHasChangesChange={onInviteChanges} />
          </div>
          <div hidden={method !== "password"}>
            <SetPasswordForm onHasChangesChange={onPasswordChanges} />
          </div>
        </div>
      </div>

      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />
    </div>
  );
};

export default CreateUser;
