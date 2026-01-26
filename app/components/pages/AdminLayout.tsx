import {
  Briefcase,
  Cog,
  Library,
  LogOut,
  Menu,
  Puzzle,
  Users,
} from "lucide-react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { toast } from "sonner";

import Logo from "@/components/library/Logo";
import MobileDrawer from "@/components/library/MobileDrawer";
import { Button } from "@/components/ui/button";
import { Toaster } from "@/components/ui/sonner";
import { MobileNavProvider, useMobileNav } from "@/contexts/MobileNav";
import { useAuth } from "@/hooks/useAuth";

interface NavItemProps {
  to: string;
  icon: React.ReactNode;
  label: string;
  isActive: boolean;
}

const NavItem = ({ to, icon, label, isActive }: NavItemProps) => (
  <Link
    className={`flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
      isActive
        ? "bg-primary/10 text-primary dark:bg-violet-900/30 dark:text-violet-300"
        : "text-muted-foreground hover:bg-muted hover:text-foreground"
    }`}
    to={to}
  >
    {icon}
    {label}
  </Link>
);

const AdminHeader = () => {
  const { toggle } = useMobileNav();

  return (
    <div className="border-b border-border bg-background dark:bg-neutral-900 sticky top-0 z-30">
      <div className="max-w-7xl mx-auto px-4 md:px-6">
        <div className="flex items-center justify-between h-14 md:h-16">
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
            <Logo asLink />
            <span className="hidden sm:inline text-sm text-muted-foreground">
              Settings
            </span>
          </div>
          <div className="flex items-center gap-2 md:gap-4">
            <Button asChild size="sm" variant="ghost">
              <Link className="hidden sm:flex" to="/">
                ← Back to Library
              </Link>
            </Button>
            <Button asChild className="sm:hidden" size="icon" variant="ghost">
              <Link to="/">
                <svg
                  className="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth={2}
                  viewBox="0 0 24 24"
                >
                  <path
                    d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  />
                </svg>
              </Link>
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};

interface MobileNavItemProps {
  to: string;
  icon: React.ReactNode;
  label: string;
  isActive: boolean;
}

const MobileNavItem = ({ to, icon, label, isActive }: MobileNavItemProps) => (
  <Link
    className={`flex items-center gap-2 px-3 py-2 rounded-md text-sm font-medium whitespace-nowrap transition-colors ${
      isActive
        ? "bg-primary/10 text-primary dark:bg-violet-900/30 dark:text-violet-300"
        : "text-muted-foreground hover:bg-muted hover:text-foreground"
    }`}
    to={to}
  >
    {icon}
    {label}
  </Link>
);

const AdminLayoutContent = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const { user, logout, hasPermission } = useAuth();

  const handleLogout = async () => {
    try {
      await logout();
      toast.success("Logged out successfully");
      navigate("/login");
    } catch {
      toast.error("Failed to log out");
    }
  };

  const canViewLibraries = hasPermission("libraries", "read");
  const canViewUsers = hasPermission("users", "read");
  const canViewJobs = hasPermission("jobs", "read");
  const canViewConfig = hasPermission("config", "read");

  const mobileNavItems = [
    {
      to: "/settings/server",
      icon: <Cog className="h-4 w-4" />,
      label: "Server",
      isActive:
        location.pathname === "/settings/server" ||
        location.pathname === "/settings",
      show: canViewConfig,
    },
    {
      to: "/settings/libraries",
      icon: <Library className="h-4 w-4" />,
      label: "Libraries",
      isActive: location.pathname.startsWith("/settings/libraries"),
      show: canViewLibraries,
    },
    {
      to: "/settings/users",
      icon: <Users className="h-4 w-4" />,
      label: "Users",
      isActive: location.pathname.startsWith("/settings/users"),
      show: canViewUsers,
    },
    {
      to: "/settings/jobs",
      icon: <Briefcase className="h-4 w-4" />,
      label: "Jobs",
      isActive: location.pathname === "/settings/jobs",
      show: canViewJobs,
    },
    {
      to: "/settings/plugins",
      icon: <Puzzle className="h-4 w-4" />,
      label: "Plugins",
      isActive: location.pathname === "/settings/plugins",
      show: canViewConfig,
    },
  ].filter((item) => item.show);

  return (
    <div className="min-h-screen bg-background">
      <AdminHeader />
      <MobileDrawer />

      {/* Mobile horizontal nav - visible only on mobile */}
      <div className="md:hidden border-b border-border bg-background overflow-x-auto">
        <div className="flex gap-1 px-4 py-2">
          {mobileNavItems.map((item) => (
            <MobileNavItem
              icon={item.icon}
              isActive={item.isActive}
              key={item.to}
              label={item.label}
              to={item.to}
            />
          ))}
        </div>
      </div>

      <div className="max-w-7xl mx-auto px-4 md:px-6 py-4 md:py-8">
        <div className="flex gap-8">
          {/* Desktop Sidebar - hidden on mobile */}
          <aside className="hidden md:block w-56 flex-shrink-0">
            <nav className="space-y-1">
              {canViewConfig && (
                <NavItem
                  icon={<Cog className="h-4 w-4" />}
                  isActive={
                    location.pathname === "/settings/server" ||
                    location.pathname === "/settings"
                  }
                  label="Server"
                  to="/settings/server"
                />
              )}
              {canViewLibraries && (
                <NavItem
                  icon={<Library className="h-4 w-4" />}
                  isActive={location.pathname.startsWith("/settings/libraries")}
                  label="Libraries"
                  to="/settings/libraries"
                />
              )}
              {canViewUsers && (
                <NavItem
                  icon={<Users className="h-4 w-4" />}
                  isActive={location.pathname.startsWith("/settings/users")}
                  label="Users"
                  to="/settings/users"
                />
              )}
              {canViewJobs && (
                <NavItem
                  icon={<Briefcase className="h-4 w-4" />}
                  isActive={location.pathname === "/settings/jobs"}
                  label="Jobs"
                  to="/settings/jobs"
                />
              )}
              {canViewConfig && (
                <NavItem
                  icon={<Puzzle className="h-4 w-4" />}
                  isActive={location.pathname === "/settings/plugins"}
                  label="Plugins"
                  to="/settings/plugins"
                />
              )}
            </nav>

            {/* User info and logout */}
            <div className="mt-8 pt-6 border-t border-border">
              <div className="px-3 mb-3">
                <p className="text-sm font-medium text-foreground">
                  {user?.username}
                </p>
                <p className="text-xs text-muted-foreground">
                  {user?.role_name}
                </p>
              </div>
              <Button
                className="w-full justify-start gap-3 text-muted-foreground hover:text-foreground"
                onClick={handleLogout}
                size="sm"
                variant="ghost"
              >
                <LogOut className="h-4 w-4" />
                Sign out
              </Button>
              <a
                className="group mt-4 mx-3 flex items-center justify-center gap-1.5 py-1.5 rounded border border-transparent hover:border-border/40 hover:bg-muted/30 transition-all duration-200"
                href="https://github.com/shishobooks/shisho/releases"
                rel="noopener noreferrer"
                target="_blank"
              >
                <span className="text-[10px] text-muted-foreground/40 group-hover:text-muted-foreground/70 transition-colors">
                  shisho
                </span>
                <span className="text-[10px] font-mono text-muted-foreground/50 group-hover:text-muted-foreground transition-colors">
                  {__APP_VERSION__}
                </span>
              </a>
            </div>
          </aside>

          {/* Main content */}
          <main className="flex-1 min-w-0">
            <Outlet />
          </main>
        </div>
      </div>
      <Toaster richColors />
    </div>
  );
};

const AdminLayout = () => {
  return (
    <MobileNavProvider>
      <AdminLayoutContent />
    </MobileNavProvider>
  );
};

export default AdminLayout;
