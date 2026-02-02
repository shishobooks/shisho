import equal from "fast-deep-equal";
import { Loader2, Trash2 } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import PermissionMatrix from "@/components/library/PermissionMatrix";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FormDialog } from "@/components/ui/form-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  useCreateRole,
  useDeleteRole,
  useUpdateRole,
} from "@/hooks/queries/users";
import { useFormDialogClose } from "@/hooks/useFormDialogClose";
import type { PermissionInput, Role } from "@/types";

interface RoleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  role?: Role | null; // null for create mode, Role for edit mode
}

const RoleDialog = ({ open, onOpenChange, role }: RoleDialogProps) => {
  const isEditMode = Boolean(role);
  const isSystem = role?.is_system ?? false;

  const [name, setName] = useState("");
  const [permissions, setPermissions] = useState<PermissionInput[]>([]);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);

  const createRoleMutation = useCreateRole();
  const updateRoleMutation = useUpdateRole();
  const deleteRoleMutation = useDeleteRole();

  // Store initial values for change detection
  const [initialValues, setInitialValues] = useState<{
    name: string;
    permissions: PermissionInput[];
  } | null>(null);

  // Track previous open state to detect open transitions.
  // Start with false so that if dialog starts open, we detect it as "just opened".
  const prevOpenRef = useRef(false);

  // Initialize form only when dialog opens (closed->open transition)
  // This preserves user edits when props change while dialog is open
  useEffect(() => {
    const justOpened = open && !prevOpenRef.current;
    prevOpenRef.current = open;

    // Only initialize when dialog just opened, not on every prop change
    if (!justOpened) return;

    if (role) {
      const initialName = role.name;
      const initialPermissions =
        role.permissions?.map((p) => ({
          resource: p.resource,
          operation: p.operation,
        })) ?? [];

      setName(initialName);
      setPermissions(initialPermissions);

      // Store initial values for comparison
      setInitialValues({
        name: initialName,
        permissions: initialPermissions,
      });
    } else {
      setName("");
      setPermissions([]);
      setInitialValues({
        name: "",
        permissions: [],
      });
    }
  }, [role, open]);

  // Compute hasChanges by comparing current values to initial values
  // This works for both create mode (initialValues = { name: "", permissions: [] })
  // and edit mode (initialValues = role data)
  const hasChanges = useMemo(() => {
    if (!initialValues) return false;
    return (
      name !== initialValues.name ||
      !equal(permissions, initialValues.permissions)
    );
  }, [name, permissions, initialValues]);

  const { requestClose } = useFormDialogClose(open, onOpenChange, hasChanges);

  const handleSave = async () => {
    if (!name.trim()) {
      toast.error("Role name is required");
      return;
    }

    try {
      if (isEditMode && role) {
        await updateRoleMutation.mutateAsync({
          id: role.id,
          payload: {
            name: name !== role.name ? name : undefined,
            permissions,
          },
        });
        toast.success("Role updated successfully");
      } else {
        await createRoleMutation.mutateAsync({
          name,
          permissions,
        });
        toast.success("Role created successfully");
      }
      // Reset initial values so hasChanges becomes false, then close via effect
      setInitialValues({
        name,
        permissions: [...permissions],
      });
      requestClose();
    } catch (error) {
      let msg = isEditMode ? "Failed to update role" : "Failed to create role";
      if (error instanceof Error) {
        msg = error.message;
      }
      toast.error(msg);
    }
  };

  const handleDelete = async () => {
    if (!role) return;

    try {
      await deleteRoleMutation.mutateAsync(role.id);
      toast.success("Role deleted successfully");
      setDeleteConfirmOpen(false);
      // Reset form state and initial values so hasChanges becomes false, then close
      setName("");
      setPermissions([]);
      setInitialValues({ name: "", permissions: [] });
      requestClose();
    } catch (error) {
      let msg = "Failed to delete role";
      if (error instanceof Error) {
        msg = error.message;
      }
      toast.error(msg);
    }
  };

  const isPending =
    createRoleMutation.isPending ||
    updateRoleMutation.isPending ||
    deleteRoleMutation.isPending;

  return (
    <>
      <FormDialog
        hasChanges={hasChanges}
        onOpenChange={onOpenChange}
        open={open}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {isEditMode ? "Edit Role" : "Create Role"}
              {isSystem && <Badge variant="secondary">System</Badge>}
            </DialogTitle>
            <DialogDescription>
              {isSystem
                ? "System roles cannot be renamed or deleted, but you can modify their permissions."
                : isEditMode
                  ? "Update the role name and permissions."
                  : "Create a new role with custom permissions."}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-6 py-4">
            {/* Role Name */}
            <div className="space-y-2">
              <Label htmlFor="role-name">Name</Label>
              <Input
                disabled={isSystem}
                id="role-name"
                onChange={(e) => setName(e.target.value)}
                placeholder="Enter role name"
                value={name}
              />
            </div>

            {/* Permissions Matrix */}
            <div className="space-y-2">
              <Label>Permissions</Label>
              <p className="text-xs text-muted-foreground mb-2">
                Select which resources this role can read or write. Use the row
                and column checkboxes to toggle all permissions for a resource
                or operation.
              </p>
              <PermissionMatrix
                onChange={setPermissions}
                permissions={permissions}
              />
            </div>
          </div>

          <DialogFooter className="flex-col sm:flex-row gap-2">
            {isEditMode && !isSystem && (
              <Button
                className="mr-auto"
                disabled={isPending}
                onClick={() => setDeleteConfirmOpen(true)}
                variant="destructive"
              >
                <Trash2 className="mr-2 h-4 w-4" />
                Delete Role
              </Button>
            )}
            <Button
              disabled={isPending}
              onClick={() => onOpenChange(false)}
              variant="outline"
            >
              Cancel
            </Button>
            <Button disabled={isPending} onClick={handleSave}>
              {isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  {isEditMode ? "Saving..." : "Creating..."}
                </>
              ) : isEditMode ? (
                "Save Changes"
              ) : (
                "Create Role"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </FormDialog>

      <ConfirmDialog
        confirmLabel="Delete"
        description={`Are you sure you want to delete the role "${role?.name}"? This action cannot be undone. Users assigned to this role will need to be reassigned.`}
        isPending={deleteRoleMutation.isPending}
        onConfirm={handleDelete}
        onOpenChange={setDeleteConfirmOpen}
        open={deleteConfirmOpen}
        title="Delete Role"
      />
    </>
  );
};

export default RoleDialog;
