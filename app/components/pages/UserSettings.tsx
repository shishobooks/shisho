import { ArrowLeft, Check, KeyRound, Moon, Sun } from "lucide-react";
import { Link } from "react-router-dom";

import { useTheme, type Theme } from "@/components/contexts/Theme/context";
import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";

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
    className={`flex items-center gap-3 px-4 py-3 rounded-md border transition-colors w-full cursor-pointer ${
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
  const { theme, setTheme } = useTheme();

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

          {/* Security Settings Link */}
          <div className="rounded-md border border-border p-6">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-lg font-semibold">Security</h2>
                <p className="text-sm text-muted-foreground">
                  Manage your password and API keys
                </p>
              </div>
              <Button asChild variant="outline">
                <Link to="/user/security">
                  <KeyRound className="mr-2 h-4 w-4" />
                  Security Settings
                </Link>
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default UserSettings;
