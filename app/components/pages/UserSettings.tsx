import { ArrowLeft, Check, Moon, Sun } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";

import { useTheme, type Theme } from "@/components/contexts/Theme/context";
import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { useResetPassword } from "@/hooks/queries/users";
import { useAuth } from "@/hooks/useAuth";

interface ThemeOptionProps {
  theme: Theme;
  currentTheme: Theme;
  label: string;
  icon: React.ReactNode;
  onSelect: (theme: Theme) => void;
}

const ThemeOption = ({
  theme,
  currentTheme,
  label,
  icon,
  onSelect,
}: ThemeOptionProps) => (
  <button
    className={`flex items-center gap-3 px-4 py-3 rounded-md border transition-colors w-full ${
      currentTheme === theme
        ? "border-primary bg-primary/5 text-primary"
        : "border-border hover:bg-muted"
    }`}
    onClick={() => onSelect(theme)}
    type="button"
  >
    {icon}
    <span className="flex-1 text-left font-medium">{label}</span>
    {currentTheme === theme && <Check className="h-4 w-4" />}
  </button>
);

const UserSettings = () => {
  const { user } = useAuth();
  const { theme, setTheme } = useTheme();
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
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
        <div className="mb-6">
          <Button asChild variant="ghost">
            <Link to="/">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back
            </Link>
          </Button>
        </div>

        <div className="mb-8">
          <h1 className="text-2xl font-semibold mb-2">User Settings</h1>
          <p className="text-muted-foreground">
            Manage your account preferences
          </p>
        </div>

        <div className="max-w-2xl space-y-6">
          {/* Theme Settings */}
          <div className="border border-border rounded-md p-6">
            <h2 className="text-lg font-semibold mb-4">Appearance</h2>
            <div className="space-y-3">
              <ThemeOption
                currentTheme={theme}
                icon={<Sun className="h-5 w-5" />}
                label="Light"
                onSelect={setTheme}
                theme="light"
              />
              <ThemeOption
                currentTheme={theme}
                icon={<Moon className="h-5 w-5" />}
                label="Dark"
                onSelect={setTheme}
                theme="dark"
              />
              <ThemeOption
                currentTheme={theme}
                icon={
                  <div className="h-5 w-5 flex items-center justify-center">
                    <Sun className="h-3 w-3 absolute" />
                    <Moon className="h-3 w-3 absolute translate-x-1 translate-y-1" />
                  </div>
                }
                label="System"
                onSelect={setTheme}
                theme="system"
              />
            </div>
          </div>

          <Separator />

          {/* Password Change */}
          <div className="border border-border rounded-md p-6">
            <h2 className="text-lg font-semibold mb-4">Change Password</h2>
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
        </div>
      </div>
    </div>
  );
};

export default UserSettings;
