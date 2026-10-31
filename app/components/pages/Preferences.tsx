import { Check, Moon, Sun } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useLocation } from "react-router-dom";

import { useTheme, type Theme } from "@/components/contexts/Theme/context";
import DeliveryRulesEditor from "@/components/devices/DeliveryRulesEditor";
import QueryError from "@/components/library/QueryError";
import { SizeSelector } from "@/components/library/SizeSelector";
import TopNav from "@/components/library/TopNav";
import {
  ForcedPasswordReset,
  PASSWORD_SECTION_ID,
  PasswordSection,
} from "@/components/preferences/PasswordSection";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { UnsavedChangesDialog } from "@/components/ui/unsaved-changes-dialog";
import { DEFAULT_GALLERY_SIZE } from "@/constants/gallerySize";
import { useDeliveryRules } from "@/hooks/queries/delivery";
import { useNavLibraries } from "@/hooks/queries/libraries";
import {
  useUpdateUserSettings,
  useUserSettings,
} from "@/hooks/queries/settings";
import { useAuth } from "@/hooks/useAuth";
import { usePageTitle } from "@/hooks/usePageTitle";
import { useUnsavedChanges } from "@/hooks/useUnsavedChanges";
import { toastRequestError } from "@/libraries/api";
import { cn } from "@/libraries/utils";
import type { GallerySize } from "@/types";
import { DELIVERY_RULES_SECTION_ID } from "@/utils/delivery";
import { timeZoneOptions } from "@/utils/timeZone";

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
  <Button
    aria-pressed={currentTheme === theme}
    className={cn(
      "flex items-center gap-3 px-4 py-3 rounded-md border transition-colors w-full",
      currentTheme === theme
        ? "border-primary bg-primary/5 text-primary"
        : "border-border hover:bg-muted",
    )}
    onClick={() => onSelect(theme)}
    variant="unstyled"
  >
    {icon}
    <span className="flex-1 text-left font-medium">{label}</span>
    {currentTheme === theme && <Check className="h-4 w-4" />}
  </Button>
);

// The Select value standing for "no default library", since Radix Select
// reserves the empty string.
const FIRST_LIBRARY = "first";

const Preferences = () => {
  const { demoMode, user } = useAuth();
  // A temporary password has to be replaced before anything else, so the
  // reset screen stands in for the whole page (ProtectedRoute sends every
  // page here until then).
  if (!demoMode && user?.must_change_password) {
    return <ForcedPasswordReset />;
  }
  return <PreferencesPage />;
};

const PreferencesPage = () => {
  usePageTitle("Preferences");

  const { theme, setTheme } = useTheme();
  const { demoMode } = useAuth();
  const userSettingsQuery = useUserSettings();
  const updateUserSettings = useUpdateUserSettings();
  const gallerySize: GallerySize =
    userSettingsQuery.data?.gallery_size ?? DEFAULT_GALLERY_SIZE;

  const librariesQuery = useNavLibraries();
  const libraries = librariesQuery.data ?? [];
  const librariesFailed = Boolean(librariesQuery.error) && !librariesQuery.data;
  const defaultLibraryId = userSettingsQuery.data?.default_library_id;
  const defaultLibraryValue = libraries.some((l) => l.id === defaultLibraryId)
    ? String(defaultLibraryId)
    : FIRST_LIBRARY;

  const handleDefaultLibraryChange = (value: string) => {
    updateUserSettings.mutate(
      {
        default_library_id: value === FIRST_LIBRARY ? null : Number(value),
      },
      {
        onError: (error) =>
          toastRequestError(error, "Failed to update default library"),
      },
    );
  };

  // Every other setting here saves as it changes; the Delivery Rules editor
  // and the password form have their own Save.
  const [deliveryRulesHaveChanges, setDeliveryRulesHaveChanges] =
    useState(false);
  const [passwordHasChanges, setPasswordHasChanges] = useState(false);
  const { showBlockerDialog, proceedNavigation, cancelNavigation } =
    useUnsavedChanges(deliveryRulesHaveChanges || passwordHasChanges);

  // A Device page links straight to the Delivery Rules section, and the old
  // Security URL to the Password section. Scroll once the page's queries have
  // loaded, since what they render above a section changes where it sits.
  const { hash } = useLocation();
  const deliveryRulesLoaded = useDeliveryRules().data !== undefined;
  const settingsLoaded =
    userSettingsQuery.data !== undefined && librariesQuery.data !== undefined;
  useEffect(() => {
    const sectionId = hash.slice(1);
    if (
      (sectionId === DELIVERY_RULES_SECTION_ID ||
        sectionId === PASSWORD_SECTION_ID) &&
      deliveryRulesLoaded &&
      settingsLoaded
    ) {
      document.getElementById(sectionId)?.scrollIntoView({ block: "start" });
    }
  }, [hash, deliveryRulesLoaded, settingsLoaded]);

  const timeZone = userSettingsQuery.data?.time_zone ?? undefined;
  const timeZones = useMemo(() => timeZoneOptions(timeZone), [timeZone]);
  const handleTimeZoneChange = (zone: string) => {
    updateUserSettings.mutate(
      { time_zone: zone },
      {
        onError: (error) =>
          toastRequestError(error, "Failed to update time zone"),
      },
    );
  };

  const handleGallerySizeChange = (next: GallerySize) => {
    updateUserSettings.mutate(
      { gallery_size: next },
      {
        onError: (error) =>
          toastRequestError(error, "Failed to update gallery size"),
      },
    );
  };

  return (
    <div>
      <TopNav />
      <div className="w-full px-6 py-8">
        <div className="mb-6 md:mb-8">
          <h1 className="text-2xl font-semibold mb-2">Preferences</h1>
          <p className="text-muted-foreground">
            Manage how Shisho looks and works for you
          </p>
        </div>

        <div className="max-w-2xl space-y-6">
          {/* Theme Settings */}
          <div className="border border-border rounded-md p-4 md:p-6">
            <h2 className="text-lg font-semibold mb-4">Appearance</h2>
            <div aria-label="Theme" className="space-y-3" role="group">
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

          {/* Library Settings: only a choice with two or more libraries */}
          {librariesFailed && (
            <div className="border border-border rounded-md p-4 md:p-6">
              <h2 className="text-lg font-semibold mb-4">Library</h2>
              <QueryError
                fallback="Failed to load libraries"
                query={librariesQuery}
              />
            </div>
          )}
          {libraries.length > 1 && (
            <div className="border border-border rounded-md p-4 md:p-6">
              <h2 className="text-lg font-semibold mb-4">Library</h2>
              <div className="flex flex-col gap-2">
                <label
                  className="text-sm font-medium"
                  htmlFor="default-library-setting"
                >
                  Default library
                </label>
                <Select
                  disabled={!userSettingsQuery.data}
                  onValueChange={handleDefaultLibraryChange}
                  value={defaultLibraryValue}
                >
                  <SelectTrigger
                    className="w-full sm:w-72"
                    id="default-library-setting"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={FIRST_LIBRARY}>First library</SelectItem>
                    {libraries.map((library) => (
                      <SelectItem key={library.id} value={String(library.id)}>
                        {library.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  The library that opens when you go to the home page.
                </p>
              </div>
            </div>
          )}

          {/* Reader Settings */}
          <div className="border border-border rounded-md p-4 md:p-6">
            <h2 className="text-lg font-semibold mb-4">Reader</h2>
            <div className="flex items-center justify-between">
              <div>
                <label
                  className="text-sm font-medium"
                  htmlFor="hide-chrome-setting"
                >
                  Auto-hide controls
                </label>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Hide the header and footer in the reader. Move your mouse or
                  tap the screen to reveal them.
                </p>
              </div>
              <Switch
                checked={userSettingsQuery.data?.viewer_hide_chrome ?? false}
                disabled={!userSettingsQuery.data}
                id="hide-chrome-setting"
                onCheckedChange={(checked) =>
                  updateUserSettings.mutate(
                    { viewer_hide_chrome: checked },
                    {
                      onError: (error) =>
                        toastRequestError(
                          error,
                          "Failed to save reader settings",
                        ),
                    },
                  )
                }
              />
            </div>
          </div>

          {/* Reading Settings */}
          <div className="border border-border rounded-md p-4 md:p-6">
            <h2 className="text-lg font-semibold mb-4">Reading</h2>
            <div className="flex flex-col gap-2">
              <label
                className="text-sm font-medium"
                htmlFor="time-zone-setting"
              >
                Time zone
              </label>
              <Select
                disabled={!userSettingsQuery.data}
                onValueChange={handleTimeZoneChange}
                value={timeZone}
              >
                <SelectTrigger
                  className="w-full sm:w-72"
                  id="time-zone-setting"
                >
                  <SelectValue placeholder="Not set" />
                </SelectTrigger>
                <SelectContent>
                  {timeZones.map((zone) => (
                    <SelectItem key={zone} value={zone}>
                      {zone}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                Decides the day reading starts and finishes, from the status
                menu or as you read. Changing it doesn&apos;t change dates
                already recorded.
              </p>
            </div>
          </div>

          {/* Default Delivery Rules */}
          <div
            className="scroll-mt-20 border border-border rounded-md p-4 md:p-6"
            id={DELIVERY_RULES_SECTION_ID}
          >
            <h2 className="text-lg font-semibold mb-1">Delivery Rules</h2>
            <p className="text-sm text-muted-foreground mb-4">
              Choose which File Type each book&apos;s main download button and
              OPDS deliver. For each book, the first rule matching one of its
              files wins, separately for ebooks and audiobooks. OPDS leaves out
              File Types with no rule; the download menu always offers every
              File Type.
            </p>
            <DeliveryRulesEditor
              onHasChangesChange={setDeliveryRulesHaveChanges}
            />
          </div>

          {!demoMode && (
            <PasswordSection onHasChangesChange={setPasswordHasChanges} />
          )}
        </div>
      </div>
      <UnsavedChangesDialog
        onDiscard={proceedNavigation}
        onStay={cancelNavigation}
        open={showBlockerDialog}
      />
    </div>
  );
};

export default Preferences;
