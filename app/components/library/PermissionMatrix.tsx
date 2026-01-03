import { Checkbox } from "@/components/ui/checkbox";
import type { PermissionInput } from "@/types";
import {
  OperationRead,
  OperationWrite,
  ResourceBooks,
  ResourceConfig,
  ResourceJobs,
  ResourceLibraries,
  ResourcePeople,
  ResourceSeries,
  ResourceUsers,
} from "@/types";

// Define available resources with display names
const RESOURCES = [
  { key: ResourceLibraries, label: "Libraries" },
  { key: ResourceBooks, label: "Books" },
  { key: ResourcePeople, label: "People" },
  { key: ResourceSeries, label: "Series" },
  { key: ResourceUsers, label: "Users" },
  { key: ResourceJobs, label: "Jobs" },
  { key: ResourceConfig, label: "Config" },
] as const;

// Define available operations with display names
const OPERATIONS = [
  { key: OperationRead, label: "Read" },
  { key: OperationWrite, label: "Write" },
] as const;

interface PermissionMatrixProps {
  permissions: PermissionInput[];
  onChange: (permissions: PermissionInput[]) => void;
  disabled?: boolean;
}

const PermissionMatrix = ({
  permissions,
  onChange,
  disabled = false,
}: PermissionMatrixProps) => {
  // Check if a permission exists
  const hasPermission = (resource: string, operation: string): boolean => {
    return permissions.some(
      (p) => p.resource === resource && p.operation === operation,
    );
  };

  // Toggle a permission
  const togglePermission = (resource: string, operation: string) => {
    if (disabled) return;

    const exists = hasPermission(resource, operation);
    if (exists) {
      // Remove the permission
      onChange(
        permissions.filter(
          (p) => !(p.resource === resource && p.operation === operation),
        ),
      );
    } else {
      // Add the permission
      onChange([...permissions, { resource, operation }]);
    }
  };

  // Toggle all permissions for a resource
  const toggleResource = (resource: string) => {
    if (disabled) return;

    const allChecked = OPERATIONS.every((op) =>
      hasPermission(resource, op.key),
    );
    if (allChecked) {
      // Remove all permissions for this resource
      onChange(permissions.filter((p) => p.resource !== resource));
    } else {
      // Add all permissions for this resource
      const newPermissions = permissions.filter((p) => p.resource !== resource);
      OPERATIONS.forEach((op) => {
        newPermissions.push({ resource, operation: op.key });
      });
      onChange(newPermissions);
    }
  };

  // Toggle all permissions for an operation
  const toggleOperation = (operation: string) => {
    if (disabled) return;

    const allChecked = RESOURCES.every((res) =>
      hasPermission(res.key, operation),
    );
    if (allChecked) {
      // Remove all permissions for this operation
      onChange(permissions.filter((p) => p.operation !== operation));
    } else {
      // Add all permissions for this operation
      const newPermissions = permissions.filter(
        (p) => p.operation !== operation,
      );
      RESOURCES.forEach((res) => {
        newPermissions.push({ resource: res.key, operation });
      });
      onChange(newPermissions);
    }
  };

  // Check if all permissions for a resource are checked
  const isResourceFullyChecked = (resource: string): boolean => {
    return OPERATIONS.every((op) => hasPermission(resource, op.key));
  };

  // Check if some (but not all) permissions for a resource are checked
  const isResourcePartiallyChecked = (resource: string): boolean => {
    const count = OPERATIONS.filter((op) =>
      hasPermission(resource, op.key),
    ).length;
    return count > 0 && count < OPERATIONS.length;
  };

  // Check if all permissions for an operation are checked
  const isOperationFullyChecked = (operation: string): boolean => {
    return RESOURCES.every((res) => hasPermission(res.key, operation));
  };

  // Check if some (but not all) permissions for an operation are checked
  const isOperationPartiallyChecked = (operation: string): boolean => {
    const count = RESOURCES.filter((res) =>
      hasPermission(res.key, operation),
    ).length;
    return count > 0 && count < RESOURCES.length;
  };

  return (
    <div className="border border-border rounded-md overflow-hidden">
      <table className="w-full text-sm">
        <thead>
          <tr className="bg-muted/50">
            <th className="text-left font-medium py-3 px-4 border-b border-border">
              Resource
            </th>
            {OPERATIONS.map((op) => (
              <th
                className="text-center font-medium py-3 px-4 border-b border-border w-24"
                key={op.key}
              >
                <div className="flex flex-col items-center gap-1">
                  <span>{op.label}</span>
                  <Checkbox
                    checked={isOperationFullyChecked(op.key)}
                    className={
                      isOperationPartiallyChecked(op.key)
                        ? "data-[state=unchecked]:bg-primary/30"
                        : ""
                    }
                    disabled={disabled}
                    onCheckedChange={() => toggleOperation(op.key)}
                  />
                </div>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {RESOURCES.map((resource, index) => (
            <tr
              className={index % 2 === 0 ? "bg-background" : "bg-muted/20"}
              key={resource.key}
            >
              <td className="py-3 px-4 border-b border-border">
                <div className="flex items-center gap-3">
                  <Checkbox
                    checked={isResourceFullyChecked(resource.key)}
                    className={
                      isResourcePartiallyChecked(resource.key)
                        ? "data-[state=unchecked]:bg-primary/30"
                        : ""
                    }
                    disabled={disabled}
                    onCheckedChange={() => toggleResource(resource.key)}
                  />
                  <span>{resource.label}</span>
                </div>
              </td>
              {OPERATIONS.map((op) => (
                <td
                  className="text-center py-3 px-4 border-b border-border"
                  key={op.key}
                >
                  <Checkbox
                    checked={hasPermission(resource.key, op.key)}
                    disabled={disabled}
                    onCheckedChange={() =>
                      togglePermission(resource.key, op.key)
                    }
                  />
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default PermissionMatrix;
