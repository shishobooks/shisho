import {
  BookPlus,
  Check,
  ChevronDown,
  Library,
  LogOut,
  Plus,
  Settings,
  User,
  UserCog,
} from "lucide-react";
import { useCallback } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";

import GlobalSearch from "@/components/library/GlobalSearch";
import Logo from "@/components/library/Logo";
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
import { useCreateLibrary, useLibraries } from "@/hooks/queries/libraries";
import { useAuth } from "@/hooks/useAuth";

const TopNav = () => {
  const { libraryId } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const { user, logout, hasPermission } = useAuth();
  const createLibraryMutation = useCreateLibrary();
  const isDevelopment = import.meta.env.DEV;

  // Check if user has any admin permissions
  const canAccessAdmin =
    hasPermission("config", "read") ||
    hasPermission("users", "read") ||
    hasPermission("jobs", "read") ||
    hasPermission("libraries", "read");

  const canCreateLibrary = hasPermission("libraries", "write");

  // Load all libraries for the switcher
  const librariesQuery = useLibraries({});
  const libraries = librariesQuery.data?.libraries || [];
  const currentLibrary = libraries.find((lib) => lib.id === Number(libraryId));

  const handleLogout = useCallback(async () => {
    try {
      await logout();
      toast.success("Signed out successfully");
      navigate("/login");
    } catch {
      toast.error("Failed to sign out");
    }
  }, [logout, navigate]);

  const handleCreateDefaultLibrary = useCallback(async () => {
    try {
      const library = await createLibraryMutation.mutateAsync({
        payload: {
          name: "Main",
          library_paths: [
            "/Users/robinjoseph/code/personal/shisho/tmp/library",
          ],
        },
      });
      // Backend automatically triggers a scan after library creation
      toast.success("Default library created! Scanning for media...");
      navigate(`/libraries/${library.id}`);
    } catch (e) {
      let msg = "Something went wrong.";
      if (e instanceof Error) {
        msg = e.message;
      }
      toast.error(msg);
    }
  }, [createLibraryMutation, navigate]);

  const handleLibrarySwitch = useCallback(
    (newLibraryId: number) => {
      navigate(`/libraries/${newLibraryId}`);
    },
    [navigate],
  );

  const isBooksActive =
    location.pathname === `/libraries/${libraryId}` ||
    (location.pathname.startsWith(`/libraries/${libraryId}/books`) &&
      !location.pathname.startsWith(`/libraries/${libraryId}/series`) &&
      !location.pathname.startsWith(`/libraries/${libraryId}/people`) &&
      !location.pathname.startsWith(`/libraries/${libraryId}/settings`));
  const isSeriesActive = location.pathname.startsWith(
    `/libraries/${libraryId}/series`,
  );
  const isPeopleActive = location.pathname.startsWith(
    `/libraries/${libraryId}/people`,
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
            {/* Library Switcher Dropdown */}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  className="h-9 gap-2 text-muted-foreground hover:text-foreground"
                  variant="ghost"
                >
                  <Library className="h-4 w-4" />
                  {currentLibrary?.name || "Select Library"}
                  <ChevronDown className="h-3 w-3" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="w-56">
                <DropdownMenuLabel>Libraries</DropdownMenuLabel>
                <DropdownMenuSeparator />
                {libraries.map((library) => (
                  <DropdownMenuItem
                    key={library.id}
                    onClick={() => handleLibrarySwitch(library.id)}
                  >
                    <span className="flex-1">{library.name}</span>
                    {library.id === Number(libraryId) && (
                      <Check className="h-4 w-4 text-primary" />
                    )}
                  </DropdownMenuItem>
                ))}
                {libraries.length === 0 && (
                  <DropdownMenuItem disabled>
                    <span className="text-muted-foreground">
                      No libraries found
                    </span>
                  </DropdownMenuItem>
                )}
                <DropdownMenuSeparator />
                {canCreateLibrary && (
                  <DropdownMenuItem asChild>
                    <Link to="/libraries/create">
                      <Plus className="h-4 w-4" />
                      Create new library
                    </Link>
                  </DropdownMenuItem>
                )}
                {isDevelopment && (
                  <DropdownMenuItem
                    disabled={createLibraryMutation.isPending}
                    onClick={handleCreateDefaultLibrary}
                  >
                    <BookPlus className="h-4 w-4" />
                    Create default library (dev)
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
            {/* Navigation buttons for current library */}
            {libraryId && (
              <nav className="flex gap-1">
                <Button
                  className={`h-9 ${
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
