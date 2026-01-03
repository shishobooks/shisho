import { ArrowLeft, Loader2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { useLibraries } from "@/hooks/queries/libraries";
import { useCreateUser, useRoles } from "@/hooks/queries/users";

const CreateUser = () => {
  const navigate = useNavigate();
  const createUserMutation = useCreateUser();
  const { data: rolesData } = useRoles();
  const { data: librariesData } = useLibraries();

  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [roleId, setRoleId] = useState<number | null>(null);
  const [allLibraryAccess, setAllLibraryAccess] = useState(true);
  const [selectedLibraries, setSelectedLibraries] = useState<number[]>([]);

  const handleLibraryToggle = (libraryId: number) => {
    setSelectedLibraries((prev) =>
      prev.includes(libraryId)
        ? prev.filter((id) => id !== libraryId)
        : [...prev, libraryId],
    );
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

    if (!roleId) {
      toast.error("Please select a role");
      return;
    }

    try {
      const user = await createUserMutation.mutateAsync({
        username,
        email: email.trim() || undefined,
        password,
        role_id: roleId,
        all_library_access: allLibraryAccess,
        library_ids: allLibraryAccess ? undefined : selectedLibraries,
      });

      toast.success("User created successfully!");
      navigate(`/settings/users/${user.id}`);
    } catch (error) {
      let msg = "Failed to create user";
      if (error instanceof Error) {
        msg = error.message;
      }
      toast.error(msg);
    }
  };

  const roles = rolesData?.roles ?? [];
  const libraries = librariesData?.libraries ?? [];

  return (
    <div>
      <div className="mb-6">
        <Button asChild variant="ghost">
          <Link to="/settings/users">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Users
          </Link>
        </Button>
      </div>

      <div className="mb-8">
        <h1 className="text-2xl font-semibold mb-2">Create User</h1>
        <p className="text-muted-foreground">Add a new user to the system</p>
      </div>

      <div className="max-w-2xl space-y-6 border border-border rounded-md p-6">
        {/* Basic Info */}
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
            <p className="text-xs text-muted-foreground">
              At least 3 characters
            </p>
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
            <p className="text-xs text-muted-foreground">
              At least 8 characters
            </p>
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
        </div>

        <Separator />

        {/* Role */}
        <div className="space-y-4">
          <h2 className="text-lg font-medium">Role</h2>
          <div className="space-y-2">
            <Label>Select Role</Label>
            <div className="grid gap-2">
              {roles.map((role) => (
                <div className="flex items-center space-x-2" key={role.id}>
                  <Checkbox
                    checked={roleId === role.id}
                    id={`role-${role.id}`}
                    onCheckedChange={(checked) => {
                      if (checked) setRoleId(role.id);
                    }}
                  />
                  <Label
                    className="text-sm font-normal cursor-pointer"
                    htmlFor={`role-${role.id}`}
                  >
                    {role.name}
                    {role.is_system && (
                      <span className="text-muted-foreground ml-1">
                        (system)
                      </span>
                    )}
                  </Label>
                </div>
              ))}
            </div>
          </div>
        </div>

        <Separator />

        {/* Library Access */}
        <div className="space-y-4">
          <h2 className="text-lg font-medium">Library Access</h2>

          <div className="flex items-center space-x-2">
            <Checkbox
              checked={allLibraryAccess}
              id="all-library-access"
              onCheckedChange={(checked) =>
                setAllLibraryAccess(checked as boolean)
              }
            />
            <Label
              className="text-sm font-normal cursor-pointer"
              htmlFor="all-library-access"
            >
              Access to all libraries
            </Label>
          </div>

          {!allLibraryAccess && (
            <div className="space-y-2 pl-6">
              <Label>Select Libraries</Label>
              {libraries.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No libraries available. Create a library first.
                </p>
              ) : (
                <div className="grid gap-2">
                  {libraries.map((library) => (
                    <div
                      className="flex items-center space-x-2"
                      key={library.id}
                    >
                      <Checkbox
                        checked={selectedLibraries.includes(library.id)}
                        id={`library-${library.id}`}
                        onCheckedChange={() => handleLibraryToggle(library.id)}
                      />
                      <Label
                        className="text-sm font-normal cursor-pointer"
                        htmlFor={`library-${library.id}`}
                      >
                        {library.name}
                      </Label>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        <Separator />

        {/* Create Button */}
        <div className="flex justify-end pt-4">
          <Button
            disabled={createUserMutation.isPending}
            onClick={handleCreate}
          >
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
    </div>
  );
};

export default CreateUser;
