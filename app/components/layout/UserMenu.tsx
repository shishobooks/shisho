import {
  List,
  LogOut,
  Settings,
  TabletSmartphone,
  User,
  UserCog,
} from "lucide-react";
import { useCallback } from "react";
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
import { useAuth } from "@/hooks/useAuth";
import { toastRequestError } from "@/libraries/api";

const UserMenu = () => {
  const { demoMode, user, logout } = useAuth();
  const navigate = useNavigate();

  // Administration opens the first administration page the role may see.
  const canAccessAdmin = useAdminNavItems().some((item) => item.show);

  const handleLogout = useCallback(async () => {
    try {
      await logout();
      toast.success("Signed out successfully");
      navigate("/login");
    } catch (error) {
      toastRequestError(error, "Failed to sign out");
    }
  }, [logout, navigate]);

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
