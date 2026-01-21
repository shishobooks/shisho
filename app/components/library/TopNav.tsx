import { KeyRound, LogOut, Settings, User, UserCog } from "lucide-react";
import { useCallback } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import GlobalSearch from "@/components/library/GlobalSearch";
import LibraryListPicker from "@/components/library/LibraryListPicker";
import Logo from "@/components/library/Logo";
import { ResyncButton } from "@/components/library/ResyncButton";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useAuth } from "@/hooks/useAuth";

const TopNav = () => {
  const { libraryId } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const { user, logout, hasPermission } = useAuth();

  // Check if user has any admin permissions
  const canAccessAdmin =
    hasPermission("config", "read") ||
    hasPermission("users", "read") ||
    hasPermission("jobs", "read") ||
    hasPermission("libraries", "read");

  const canResync = hasPermission("jobs", "write");

  const handleLogout = useCallback(async () => {
    try {
      await logout();
      toast.success("Signed out successfully");
      navigate("/login");
    } catch {
      toast.error("Failed to sign out");
    }
  }, [logout, navigate]);

  const isBooksActive =
    location.pathname === `/libraries/${libraryId}` ||
    (location.pathname.startsWith(`/libraries/${libraryId}/books`) &&
      !location.pathname.startsWith(`/libraries/${libraryId}/series`) &&
      !location.pathname.startsWith(`/libraries/${libraryId}/people`) &&
      !location.pathname.startsWith(`/libraries/${libraryId}/genres`) &&
      !location.pathname.startsWith(`/libraries/${libraryId}/tags`) &&
      !location.pathname.startsWith(`/libraries/${libraryId}/settings`));
  const isSeriesActive = location.pathname.startsWith(
    `/libraries/${libraryId}/series`,
  );
  const isPeopleActive = location.pathname.startsWith(
    `/libraries/${libraryId}/people`,
  );
  const isGenresActive = location.pathname.startsWith(
    `/libraries/${libraryId}/genres`,
  );
  const isTagsActive = location.pathname.startsWith(
    `/libraries/${libraryId}/tags`,
  );
  const isLibrarySettingsActive = location.pathname.startsWith(
    `/libraries/${libraryId}/settings`,
  );

  return (
    <div className="border-b border-border bg-background dark:bg-neutral-900">
      <div className="max-w-7xl mx-auto px-6">
        <div className="flex items-center justify-between h-16">
          <div className="flex items-center gap-8">
            <Logo asLink />
            {/* Library/List Picker and Resync */}
            <div className="flex items-center gap-1">
              <LibraryListPicker />
              {/* Resync Button */}
              {libraryId && canResync && (
                <ResyncButton libraryId={Number(libraryId)} />
              )}
            </div>
            {/* Navigation buttons for current library */}
            {libraryId && (
              <nav className="flex gap-1">
                <Button
                  className={`h-9 cursor-pointer ${
                    isBooksActive
                      ? "bg-primary text-primary-foreground hover:bg-primary/90 dark:bg-violet-300 dark:text-neutral-900 dark:hover:bg-violet-400"
                      : "hover:text-primary dark:hover:text-violet-300"
                  }`}
                  onClick={() => {
                    // Clear series filter when clicking Books
                    navigate(`/libraries/${libraryId}`, { replace: true });
                  }}
                  variant={isBooksActive ? "default" : "ghost"}
                >
                  Books
                </Button>
                <Button
                  asChild
                  className={`h-9 ${
                    isSeriesActive
                      ? "bg-primary text-primary-foreground hover:bg-primary/90 dark:bg-violet-300 dark:text-neutral-900 dark:hover:bg-violet-400"
                      : "hover:text-primary dark:hover:text-violet-300"
                  }`}
                  variant={isSeriesActive ? "default" : "ghost"}
                >
                  <Link to={`/libraries/${libraryId}/series`}>Series</Link>
                </Button>
                <Button
                  asChild
                  className={`h-9 ${
                    isPeopleActive
                      ? "bg-primary text-primary-foreground hover:bg-primary/90 dark:bg-violet-300 dark:text-neutral-900 dark:hover:bg-violet-400"
                      : "hover:text-primary dark:hover:text-violet-300"
                  }`}
                  variant={isPeopleActive ? "default" : "ghost"}
                >
                  <Link to={`/libraries/${libraryId}/people`}>People</Link>
                </Button>
                <Button
                  asChild
                  className={`h-9 ${
                    isGenresActive
                      ? "bg-primary text-primary-foreground hover:bg-primary/90 dark:bg-violet-300 dark:text-neutral-900 dark:hover:bg-violet-400"
                      : "hover:text-primary dark:hover:text-violet-300"
                  }`}
                  variant={isGenresActive ? "default" : "ghost"}
                >
                  <Link to={`/libraries/${libraryId}/genres`}>Genres</Link>
                </Button>
                <Button
                  asChild
                  className={`h-9 ${
                    isTagsActive
                      ? "bg-primary text-primary-foreground hover:bg-primary/90 dark:bg-violet-300 dark:text-neutral-900 dark:hover:bg-violet-400"
                      : "hover:text-primary dark:hover:text-violet-300"
                  }`}
                  variant={isTagsActive ? "default" : "ghost"}
                >
                  <Link to={`/libraries/${libraryId}/tags`}>Tags</Link>
                </Button>
                {hasPermission("libraries", "write") && (
                  <Button
                    asChild
                    className={`h-9 ${
                      isLibrarySettingsActive
                        ? "bg-primary text-primary-foreground hover:bg-primary/90 dark:bg-violet-300 dark:text-neutral-900 dark:hover:bg-violet-400"
                        : "hover:text-primary dark:hover:text-violet-300"
                    }`}
                    variant={isLibrarySettingsActive ? "default" : "ghost"}
                  >
                    <Link to={`/libraries/${libraryId}/settings`}>
                      Settings
                    </Link>
                  </Button>
                )}
              </nav>
            )}
          </div>
          <div className="flex items-center gap-4">
            <GlobalSearch />
            {canAccessAdmin && (
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      asChild
                      className="h-9 w-9"
                      size="icon"
                      variant="ghost"
                    >
                      <Link to="/settings">
                        <Settings className="h-4 w-4" />
                      </Link>
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>Global Settings</p>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button className="h-9 w-9" size="icon" variant="ghost">
                  <User className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuLabel>
                  <div className="flex flex-col">
                    <span>{user?.username}</span>
                    <span className="text-xs font-normal text-muted-foreground">
                      {user?.role_name}
                    </span>
                  </div>
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild>
                  <Link to="/user/settings">
                    <UserCog className="h-4 w-4" />
                    User Settings
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link to="/user/security">
                    <KeyRound className="h-4 w-4" />
                    Security
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuItem onClick={handleLogout}>
                  <LogOut className="h-4 w-4" />
                  Sign out
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </div>
    </div>
  );
};

export default TopNav;
