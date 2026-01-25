import { ArrowLeft, Copy, Plus, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";
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

          {/* eReader Browser Access */}
          <EReaderSection />

          <Separator />

          {/* Kobo Wireless Sync */}
          <KoboSyncSection />
        </div>
      </div>
    </div>
  );
};

function EReaderSection() {
  const { data: apiKeys, isLoading } = useApiKeys();
  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  const eReaderKeys = apiKeys?.filter((key) =>
    key.permissions?.some((p) => p?.permission === PermissionEReaderBrowser),
  );

  return (
    <div className="rounded-md border border-border p-6">
      <div className="mb-4 flex items-start justify-between">
        <div>
          <h2 className="text-lg font-semibold">eReader Browser Access</h2>
          <p className="text-sm text-muted-foreground">
            Browse and download books from your eReader's web browser
          </p>
        </div>
        <CreateEReaderKeyDialog
          onOpenChange={setCreateDialogOpen}
          open={createDialogOpen}
        />
      </div>

      {isLoading ? (
        <div className="py-6 text-center text-sm text-muted-foreground">
          Loading...
        </div>
      ) : eReaderKeys?.length === 0 ? (
        <div className="rounded-md border border-dashed border-border py-6 text-center">
          <p className="text-sm text-muted-foreground">
            No devices configured. Add one to browse your library from an
            eReader.
          </p>
        </div>
      ) : (
        <div className="divide-y divide-border">
          {eReaderKeys?.map((key) => (
            <EReaderKeyRow apiKey={key} key={key.id} />
          ))}
        </div>
      )}
    </div>
  );
}

function CreateEReaderKeyDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [name, setName] = useState("");
  const createApiKey = useCreateApiKey();
  const addPermission = useAddApiKeyPermission();

  const handleCreate = async () => {
    if (!name.trim()) {
      toast.error("Name is required");
      return;
    }

    try {
      const apiKey = await createApiKey.mutateAsync(name);
      await addPermission.mutateAsync({
        id: apiKey.id,
        permission: PermissionEReaderBrowser,
      });
      toast.success("Device added");
      setName("");
      onOpenChange(false);
    } catch {
      toast.error("Failed to add device");
    }
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogTrigger asChild>
        <Button size="sm">
          <Plus className="mr-2 h-4 w-4" />
          Add Device
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add eReader Device</DialogTitle>
          <DialogDescription>
            Give this device a name so you can identify it later.
          </DialogDescription>
        </DialogHeader>
        <div className="py-4">
          <div className="space-y-2">
            <Label htmlFor="ereader-name">Device Name</Label>
            <Input
              id="ereader-name"
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleCreate();
              }}
              placeholder="e.g., Bedroom Kindle"
              value={name}
            />
          </div>
        </div>
        <DialogFooter>
          <Button
            disabled={createApiKey.isPending || addPermission.isPending}
            onClick={handleCreate}
          >
            {createApiKey.isPending || addPermission.isPending
              ? "Adding..."
              : "Add Device"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function EReaderKeyRow({ apiKey }: { apiKey: APIKey }) {
  const [setupDialogOpen, setSetupDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const deleteApiKey = useDeleteApiKey();

  const handleDelete = async () => {
    try {
      await deleteApiKey.mutateAsync(apiKey.id);
      toast.success("Device removed");
      setDeleteDialogOpen(false);
    } catch {
      toast.error("Failed to remove device");
    }
  };

  return (
    <>
      <div className="flex items-center justify-between py-3 first:pt-0 last:pb-0">
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium">{apiKey.name}</p>
          <p className="text-xs text-muted-foreground">
            Added {new Date(apiKey.createdAt).toLocaleDateString()}
            {apiKey.lastAccessedAt &&
              ` · Last used ${new Date(apiKey.lastAccessedAt).toLocaleDateString()}`}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            onClick={() => setSetupDialogOpen(true)}
            size="sm"
            variant="outline"
          >
            Setup
          </Button>
          <Button
            onClick={() => setDeleteDialogOpen(true)}
            size="sm"
            variant="ghost"
          >
            <Trash2 className="h-4 w-4 text-destructive" />
          </Button>
        </div>
      </div>

      <EReaderSetupDialog
        apiKey={apiKey}
        onOpenChange={setSetupDialogOpen}
        open={setupDialogOpen}
      />
      <ConfirmDialog
        confirmLabel="Remove"
        description={`Are you sure you want to remove "${apiKey.name}"? You'll need to set it up again to use it.`}
        isPending={deleteApiKey.isPending}
        onConfirm={handleDelete}
        onOpenChange={setDeleteDialogOpen}
        open={deleteDialogOpen}
        title="Remove Device"
      />
    </>
  );
}

function EReaderSetupDialog({
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

  useEffect(() => {
    if (open && !shortUrl) {
      generateShortUrl
        .mutateAsync(apiKey.id)
        .then(setShortUrl)
        .catch(() => toast.error("Failed to generate setup URL"));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const handleCopy = () => {
    if (shortUrl) {
      const url = `${window.location.origin}/e/${shortUrl.shortCode}`;
      navigator.clipboard.writeText(url);
      toast.success("Copied to clipboard");
    }
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
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

function KoboSyncSection() {
  const { data: apiKeys, isLoading } = useApiKeys();
  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  const koboKeys = apiKeys?.filter((key) =>
    key.permissions?.some((p) => p?.permission === PermissionKoboSync),
  );

  return (
    <div className="rounded-md border border-border p-6">
      <div className="mb-4 flex items-start justify-between">
        <div>
          <h2 className="text-lg font-semibold">Kobo Wireless Sync</h2>
          <p className="text-sm text-muted-foreground">
            Sync books directly to your Kobo device over WiFi
          </p>
        </div>
        <CreateKoboKeyDialog
          onOpenChange={setCreateDialogOpen}
          open={createDialogOpen}
        />
      </div>

      {isLoading ? (
        <div className="py-6 text-center text-sm text-muted-foreground">
          Loading...
        </div>
      ) : koboKeys?.length === 0 ? (
        <div className="rounded-md border border-dashed border-border py-6 text-center">
          <p className="text-sm text-muted-foreground">
            No Kobo devices configured. Add one to sync books wirelessly.
          </p>
        </div>
      ) : (
        <div className="divide-y divide-border">
          {koboKeys?.map((key) => (
            <KoboKeyRow apiKey={key} key={key.id} />
          ))}
        </div>
      )}
    </div>
  );
}

function CreateKoboKeyDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [name, setName] = useState("");
  const createApiKey = useCreateApiKey();
  const addPermission = useAddApiKeyPermission();

  const handleCreate = async () => {
    if (!name.trim()) {
      toast.error("Name is required");
      return;
    }

    try {
      const apiKey = await createApiKey.mutateAsync(name);
      await addPermission.mutateAsync({
        id: apiKey.id,
        permission: PermissionKoboSync,
      });
      toast.success("Kobo device added");
      setName("");
      onOpenChange(false);
    } catch {
      toast.error("Failed to add device");
    }
  };

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogTrigger asChild>
        <Button size="sm">
          <Plus className="mr-2 h-4 w-4" />
          Add Kobo
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add Kobo Device</DialogTitle>
          <DialogDescription>
            Give this Kobo a name so you can identify it later.
          </DialogDescription>
        </DialogHeader>
        <div className="py-4">
          <div className="space-y-2">
            <Label htmlFor="kobo-name">Device Name</Label>
            <Input
              id="kobo-name"
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleCreate();
              }}
              placeholder="e.g., Kobo Libra 2"
              value={name}
            />
          </div>
        </div>
        <DialogFooter>
          <Button
            disabled={createApiKey.isPending || addPermission.isPending}
            onClick={handleCreate}
          >
            {createApiKey.isPending || addPermission.isPending
              ? "Adding..."
              : "Add Kobo"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function KoboKeyRow({ apiKey }: { apiKey: APIKey }) {
  const [setupDialogOpen, setSetupDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const deleteApiKey = useDeleteApiKey();

  const handleDelete = async () => {
    try {
      await deleteApiKey.mutateAsync(apiKey.id);
      toast.success("Kobo device removed");
      setDeleteDialogOpen(false);
    } catch {
      toast.error("Failed to remove device");
    }
  };

  return (
    <>
      <div className="flex items-center justify-between py-3 first:pt-0 last:pb-0">
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium">{apiKey.name}</p>
          <p className="text-xs text-muted-foreground">
            Added {new Date(apiKey.createdAt).toLocaleDateString()}
            {apiKey.lastAccessedAt &&
              ` · Last synced ${new Date(apiKey.lastAccessedAt).toLocaleDateString()}`}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            onClick={() => setSetupDialogOpen(true)}
            size="sm"
            variant="outline"
          >
            Setup
          </Button>
          <Button
            onClick={() => setDeleteDialogOpen(true)}
            size="sm"
            variant="ghost"
          >
            <Trash2 className="h-4 w-4 text-destructive" />
          </Button>
        </div>
      </div>

      <KoboSetupDialog
        apiKey={apiKey}
        onOpenChange={setSetupDialogOpen}
        open={setupDialogOpen}
      />
      <ConfirmDialog
        confirmLabel="Remove"
        description={`Are you sure you want to remove "${apiKey.name}"? You'll need to reconfigure your Kobo to use it again.`}
        isPending={deleteApiKey.isPending}
        onConfirm={handleDelete}
        onOpenChange={setDeleteDialogOpen}
        open={deleteDialogOpen}
        title="Remove Kobo Device"
      />
    </>
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
                className={`flex-1 px-3 py-2 text-sm font-medium transition-colors first:rounded-l-md last:rounded-r-md cursor-pointer ${
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
                className={`flex-1 border-x border-input px-3 py-2 text-sm font-medium transition-colors cursor-pointer ${
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
                className={`flex-1 px-3 py-2 text-sm font-medium transition-colors first:rounded-l-md last:rounded-r-md cursor-pointer ${
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
