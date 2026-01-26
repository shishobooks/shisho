import {
  KeyRound,
  List,
  LogOut,
  Menu,
  Search,
  Settings,
  User,
  UserCog,
  X,
} from "lucide-react";
import { useCallback, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
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
import { useMobileNav } from "@/contexts/MobileNav";
import { useAuth } from "@/hooks/useAuth";
import { cn } from "@/libraries/utils";

const TopNav = () => {
  const { libraryId } = useParams();
  const navigate = useNavigate();
  const { user, logout, hasPermission } = useAuth();
  const { toggle } = useMobileNav();
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false);

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

  return (
    <div className="border-b border-border bg-background dark:bg-neutral-900 sticky top-0 z-30">
      <div className="max-w-7xl mx-auto px-4 md:px-6">
        <div className="flex items-center justify-between h-14 md:h-16">
          {/* Left section */}
          <div className="flex items-center gap-2 md:gap-8">
            {/* Mobile hamburger menu */}
            <Button
              aria-label="Open navigation menu"
              className="md:hidden h-9 w-9 -ml-1"
              onClick={toggle}
              size="icon"
              variant="ghost"
            >
              <Menu className="h-5 w-5" />
            </Button>

            {/* Logo - hidden on mobile when search is open */}
            <div
              className={cn(
                "transition-opacity duration-200",
                mobileSearchOpen && "hidden sm:block",
              )}
            >
              <Logo asLink />
            </div>

            {/* Library/List Picker - hidden on mobile, shown on tablet+ */}
            <div className="hidden sm:flex items-center gap-1">
              <LibraryListPicker />
              {libraryId && canResync && (
                <ResyncButton libraryId={Number(libraryId)} />
              )}
            </div>
          </div>

          {/* Right section */}
          <div className="flex items-center gap-1 md:gap-4">
            {/* Mobile search toggle */}
            <Button
              aria-label={mobileSearchOpen ? "Close search" : "Open search"}
              className="md:hidden h-9 w-9"
              onClick={() => setMobileSearchOpen(!mobileSearchOpen)}
              size="icon"
              variant="ghost"
            >
              {mobileSearchOpen ? (
                <X className="h-5 w-5" />
              ) : (
                <Search className="h-5 w-5" />
              )}
            </Button>

            {/* Desktop search */}
            <div className="hidden md:block">
              <GlobalSearch />
            </div>

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
                  <Link to="/lists">
                    <List className="h-4 w-4" />
                    Lists
                  </Link>
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link to="/user/security">
                    <KeyRound className="h-4 w-4" />
                    Security
                  </Link>
                </DropdownMenuItem>
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

        {/* Mobile search bar - expandable */}
        <div
          className={cn(
            "md:hidden overflow-hidden transition-all duration-200 ease-out",
            mobileSearchOpen ? "max-h-16 pb-3" : "max-h-0",
          )}
        >
          <GlobalSearch fullWidth onClose={() => setMobileSearchOpen(false)} />
        </div>
      </div>
    </div>
  );
};

export default TopNav;
