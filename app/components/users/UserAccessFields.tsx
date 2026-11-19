import { AlertTriangle } from "lucide-react";

import QueryError from "@/components/library/QueryError";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Separator } from "@/components/ui/separator";
import { useLibraries } from "@/hooks/queries/libraries";
import { useRoles } from "@/hooks/queries/users";
import { sortRoles } from "@/utils/roles";

export interface UserAccess {
  roleId: number | null;
  allLibraryAccess: boolean;
  selectedLibraries: number[];
}

/** True when the access grants no Library, so the new user sees no books. */
const grantsNoLibrary = (access: UserAccess) =>
  !access.allLibraryAccess && access.selectedLibraries.length === 0;

interface UserAccessFieldsProps {
  /** Prefixes element ids, so two forms on one page don't collide. */
  idPrefix: string;
  value: UserAccess;
  onChange: (value: UserAccess) => void;
}

/**
 * The role and Library access a new user gets, shared by both Add User
 * paths. It warns when no Library is chosen, since that user would sign in
 * to an empty server.
 */
const UserAccessFields = ({
  idPrefix,
  value,
  onChange,
}: UserAccessFieldsProps) => {
  const { data: rolesData } = useRoles();
  const librariesQuery = useLibraries();
  const librariesData = librariesQuery.data;
  const roles = sortRoles(rolesData?.items ?? []);
  const libraries = librariesData?.items ?? [];

  const toggleLibrary = (libraryId: number) =>
    onChange({
      ...value,
      selectedLibraries: value.selectedLibraries.includes(libraryId)
        ? value.selectedLibraries.filter((id) => id !== libraryId)
        : [...value.selectedLibraries, libraryId],
    });

  return (
    <>
      <div className="space-y-4">
        <h2 className="text-lg font-medium">Role</h2>
        <div className="space-y-2">
          <Label id={`${idPrefix}-select-role-label`}>Select Role</Label>
          <RadioGroup
            aria-labelledby={`${idPrefix}-select-role-label`}
            name={`${idPrefix}-role`}
            onValueChange={(role) =>
              onChange({ ...value, roleId: Number(role) })
            }
            value={value.roleId === null ? "" : String(value.roleId)}
          >
            {roles.map((role) => (
              <div className="flex items-center space-x-2" key={role.id}>
                <RadioGroupItem
                  id={`${idPrefix}-role-${role.id}`}
                  value={String(role.id)}
                />
                <Label
                  className="text-sm font-normal cursor-pointer"
                  htmlFor={`${idPrefix}-role-${role.id}`}
                >
                  {role.name}
                  {role.is_system && (
                    <span className="text-muted-foreground ml-1">(system)</span>
                  )}
                </Label>
              </div>
            ))}
          </RadioGroup>
        </div>
      </div>

      <Separator />

      <div className="space-y-4">
        <h2 className="text-lg font-medium">Library Access</h2>

        <div className="flex items-center space-x-2">
          <Checkbox
            checked={value.allLibraryAccess}
            id={`${idPrefix}-all-library-access`}
            onCheckedChange={(checked) =>
              onChange({ ...value, allLibraryAccess: checked === true })
            }
          />
          <Label
            className="text-sm font-normal cursor-pointer"
            htmlFor={`${idPrefix}-all-library-access`}
          >
            Access to all libraries
          </Label>
        </div>

        {!value.allLibraryAccess && (
          <div className="space-y-2 pl-6">
            <Label id={`${idPrefix}-select-libraries-label`}>
              Select Libraries
            </Label>
            {librariesQuery.error && !librariesData ? (
              <QueryError
                fallback="Failed to load libraries"
                query={librariesQuery}
              />
            ) : libraries.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No libraries available. Create a library first.
              </p>
            ) : (
              <div
                aria-labelledby={`${idPrefix}-select-libraries-label`}
                className="grid gap-2"
                role="group"
              >
                {libraries.map((library) => (
                  <div className="flex items-center space-x-2" key={library.id}>
                    <Checkbox
                      checked={value.selectedLibraries.includes(library.id)}
                      id={`${idPrefix}-library-${library.id}`}
                      onCheckedChange={() => toggleLibrary(library.id)}
                    />
                    <Label
                      className="text-sm font-normal cursor-pointer"
                      htmlFor={`${idPrefix}-library-${library.id}`}
                    >
                      {library.name}
                    </Label>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {grantsNoLibrary(value) && (
          <p
            className="flex gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-400"
            role="status"
          >
            <AlertTriangle
              aria-hidden="true"
              className="mt-px h-3.5 w-3.5 shrink-0"
            />
            <span>
              No libraries are selected, so they won&apos;t see any books until
              you give them access to one.
            </span>
          </p>
        )}
      </div>
    </>
  );
};

export default UserAccessFields;
