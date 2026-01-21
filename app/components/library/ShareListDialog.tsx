import { Info, Loader2, X } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useCreateShare,
  useDeleteShare,
  useListShares,
  useUpdateShare,
} from "@/hooks/queries/lists";
import { useUsers } from "@/hooks/queries/users";
import {
  ListPermissionEditor,
  ListPermissionManager,
  ListPermissionViewer,
} from "@/types";

interface ShareListDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  listId: number;
  listName: string;
}

const PERMISSION_OPTIONS = [
  { value: ListPermissionViewer, label: "Viewer" },
  { value: ListPermissionEditor, label: "Editor" },
  { value: ListPermissionManager, label: "Manager" },
];

const getPermissionBadgeVariant = (permission: string) => {
  switch (permission) {
    case ListPermissionManager:
      return "default";
    case ListPermissionEditor:
      return "secondary";
    default:
      return "outline";
  }
};

export function ShareListDialog({
  open,
  onOpenChange,
  listId,
  listName,
}: ShareListDialogProps) {
  const [selectedUserId, setSelectedUserId] = useState<string>("");
  const [selectedPermission, setSelectedPermission] =
    useState<string>(ListPermissionViewer);

  const sharesQuery = useListShares(listId, { enabled: open });
  const usersQuery = useUsers({}, { enabled: open });

  const createShareMutation = useCreateShare();
  const updateShareMutation = useUpdateShare();
  const deleteShareMutation = useDeleteShare();

  const shares = sharesQuery.data ?? [];
  const users = usersQuery.data?.users ?? [];

  // Filter out users who already have shares
  const availableUsers = users.filter(
    (user) => !shares.some((share) => share.user_id === user.id),
  );

  const handleAddShare = async () => {
    if (!selectedUserId) return;

    try {
      await createShareMutation.mutateAsync({
        listId,
        payload: {
          user_id: parseInt(selectedUserId, 10),
          permission: selectedPermission,
        },
      });
      toast.success("Share added");
      setSelectedUserId("");
      setSelectedPermission(ListPermissionViewer);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to add share",
      );
    }
  };

  const handleUpdatePermission = async (
    shareId: number,
    permission: string,
  ) => {
    try {
      await updateShareMutation.mutateAsync({
        listId,
        shareId,
        payload: { permission },
      });
      toast.success("Permission updated");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to update permission",
      );
    }
  };

  const handleRemoveShare = async (shareId: number) => {
    try {
      await deleteShareMutation.mutateAsync({ listId, shareId });
      toast.success("Share removed");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to remove share",
      );
    }
  };

  const isPending =
    createShareMutation.isPending ||
    updateShareMutation.isPending ||
    deleteShareMutation.isPending;

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Share List</DialogTitle>
          <DialogDescription>
            Manage who has access to "{listName}"
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-4">
          {/* Add New Share Section */}
          <div className="space-y-3">
            <h3 className="text-sm font-medium">Add User</h3>
            <div className="flex gap-2">
              <Select
                disabled={usersQuery.isLoading || availableUsers.length === 0}
                onValueChange={setSelectedUserId}
                value={selectedUserId}
              >
                <SelectTrigger className="flex-1">
                  <SelectValue
                    placeholder={
                      availableUsers.length === 0
                        ? "No users available"
                        : "Select user..."
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  {availableUsers.map((user) => (
                    <SelectItem key={user.id} value={String(user.id)}>
                      {user.username}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Select
                onValueChange={setSelectedPermission}
                value={selectedPermission}
              >
                <SelectTrigger className="w-28">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PERMISSION_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Button
                disabled={!selectedUserId || isPending}
                onClick={handleAddShare}
                size="default"
              >
                {createShareMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  "Share"
                )}
              </Button>
            </div>
          </div>

          {/* Current Shares Section */}
          <div className="space-y-3">
            <h3 className="text-sm font-medium">Current Shares</h3>
            {sharesQuery.isLoading ? (
              <div className="flex items-center justify-center py-4">
                <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
              </div>
            ) : shares.length === 0 ? (
              <p className="text-sm text-muted-foreground py-2">
                This list hasn't been shared with anyone yet.
              </p>
            ) : (
              <div className="space-y-2">
                {shares.map((share) => (
                  <div
                    className="flex items-center justify-between gap-2 py-2 px-3 rounded-md border bg-muted/30"
                    key={share.id}
                  >
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-medium truncate">
                          {share.user?.username ?? `User ${share.user_id}`}
                        </span>
                        <Badge
                          variant={getPermissionBadgeVariant(share.permission)}
                        >
                          {share.permission}
                        </Badge>
                      </div>
                      {share.shared_by_user && (
                        <p className="text-xs text-muted-foreground">
                          Shared by {share.shared_by_user.username}
                        </p>
                      )}
                    </div>

                    <div className="flex items-center gap-1">
                      <Select
                        disabled={isPending}
                        onValueChange={(value) =>
                          handleUpdatePermission(share.id, value)
                        }
                        value={share.permission}
                      >
                        <SelectTrigger className="w-24 h-8">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {PERMISSION_OPTIONS.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>

                      <Button
                        className="h-8 w-8"
                        disabled={isPending}
                        onClick={() => handleRemoveShare(share.id)}
                        size="icon"
                        variant="ghost"
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Info Section */}
          <div className="flex items-start gap-2 p-3 rounded-md bg-muted/50 text-sm text-muted-foreground">
            <Info className="h-4 w-4 mt-0.5 shrink-0" />
            <p>Users will only see books in libraries they have access to.</p>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
