import { Check, KeyRound, Moon, Sun } from "lucide-react";
import { Link } from "react-router-dom";

import { useTheme, type Theme } from "@/components/contexts/Theme/context";
import { SizeSelector } from "@/components/library/SizeSelector";
import TopNav from "@/components/library/TopNav";
import { Button } from "@/components/ui/button";
import { DEFAULT_GALLERY_SIZE } from "@/constants/gallerySize";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { usePageTitle } from "@/hooks/usePageTitle";
import type { GallerySize } from "@/types";

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
  usePageTitle("User Settings");

  const { theme, setTheme } = useTheme();
  const userSettingsQuery = useUserSettings();
  const updateUserSettings = useUpdateUserSettings();
  const gallerySize: GallerySize =
    userSettingsQuery.data?.gallery_size ?? DEFAULT_GALLERY_SIZE;

  const handleGallerySizeChange = (next: GallerySize) => {
    updateUserSettings.mutate({ gallery_size: next });
  };

  return (
    <div>
      <TopNav />
      <div className="max-w-7xl w-full mx-auto px-6 py-8">
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
              <div className="mt-6 flex flex-col gap-2">
                <label className="text-sm font-medium">
                  Gallery cover size
                </label>
                <SizeSelector
                  onChange={handleGallerySizeChange}
                  value={gallerySize}
                />
                <p className="text-xs text-muted-foreground">
                  Applies to every gallery page. Used as your default; changes
                  made inline on a gallery page override this until you save
                  them.
                </p>
              </div>
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
