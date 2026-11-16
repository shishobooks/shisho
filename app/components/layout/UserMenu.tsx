import {
  BookmarkCheck,
  ChartColumn,
  History,
  List,
  LogOut,
  Rocket,
  Settings,
  TabletSmartphone,
  User,
  UserCog,
} from "lucide-react";
import { useCallback, useRef } from "react";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";

import { useAdminNavItems } from "@/components/pages/useAdminNavItems";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useGettingStartedPanel } from "@/contexts/GettingStarted";
import { useRestoreGettingStarted } from "@/hooks/queries/gettingStarted";
import { useAuth } from "@/hooks/useAuth";
import { toastRequestError } from "@/libraries/api";
import { anyOf, ROUTE_PERMISSIONS } from "@/utils/permissions";

const UserMenu = () => {
  const { can, demoMode, user, logout } = useAuth();
  const navigate = useNavigate();
  const restoreGettingStarted = useRestoreGettingStarted();
  const { setOpen: setGettingStartedOpen } = useGettingStartedPanel();
  // Set while the Getting started entry opens its panel, which must keep
  // focus; every other entry returns focus to the menu button.
  const openingGettingStarted = useRef(false);
  // A role that can do none of the steps has no list to bring back. The
  // whole feature is off in Demo Mode, so the entry is hidden there too.
  const canGetStarted =
    !demoMode &&
    can(anyOf("books:read", "libraries:write", "users:write", "config:write"));

  // Administration opens the first administration page the role may see.
  const canAccessAdmin = useAdminNavItems().some((item) => item.show);

  const handleLogout = useCallback(async () => {
    try {
      await logout();
      toast.success("Signed out successfully");
      // Signing out already sent the page to /login?redirect=... in place;
      // replacing that entry leaves one plain /login, so the next person to
      // sign in is not sent to the page the last one was on.
      navigate("/login", { replace: true });
    } catch (error) {
      toastRequestError(error, "Failed to sign out");
    }
  }, [logout, navigate]);

  // Brings Getting Started back after a dismissal, or turns it on for a user
  // it was never on for, and opens it.
  const handleGettingStarted = async () => {
    openingGettingStarted.current = true;
    try {
      await restoreGettingStarted.mutateAsync();
      setGettingStartedOpen(true);
    } catch (error) {
      toastRequestError(error, "Failed to show Getting Started");
    }
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          aria-label="Account menu"
          className="h-9 w-9"
          size="icon"
          variant="ghost"
        >
          <User className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        onCloseAutoFocus={(e) => {
          if (openingGettingStarted.current) {
            openingGettingStarted.current = false;
            e.preventDefault();
          }
        }}
      >
        <DropdownMenuLabel>
          <div className="flex flex-col">
            <span>{user?.username}</span>
            <span className="text-xs font-normal text-muted-foreground">
              {user?.role_name}
            </span>
          </div>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        {can(ROUTE_PERMISSIONS.toRead) && (
          <DropdownMenuItem asChild>
            <Link to="/to-read">
              <BookmarkCheck className="h-4 w-4" />
              To Read
            </Link>
          </DropdownMenuItem>
        )}
        {can(ROUTE_PERMISSIONS.readingStats) && (
          <DropdownMenuItem asChild>
            <Link to="/reading-stats">
              <ChartColumn className="h-4 w-4" />
              Reading Stats
            </Link>
          </DropdownMenuItem>
        )}
        {can(ROUTE_PERMISSIONS.readingHistory) && (
          <DropdownMenuItem asChild>
            <Link to="/reading-history">
              <History className="h-4 w-4" />
              Reading History
            </Link>
          </DropdownMenuItem>
        )}
        <DropdownMenuItem asChild>
          <Link to="/lists">
            <List className="h-4 w-4" />
            Lists
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link to="/user/devices">
            <TabletSmartphone className="h-4 w-4" />
            Devices
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link to="/user/preferences">
            <UserCog className="h-4 w-4" />
            Preferences
          </Link>
        </DropdownMenuItem>
        {canGetStarted && (
          <DropdownMenuItem onClick={handleGettingStarted}>
            <Rocket className="h-4 w-4" />
            Getting started
          </DropdownMenuItem>
        )}
        {canAccessAdmin && !demoMode && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link to="/admin">
                <Settings className="h-4 w-4" />
                Administration
              </Link>
            </DropdownMenuItem>
          </>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={handleLogout}>
          <LogOut className="h-4 w-4" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export default UserMenu;
