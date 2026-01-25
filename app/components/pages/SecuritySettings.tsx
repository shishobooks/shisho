import { ArrowLeft, Copy, Key, Trash2 } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import {
  useAddApiKeyPermission,
  useApiKeys,
  useClearKoboSync,
  useCreateApiKey,
  useDeleteApiKey,
  useGenerateShortUrl,
  useRemoveApiKeyPermission,
} from "@/hooks/queries/apiKeys";
import { useLibraries } from "@/hooks/queries/libraries";
import { useListLists } from "@/hooks/queries/lists";
import { useResetPassword } from "@/hooks/queries/users";
import { useAuth } from "@/hooks/useAuth";
import {
  PermissionEReaderBrowser,
  PermissionKoboSync,
  type APIKey,
  type APIKeyShortURL,
} from "@/types/generated/apikeys";

const SecuritySettings = () => {
  const { user } = useAuth();
  const resetPasswordMutation = useResetPassword();

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  const handleResetPassword = async () => {
    if (!currentPassword) {
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
          current_password: currentPassword,
          new_password: newPassword,
        },
      });
      toast.success("Password changed successfully");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    } catch {
      toast.error("Failed to change password");
    }
  };

  return (
    <div>
      <TopNav />
      <div className="mx-auto w-full max-w-7xl px-6 py-8">
        <div className="mb-6">
          <Button asChild variant="ghost">
            <Link to="/">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back
            </Link>
          </Button>
        </div>

        <div className="mb-8">
          <h1 className="mb-2 text-2xl font-semibold">Security Settings</h1>
          <p className="text-muted-foreground">
            Manage your password and API keys
          </p>
        </div>

        <div className="max-w-2xl space-y-6">
          {/* Password Change */}
          <div className="rounded-md border border-border p-6">
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

          <Separator />

          {/* API Keys */}
          <ApiKeysSection />
        </div>
      </div>
    </div>
  );
};

function ApiKeysSection() {
  const { data: apiKeys, isLoading } = useApiKeys();
  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  return (
    <div className="rounded-md border border-border p-6">
      <div className="mb-4 flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">API Keys</h2>
          <p className="text-sm text-muted-foreground">
            Use API keys to access your library from an eReader browser
          </p>
        </div>
        <CreateApiKeyDialog
          onOpenChange={setCreateDialogOpen}
          open={createDialogOpen}
        />
      </div>

      {isLoading ? (
        <div className="py-4 text-muted-foreground">Loading...</div>
      ) : (
        <div className="space-y-4">
          {apiKeys?.map((key) => <ApiKeyCard apiKey={key} key={key.id} />)}
          {apiKeys?.length === 0 && (
            <p className="py-4 text-muted-foreground">
              No API keys yet. Create one to access your library from an
              eReader.
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function CreateApiKeyDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [name, setName] = useState("");
  const [enableEReader, setEnableEReader] = useState(true);
  const [enableKoboSync, setEnableKoboSync] = useState(false);
  const createApiKey = useCreateApiKey();
  const addPermission = useAddApiKeyPermission();

  const handleCreate = async () => {
    if (!name.trim()) {
      toast.error("Name is required");
      return;
    }

    try {
      const apiKey = await createApiKey.mutateAsync(name);
      if (enableEReader) {
        await addPermission.mutateAsync({
          id: apiKey.id,
          permission: PermissionEReaderBrowser,
        });
      }
      if (enableKoboSync) {
        await addPermission.mutateAsync({
          id: apiKey.id,
          permission: PermissionKoboSync,
        });
      }
      toast.success("API key created");
      setName("");
      setEnableEReader(true);
      setEnableKoboSync(false);
      onOpenChange(false);
    } catch {
      toast.error("Failed to create API key");
    }
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogTrigger asChild>
        <Button size="sm">
          <Key className="mr-2 h-4 w-4" />
          Create API Key
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create API Key</DialogTitle>
          <DialogDescription>
            Create a new API key to access your library from an eReader.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="key-name">Name</Label>
            <Input
              id="key-name"
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g., My Kobo"
              value={name}
            />
          </div>
          <div className="flex items-center space-x-2">
            <Checkbox
              checked={enableEReader}
              id="enable-ereader"
              onCheckedChange={(checked) => setEnableEReader(checked === true)}
            />
            <Label className="cursor-pointer" htmlFor="enable-ereader">
              Enable eReader browser access
            </Label>
          </div>
          <div className="flex items-center space-x-2">
            <Checkbox
              checked={enableKoboSync}
              id="enable-kobo-sync"
              onCheckedChange={(checked) => setEnableKoboSync(checked === true)}
            />
            <Label className="cursor-pointer" htmlFor="enable-kobo-sync">
              Enable Kobo wireless sync
            </Label>
          </div>
        </div>
        <DialogFooter>
          <Button
            disabled={createApiKey.isPending || addPermission.isPending}
            onClick={handleCreate}
          >
            {createApiKey.isPending || addPermission.isPending
              ? "Creating..."
              : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ApiKeyCard({ apiKey }: { apiKey: APIKey }) {
  const [setupDialogOpen, setSetupDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [koboSetupDialogOpen, setKoboSetupDialogOpen] = useState(false);
  const deleteApiKey = useDeleteApiKey();
  const addPermission = useAddApiKeyPermission();
  const removePermission = useRemoveApiKeyPermission();

  const hasEReaderPermission = apiKey.permissions?.some(
    (p) => p?.permission === PermissionEReaderBrowser,
  );
  const hasKoboSyncPermission = apiKey.permissions?.some(
    (p) => p?.permission === PermissionKoboSync,
  );

  const handleToggleEReader = async (checked: boolean) => {
    try {
      if (checked) {
        await addPermission.mutateAsync({
          id: apiKey.id,
          permission: PermissionEReaderBrowser,
        });
      } else {
        await removePermission.mutateAsync({
          id: apiKey.id,
          permission: PermissionEReaderBrowser,
        });
      }
    } catch {
      toast.error("Failed to update permission");
    }
  };

  const handleToggleKoboSync = async (checked: boolean) => {
    try {
      if (checked) {
        await addPermission.mutateAsync({
          id: apiKey.id,
          permission: PermissionKoboSync,
        });
      } else {
        await removePermission.mutateAsync({
          id: apiKey.id,
          permission: PermissionKoboSync,
        });
      }
    } catch {
      toast.error("Failed to update permission");
    }
  };

  const handleDelete = async () => {
    try {
      await deleteApiKey.mutateAsync(apiKey.id);
      toast.success("API key deleted");
      setDeleteDialogOpen(false);
    } catch {
      toast.error("Failed to delete API key");
    }
  };

  const handleCopyKey = () => {
    navigator.clipboard.writeText(apiKey.key);
    toast.success("API key copied to clipboard");
  };

  return (
    <>
      <div className="rounded-md border border-border p-4">
        <div className="mb-3 flex items-center justify-between">
          <div>
            <h3 className="font-medium">{apiKey.name}</h3>
            <p className="text-xs text-muted-foreground">
              Created {new Date(apiKey.createdAt).toLocaleDateString()}
              {apiKey.lastAccessedAt &&
                ` • Last used ${new Date(apiKey.lastAccessedAt).toLocaleDateString()}`}
            </p>
          </div>
          <div className="flex gap-2">
            <SetupDialog
              apiKey={apiKey}
              onOpenChange={setSetupDialogOpen}
              open={setupDialogOpen}
            />
            {hasKoboSyncPermission && (
              <KoboSetupDialog
                apiKey={apiKey}
                onOpenChange={setKoboSetupDialogOpen}
                open={koboSetupDialogOpen}
              />
            )}
            <Button
              onClick={() => setDeleteDialogOpen(true)}
              size="sm"
              variant="ghost"
            >
              <Trash2 className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        </div>

        <div className="mb-3 flex items-center gap-2">
          <code className="flex-1 rounded bg-muted px-2 py-1 font-mono text-sm">
            {apiKey.key.slice(0, 8)}...{apiKey.key.slice(-4)}
          </code>
          <Button onClick={handleCopyKey} size="sm" variant="outline">
            <Copy className="h-4 w-4" />
          </Button>
        </div>

        <div className="flex items-center space-x-2">
          <Checkbox
            checked={hasEReaderPermission}
            disabled={addPermission.isPending || removePermission.isPending}
            id={`ereader-${apiKey.id}`}
            onCheckedChange={handleToggleEReader}
          />
          <Label
            className="cursor-pointer text-sm"
            htmlFor={`ereader-${apiKey.id}`}
          >
            eReader browser access
          </Label>
        </div>
        <div className="mt-2 flex items-center space-x-2">
          <Checkbox
            checked={hasKoboSyncPermission}
            disabled={addPermission.isPending || removePermission.isPending}
            id={`kobo-sync-${apiKey.id}`}
            onCheckedChange={handleToggleKoboSync}
          />
          <Label
            className="cursor-pointer text-sm"
            htmlFor={`kobo-sync-${apiKey.id}`}
          >
            Kobo wireless sync
          </Label>
        </div>
      </div>

      <ConfirmDialog
        confirmLabel="Delete"
        description={`Are you sure you want to delete the API key "${apiKey.name}"?`}
        isPending={deleteApiKey.isPending}
        onConfirm={handleDelete}
        onOpenChange={setDeleteDialogOpen}
        open={deleteDialogOpen}
        title="Delete API Key"
      />
    </>
  );
}

function SetupDialog({
  apiKey,
  open,
  onOpenChange,
}: {
  apiKey: APIKey;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const generateShortUrl = useGenerateShortUrl();
  const [shortUrl, setShortUrl] = useState<APIKeyShortURL | null>(null);

  const handleOpen = async (isOpen: boolean) => {
    onOpenChange(isOpen);
    if (isOpen && !shortUrl) {
      try {
        const result = await generateShortUrl.mutateAsync(apiKey.id);
        setShortUrl(result);
      } catch {
        toast.error("Failed to generate setup URL");
      }
    }
  };

  const handleCopy = () => {
    if (shortUrl) {
      const url = `${window.location.origin}/e/${shortUrl.shortCode}`;
      navigator.clipboard.writeText(url);
      toast.success("Copied to clipboard");
    }
  };

  return (
    <Dialog onOpenChange={handleOpen} open={open}>
      <DialogTrigger asChild>
        <Button size="sm" variant="outline">
          Setup
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Setup on eReader</DialogTitle>
          <DialogDescription>
            Enter this URL on your eReader's web browser, then bookmark the
            page.
          </DialogDescription>
        </DialogHeader>
        {generateShortUrl.isPending ? (
          <div className="py-4">Generating URL...</div>
        ) : shortUrl ? (
          <div className="space-y-4 py-4">
            <div className="flex gap-2">
              <Input
                className="font-mono"
                readOnly
                value={`${window.location.origin}/e/${shortUrl.shortCode}`}
              />
              <Button onClick={handleCopy} variant="outline">
                <Copy className="h-4 w-4" />
              </Button>
            </div>
            <p className="text-sm text-muted-foreground">
              This URL expires in 30 minutes. After opening it on your eReader,
              bookmark the page to access your library anytime.
            </p>
          </div>
        ) : null}
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function KoboSetupDialog({
  apiKey,
  open,
  onOpenChange,
}: {
  apiKey: APIKey;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [scopeType, setScopeType] = useState<"all" | "library" | "list">("all");
  const [scopeId, setScopeId] = useState("");
  const { data: librariesData } = useLibraries();
  const { data: listsData } = useListLists();
  const clearKoboSync = useClearKoboSync();

  const handleResetSync = async () => {
    try {
      await clearKoboSync.mutateAsync(apiKey.id);
      toast.success("Sync history cleared. Next sync will be a fresh sync.");
    } catch {
      toast.error("Failed to clear sync history");
    }
  };

  const buildSyncURL = () => {
    const origin = window.location.origin;
    let scopePath: string;
    switch (scopeType) {
      case "library":
        scopePath = `library/${scopeId}`;
        break;
      case "list":
        scopePath = `list/${scopeId}`;
        break;
      default:
        scopePath = "all";
    }
    return `${origin}/kobo/${apiKey.key}/${scopePath}`;
  };

  const syncURL = buildSyncURL();

  const handleCopy = () => {
    navigator.clipboard.writeText(syncURL);
    toast.success("Copied to clipboard");
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogTrigger asChild>
        <Button size="sm" variant="outline">
          Kobo Setup
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Kobo Sync Setup</DialogTitle>
          <DialogDescription>
            Configure your Kobo device to sync books wirelessly from Shisho.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-4">
          {/* Scope Selection */}
          <div className="space-y-3">
            <Label>Sync Scope</Label>
            <div className="flex rounded-md border border-input">
              <button
                className={`flex-1 px-3 py-2 text-sm font-medium transition-colors first:rounded-l-md last:rounded-r-md ${
                  scopeType === "all"
                    ? "bg-primary text-primary-foreground"
                    : "hover:bg-muted"
                }`}
                onClick={() => {
                  setScopeType("all");
                  setScopeId("");
                }}
                type="button"
              >
                All Libraries
              </button>
              <button
                className={`flex-1 border-x border-input px-3 py-2 text-sm font-medium transition-colors ${
                  scopeType === "library"
                    ? "bg-primary text-primary-foreground"
                    : "hover:bg-muted"
                }`}
                onClick={() => {
                  setScopeType("library");
                  setScopeId("");
                }}
                type="button"
              >
                Library
              </button>
              <button
                className={`flex-1 px-3 py-2 text-sm font-medium transition-colors first:rounded-l-md last:rounded-r-md ${
                  scopeType === "list"
                    ? "bg-primary text-primary-foreground"
                    : "hover:bg-muted"
                }`}
                onClick={() => {
                  setScopeType("list");
                  setScopeId("");
                }}
                type="button"
              >
                List
              </button>
            </div>
            {scopeType === "library" && librariesData && (
              <Select onValueChange={setScopeId} value={scopeId}>
                <SelectTrigger>
                  <SelectValue placeholder="Select a library..." />
                </SelectTrigger>
                <SelectContent>
                  {librariesData.libraries.map((lib) => (
                    <SelectItem key={lib.id} value={String(lib.id)}>
                      {lib.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            {scopeType === "list" && listsData && (
              <Select onValueChange={setScopeId} value={scopeId}>
                <SelectTrigger>
                  <SelectValue placeholder="Select a list..." />
                </SelectTrigger>
                <SelectContent>
                  {listsData.lists.map((list) => (
                    <SelectItem key={list.id} value={String(list.id)}>
                      {list.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>

          {/* Generated URL */}
          <div className="space-y-2">
            <Label>API Endpoint URL</Label>
            <div className="flex gap-2">
              <Input className="font-mono text-xs" readOnly value={syncURL} />
              <Button onClick={handleCopy} size="sm" variant="outline">
                <Copy className="h-4 w-4" />
              </Button>
            </div>
          </div>

          {/* Setup Instructions */}
          <div className="space-y-2">
            <Label>Setup Instructions</Label>
            <ol className="list-decimal space-y-1 pl-5 text-sm text-muted-foreground">
              <li>Connect your Kobo via USB</li>
              <li>
                Navigate to{" "}
                <code className="rounded bg-muted px-1">
                  .kobo/Kobo/Kobo eReader.conf
                </code>
              </li>
              <li>
                Find{" "}
                <code className="rounded bg-muted px-1">
                  api_endpoint=https://storeapi.kobo.com
                </code>
              </li>
              <li>Replace with the URL above</li>
              <li>Eject the Kobo and sync</li>
            </ol>
          </div>

          {/* Reset Sync */}
          <Separator />
          <div className="flex items-center justify-between">
            <div>
              <Label>Reset Sync</Label>
              <p className="text-sm text-muted-foreground">
                Clear sync history to force a fresh sync
              </p>
            </div>
            <Button
              disabled={clearKoboSync.isPending}
              onClick={handleResetSync}
              size="sm"
              variant="outline"
            >
              {clearKoboSync.isPending ? "Resetting..." : "Reset"}
            </Button>
          </div>
        </div>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} variant="outline">
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default SecuritySettings;
